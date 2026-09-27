package mdppstudio

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func TestRenderOutputBlocked(t *testing.T) {
	for _, test := range []struct {
		name     string
		severity Severity
		want     bool
	}{
		{name: "empty", want: false}, {name: "warning", severity: SeverityWarning, want: false}, {name: "error", severity: SeverityError, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := (RenderOutput{Diagnostics: []Diagnostic{{Severity: test.severity}}}).Blocked()
			if got != test.want {
				t.Fatalf("Blocked() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOutputHashStable(t *testing.T) {
	base := RenderOutput{Version: "v1", HTML: "<p>text</p>", Text: "text"}
	if OutputHash(base) != OutputHash(base) {
		t.Fatal("equal output hashes differ")
	}
	for _, changed := range []RenderOutput{{Version: "v2", HTML: base.HTML, Text: base.Text}, {Version: base.Version, HTML: "<p>other</p>", Text: base.Text}, {Version: base.Version, HTML: base.HTML, Text: "other"}} {
		if OutputHash(base) == OutputHash(changed) {
			t.Fatalf("changed output has the same hash: %#v", changed)
		}
	}
}

func TestPreviewDocumentSandboxing(t *testing.T) {
	doc := PreviewDocument(RenderOutput{HTML: "<p>safe render</p>"}, TargetSite, "es", []string{"/site.css?a=1&b=2"})
	for _, want := range []string{"<html lang=\"es\">", "<meta charset=\"utf-8\">", "default-src 'none'; img-src https: data:", "<link rel=\"stylesheet\" href=\"/site.css?a=1&amp;b=2\">", "<p>safe render</p>"} {
		if !strings.Contains(doc, want) {
			t.Errorf("document missing %q", want)
		}
	}
	textDoc := PreviewDocument(RenderOutput{Text: "<script>alert(1)</script>"}, TargetText, "en", nil)
	if !strings.Contains(textDoc, "<pre>&lt;script&gt;alert(1)&lt;/script&gt;</pre>") || strings.Contains(textDoc, "<pre><script>") {
		t.Fatalf("text target was not escaped: %s", textDoc)
	}
}

func TestWorkerMessagesJSON(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"init request", WorkerRequest{Type: "init", Protocol: 1, WASMURL: "/mdpp-lint.wasm", ExecURL: "/wasm_exec.js", Required: []string{"Dates"}, MaxBytes: 100000}, `{"type":"init","protocol":1,"wasmURL":"/mdpp-lint.wasm","execURL":"/wasm_exec.js","required":["Dates"],"maxBytes":100000}`},
		{"analyze request", WorkerRequest{Type: "analyze", Protocol: 1, ID: 7, Source: "# Hi"}, `{"type":"analyze","protocol":1,"id":7,"source":"# Hi"}`},
		{"ready response", WorkerResponse{Type: "ready", Protocol: 1, Engine: "0.4.8"}, `{"type":"ready","protocol":1,"engine":"0.4.8"}`},
		{"result response", WorkerResponse{Type: "result", Protocol: 1, ID: 7, Engine: "0.4.8", Millis: 3, Analysis: &Analysis{Engine: "0.4.8"}}, `{"type":"result","protocol":1,"id":7,"engine":"0.4.8","millis":3,"analysis":{"spans":null,"diagnostics":null,"headings":null,"links":null,"words":0,"engine":"0.4.8"}}`},
		{"error response", WorkerResponse{Type: "error", Protocol: 1, ID: 7, Code: "wasm-load", Message: "failed"}, `{"type":"error","protocol":1,"id":7,"code":"wasm-load","message":"failed"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("JSON = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSaveCheckpointRule(t *testing.T) {
	s, store := serviceFixture(t, "one")
	at := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	manual, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "manual", Actor: "author-a", At: at})
	if err != nil || manual.Version == nil || manual.Version.Kind != VersionSave {
		t.Fatalf("manual save = %#v, %v", manual, err)
	}
	auto, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 2, Source: "autosave 1", Actor: "author-a", At: at.Add(time.Minute), Autosave: true})
	if err != nil || auto.Version != nil {
		t.Fatalf("early autosave = %#v, %v", auto, err)
	}
	auto, err = s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 3, Source: "autosave 2", Actor: "author-a", At: at.Add(5 * time.Minute), Autosave: true})
	if err != nil || auto.Version == nil || auto.Version.Kind != VersionAutosave {
		t.Fatalf("checkpoint autosave = %#v, %v", auto, err)
	}
	auto, err = s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 4, Source: "autosave 3", Actor: "author-b", At: at.Add(5*time.Minute + time.Second), Autosave: true})
	if err != nil || auto.Version == nil || auto.Version.Kind != VersionAutosave {
		t.Fatalf("author change autosave = %#v, %v", auto, err)
	}
	versions, err := store.Versions(context.Background(), "draft", 0)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions = %d, %v; want 3", len(versions), err)
	}
}

func TestSaveConflictKeepsTypedText(t *testing.T) {
	s, store := serviceFixture(t, "initial")
	winner, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "first tab", Key: testID(1), Actor: "author-a"})
	if err != nil || winner.Draft.Revision != 2 {
		t.Fatalf("winner = %#v, %v", winner, err)
	}
	_, err = s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "second tab typed text", Key: testID(2), Actor: "author-b"})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Current.Revision != 2 {
		t.Fatalf("error = %v, want conflict at revision 2", err)
	}
	current, err := store.Draft(context.Background(), "draft")
	if err != nil || current.Source != "first tab" {
		t.Fatalf("stored draft = %#v, %v", current, err)
	}
}

func TestSaveReplay(t *testing.T) {
	s, store := serviceFixture(t, "initial")
	key := testID(3)
	first, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "first", Key: key, Actor: "author-a"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "different", Key: key, Actor: "author-a"})
	if err != nil || !replay.Replayed || replay.Draft.Revision != first.Draft.Revision || replay.Draft.Source != "first" {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	versions, _ := store.Versions(context.Background(), "draft", 0)
	if len(versions) != 1 {
		t.Fatalf("replay created another version: %d", len(versions))
	}
}

func TestSaveLockedAndTooLarge(t *testing.T) {
	s, store := serviceFixture(t, "initial")
	d, _ := store.Draft(context.Background(), "draft")
	d.Locked = true
	_, err := store.PutDraft(context.Background(), DraftWrite{Draft: d, Expected: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 2, Source: "blocked", Actor: "author-a"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked error = %v", err)
	}
	d.Locked = false
	_, err = store.PutDraft(context.Background(), DraftWrite{Draft: d, Expected: 2})
	if err != nil {
		t.Fatal(err)
	}
	s.MaxBytes = 3
	if _, err = s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 3, Source: "too large", Actor: "author-a"}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large source error = %v", err)
	}
}

func TestRestoreCreatesNewRevision(t *testing.T) {
	s, store := serviceFixture(t, "revision one")
	var old Version
	for rev := int64(1); rev <= 4; rev++ {
		result, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: rev, Source: fmt.Sprintf("revision %d", rev+1), Actor: "author-a", At: time.Unix(rev, 0)})
		if err != nil {
			t.Fatal(err)
		}
		if rev == 1 {
			old = *result.Version
		}
	}
	result, err := s.Restore(context.Background(), RestoreRequest{DraftID: "draft", VersionID: old.ID, Expected: 5, Key: testID(4), Actor: "author-a"})
	if err != nil || result.Draft.Revision != 6 || result.Draft.Source != "revision 2" {
		t.Fatalf("restore = %#v, %v", result, err)
	}
	if result.Version == nil || result.Version.RestoredFrom != old.ID || result.Version.Kind != VersionRestore {
		t.Fatalf("restore version = %#v", result.Version)
	}
	versions, _ := store.Versions(context.Background(), "draft", 0)
	if len(versions) != 5 {
		t.Fatalf("history has %d versions, want 5", len(versions))
	}
}

func TestRestoreSavesPostedTextFirst(t *testing.T) {
	s, store := serviceFixture(t, "original")
	if err := store.AddVersion(context.Background(), Version{ID: testID(5), DraftID: "draft", Revision: 1, Kind: VersionSave, Source: "original", Author: "author-a"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: 1, Source: "current 2", Actor: "author-a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Save(context.Background(), SaveRequest{DraftID: "draft", Expected: first.Draft.Revision, Source: "current 3", Actor: "author-a"})
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]string{FieldDraft: "draft", FieldSource: "typed before restore", FieldRevision: fmt.Sprint(second.Draft.Revision), FieldKey: testID(6), FieldVersion: testID(5)}
	rec := actionRequest(t, s.RestoreAction(), "/administration/editor/restore", request, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore response = %d %s", rec.Code, rec.Body.String())
	}
	var envelope action.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var response SaveResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Revision != 5 || response.Source != "original" {
		t.Fatalf("response = %#v", response)
	}
	versions, _ := store.Versions(context.Background(), "draft", 0)
	foundTyped := false
	for _, v := range versions {
		if v.Source == "typed before restore" && v.Kind == VersionSave && v.Revision == 4 {
			foundTyped = true
		}
	}
	if !foundTyped {
		t.Fatalf("posted text was not saved before restore: %#v", versions)
	}
}

func TestGateBlocksMediaAndChangedOutput(t *testing.T) {
	s, _ := serviceFixture(t, "![image](https://example.test/a.png)")
	s.Analyzer = func(string, []string) Analysis {
		return Analysis{Links: []Link{{Kind: "image", Href: "https://example.test/a.png", From: 0, To: 12}}}
	}
	s.Media = MediaPolicyFunc(func(_ context.Context, ref MediaRef, use MediaUse) (MediaDecision, error) {
		if ref.URL != "https://example.test/a.png" || use.DraftID != "draft" {
			t.Fatalf("media check = %#v %#v", ref, use)
		}
		return MediaDecision{Allowed: false, Reason: "This image needs a recorded permission before it can be used."}, nil
	})
	gate, err := s.Gate(context.Background(), "draft", MediaUse{Target: TargetEmail}, "old-hash")
	if err != nil || !gate.Blocked || gate.Hash == "" {
		t.Fatalf("media gate = %#v, %v", gate, err)
	}
	s.Media = nil
	version := "v2"
	s.Renderer = RendererFunc(func(context.Context, RenderInput) (RenderOutput, error) {
		return RenderOutput{Version: version, HTML: "<p>render</p>", Text: "render"}, nil
	})
	changed, err := s.Gate(context.Background(), "draft", MediaUse{Target: TargetSite}, "reviewed-old-output")
	if err != nil || changed.Blocked || !changed.Changed {
		t.Fatalf("changed gate = %#v, %v", changed, err)
	}
	unchanged, err := s.Gate(context.Background(), "draft", MediaUse{Target: TargetSite}, "")
	if err != nil || unchanged.Changed {
		t.Fatalf("empty reviewed hash changed = %#v, %v", unchanged, err)
	}
}

func TestSaveActionManagedResponses(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		configure func(*Service) map[string]string
	}{
		{"success", http.StatusOK, func(*Service) map[string]string { return nil }},
		{"conflict", http.StatusConflict, func(*Service) map[string]string { return map[string]string{FieldRevision: "4"} }},
		{"too large", http.StatusUnprocessableEntity, func(s *Service) map[string]string { s.MaxBytes = 2; return nil }},
		{"denied", http.StatusForbidden, func(s *Service) map[string]string {
			s.CanEdit = func(context.Context, string, Draft) bool { return false }
			return nil
		}},
		{"render error", http.StatusServiceUnavailable, func(s *Service) map[string]string {
			s.Renderer = RendererFunc(func(context.Context, RenderInput) (RenderOutput, error) {
				return RenderOutput{}, errors.New("render failed")
			})
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := serviceFixture(t, "initial")
			extra := tc.configure(s)
			values := map[string]string{FieldDraft: "draft", FieldSource: "posted", FieldRevision: "1", FieldKey: testID(7)}
			for k, v := range extra {
				values[k] = v
			}
			rec := actionRequest(t, s.SaveAction(), "/administration/editor/save", values, true)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			var result action.Result
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if tc.status == http.StatusConflict && !strings.Contains(string(result.Data), `"conflict"`) {
				t.Fatalf("conflict response = %s", result.Data)
			}
		})
	}
}

func TestSaveActionAutosaveSkipsRender(t *testing.T) {
	s, _ := serviceFixture(t, "initial")
	renders := 0
	s.Renderer = RendererFunc(func(context.Context, RenderInput) (RenderOutput, error) {
		renders++
		return RenderOutput{Version: "test"}, nil
	})
	rec := actionRequest(t, s.SaveAction(), "/administration/editor/save", map[string]string{
		FieldDraft: "draft", FieldSource: "autosaved", FieldRevision: "1", FieldKey: testID(12), FieldAutosave: "1",
	}, true)
	if rec.Code != http.StatusOK || renders != 0 {
		t.Fatalf("autosave response = %d, renderer calls = %d", rec.Code, renders)
	}
}

func TestActionsRequirePost(t *testing.T) {
	s, _ := serviceFixture(t, "initial")
	for _, test := range []struct {
		name    string
		handler action.Handler
	}{
		{"save", s.SaveAction()},
		{"preview", s.PreviewAction()},
		{"restore", s.RestoreAction()},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/administration/editor/action?csrf_token=secret", nil)
			request.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			action.ServeHandler(rec, request, test.handler)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET status = %d, body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSaveActionNativeConflictStoresVersion(t *testing.T) {
	s, store := serviceFixture(t, "current text")
	rec, cookie, _ := actionRequestWithSession(t, s.SaveAction(), "/administration/editor/save", map[string]string{
		FieldDraft: "draft", FieldSource: "typed losing text", FieldRevision: "4", FieldKey: testID(8), "__gosx_return_to": "/administration/editor/draft",
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("native conflict status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/administration/editor/draft" {
		t.Fatalf("native conflict location = %q", got)
	}
	versions, err := store.Versions(context.Background(), "draft", 0)
	if err != nil || len(versions) != 1 || versions[0].Kind != VersionConflict || versions[0].Source != "typed losing text" {
		t.Fatalf("conflict versions = %#v, %v", versions, err)
	}
	var got SaveResponse
	manager, _ := session.New("mdppstudio-test-session-secret", session.Options{AllowInsecure: true})
	get := httptest.NewRequest(http.MethodGet, "http://example.test/administration/editor/draft", nil)
	get.AddCookie(cookie)
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = ReadSaveResult(r, "save")
		if !ok {
			t.Error("ReadSaveResult did not find flash")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(httptest.NewRecorder(), get)
	if got.ConflictVersion == "" || got.ConflictVersion != versions[0].ID || got.Conflict != nil && got.Conflict.Source == "typed losing text" {
		t.Fatalf("flash must keep only the version ID: %#v", got)
	}
}

func TestSaveActionBeaconIsNative(t *testing.T) {
	s, _ := serviceFixture(t, "initial")
	rec, _, _ := actionRequestWithSession(t, s.SaveAction(), "/administration/editor/save", map[string]string{FieldDraft: "draft", FieldSource: "beacon text", FieldRevision: "1", FieldKey: testID(9), FieldAutosave: "1", "__gosx_return_to": "/administration/editor/draft"}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/administration/editor/draft" {
		t.Fatalf("native/beacon response = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestEditorMarkupContract(t *testing.T) {
	page := server.NewPageState()
	markup := gosx.RenderHTML(Editor(page, EditorConfig{ID: "copy-editor", Draft: Draft{ID: "d1", Locale: "es", Title: "A draft", Source: "# Hola"}, SaveURL: "/save", PreviewURL: "/preview", RestoreURL: "/restore", Key: testID(10), Analysis: Analysis{Diagnostics: []Diagnostic{{From: 1, To: 4, Line: 1, Severity: SeverityWarning, Message: "Check this heading."}}}}))
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	assertEditorMarkupAccessible(t, root)
	if findHTML(root, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "h1" }) != nil {
		t.Fatal("editor rendered an h1")
	}
	textarea := findHTML(root, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "textarea" && attr(n, "name") == FieldSource
	})
	if textarea == nil || attr(textarea, "spellcheck") != "true" || attr(textarea, "lang") != "es" {
		t.Fatalf("textarea contract missing: %#v", textarea)
	}
	form := findHTML(root, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "form" && hasAttr(n, "data-gosx-native")
	})
	if form == nil {
		t.Fatal("native save form missing")
	}
	frame := findHTML(root, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "iframe" })
	if frame == nil || !hasAttr(frame, "sandbox") || attr(frame, "title") == "" || attr(frame, "srcdoc") == "" {
		t.Fatal("sandboxed preview frame missing")
	}
	if findHTML(root, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "textarea" && attr(n, "id") == "copy-editor-source"
	}) == nil {
		t.Fatal("native textarea editing surface missing")
	}
	if strings.Contains(markup, "<script>alert") {
		t.Fatal("textarea or preview allowed text injection")
	}
}

func TestEditorRegistersRuntimeOnlyWhenCalled(t *testing.T) {
	page := server.NewPageState()
	_ = Editor(page, EditorConfig{Draft: Draft{ID: "d"}})
	head := gosx.RenderHTML(page.Runtime().Head())
	if !strings.Contains(head, EditorScriptURL()) || !strings.Contains(head, "bootstrap-lite.js") {
		t.Fatalf("runtime head = %s", head)
	}
}

func TestNonEditorPagesLoadNoEditorAssets(t *testing.T) {
	page := server.NewPageState()
	markup := gosx.RenderHTML(gosx.Fragment(gosx.El("main", gosx.Text("plain page")), page.Runtime().Head()))
	if strings.Contains(markup, AssetPrefix) || strings.Contains(markup, ".wasm") {
		t.Fatalf("non-editor page references editor assets: %s", markup)
	}
}

func TestEditorWorkerNilMeansNoWorkerURLs(t *testing.T) {
	page := server.NewPageState()
	markup := gosx.RenderHTML(Editor(page, EditorConfig{Draft: Draft{ID: "draft", Revision: 1}, SaveURL: "/save", PreviewURL: "/preview", RestoreURL: "/restore", Key: testID(11)}))
	if !strings.Contains(markup, `"worker":null`) || strings.Contains(markup, ".wasm") || strings.Contains(markup, WorkerScriptURL()) {
		t.Fatalf("nil worker emitted an asset: %s", markup)
	}
}

func TestAssetsHeaders(t *testing.T) {
	for _, name := range []string{"editor.js", "lint-worker.js"} {
		t.Run(name, func(t *testing.T) {
			var body []byte
			if name == "editor.js" {
				body = editorScript
			} else {
				body = lintWorkerScript
			}
			versionedURL := AssetPrefix + name + "?v=" + assetHash(body)
			req := httptest.NewRequest(http.MethodGet, versionedURL, nil)
			rec := httptest.NewRecorder()
			Assets().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || rec.Body.Len() == 0 {
				t.Fatalf("asset response: %d %#v", rec.Code, rec.Header())
			}
			etag := rec.Header().Get("ETag")
			if etag != `"`+assetHash(body)+`"` {
				t.Fatalf("asset ETag = %q", etag)
			}
			if name == "lint-worker.js" && rec.Header().Get("Content-Security-Policy") != "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; connect-src 'self'" {
				t.Fatalf("worker CSP = %q", rec.Header().Get("Content-Security-Policy"))
			}
			plain := httptest.NewRecorder()
			Assets().ServeHTTP(plain, httptest.NewRequest(http.MethodGet, AssetPrefix+name, nil))
			if plain.Code != http.StatusOK || plain.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("unversioned response = %d, Cache-Control %q", plain.Code, plain.Header().Get("Cache-Control"))
			}
			conditional := httptest.NewRequest(http.MethodGet, AssetPrefix+name, nil)
			conditional.Header.Set("If-None-Match", etag)
			cached := httptest.NewRecorder()
			Assets().ServeHTTP(cached, conditional)
			if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
				t.Fatalf("conditional response = %d with %d body bytes", cached.Code, cached.Body.Len())
			}
		})
	}
	rec := httptest.NewRecorder()
	Assets().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, AssetPrefix+"editor.js", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status = %d", rec.Code)
	}
}

func TestEditorScriptBudget(t *testing.T) {
	if size := gzipSize(t, editorScript); size > 20_480 {
		t.Fatalf("editor.js gzip size = %d, budget is 20480", size)
	}
}
func TestWorkerShimBudget(t *testing.T) {
	if size := gzipSize(t, lintWorkerScript); size > 4096 {
		t.Fatalf("lint-worker.js gzip size = %d, budget is 4096", size)
	}
}

func gzipSize(t *testing.T, input []byte) int {
	t.Helper()
	var b bytes.Buffer
	zw, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zw.Write(input); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Len()
}

func serviceFixture(t *testing.T, source string) (*Service, *MemoryDraftStore) {
	t.Helper()
	store := NewMemoryDraftStore()
	_, err := store.PutDraft(context.Background(), DraftWrite{Draft: Draft{ID: "draft", Kind: "post", Status: "draft", Locale: "en", Title: "Test draft", Source: source, Author: "author-a", UpdatedBy: "author-a", Updated: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: store, Renderer: RendererFunc(func(_ context.Context, in RenderInput) (RenderOutput, error) {
		return RenderOutput{HTML: "<p>rendered</p>", Text: "rendered", Version: "test-renderer-1"}, nil
	}), Analyzer: func(string, []string) Analysis { return Analysis{} }, Actor: func(*http.Request) (string, error) { return "author-a", nil }, CanEdit: func(context.Context, string, Draft) bool { return true }, Now: func() time.Time { return time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC) }}
	return s, store
}

func actionRequest(t *testing.T, handler action.Handler, path string, values map[string]string, wantsJSON bool) *httptest.ResponseRecorder {
	t.Helper()
	rec, _, _ := actionRequestWithSession(t, handler, path, values, wantsJSON)
	return rec
}

func actionRequestWithSession(t *testing.T, handler action.Handler, path string, values map[string]string, wantsJSON bool) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	manager, err := session.New("mdppstudio-test-session-secret", session.Options{AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	var token string
	get := httptest.NewRequest(http.MethodGet, "http://example.test/administration/editor/draft", nil)
	getRec := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { token = session.Token(r) })).ServeHTTP(getRec, get)
	cookies := getRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("GET did not issue a session cookie")
	}
	values["csrf_token"] = token
	encoded := url.Values{}
	for k, v := range values {
		encoded.Set(k, v)
	}
	post := httptest.NewRequest(http.MethodPost, "http://example.test"+path, strings.NewReader(encoded.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if wantsJSON {
		post.Header.Set("Accept", "application/json")
	}
	post.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { action.ServeHandler(w, r, handler) })).ServeHTTP(rec, post)
	postCookies := rec.Result().Cookies()
	if len(postCookies) > 0 {
		return rec, postCookies[0], token
	}
	return rec, cookies[0], token
}

func testID(n int) string { return fmt.Sprintf("%032x", n) }
func findHTML(root *html.Node, predicate func(*html.Node) bool) *html.Node {
	if predicate(root) {
		return root
	}
	for nested := root.FirstChild; nested != nil; nested = nested.NextSibling {
		if found := findHTML(nested, predicate); found != nil {
			return found
		}
	}
	return nil
}

func assertEditorMarkupAccessible(t *testing.T, root *html.Node) {
	t.Helper()
	var nodes []*html.Node
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		nodes = append(nodes, n)
		for nested := n.FirstChild; nested != nil; nested = nested.NextSibling {
			collect(nested)
		}
	}
	collect(root)
	ids := make(map[string]*html.Node)
	previousHeadingLevel := 0
	for _, n := range nodes {
		if id := attr(n, "id"); id != "" {
			if ids[id] != nil {
				t.Errorf("duplicate id %q", id)
			}
			ids[id] = n
		}
	}
	textContent := func(root *html.Node) string {
		var b strings.Builder
		var appendText func(*html.Node)
		appendText = func(n *html.Node) {
			if n.Type == html.TextNode {
				b.WriteString(n.Data)
			}
			for nested := n.FirstChild; nested != nil; nested = nested.NextSibling {
				appendText(nested)
			}
		}
		appendText(root)
		return strings.TrimSpace(b.String())
	}
	for _, n := range nodes {
		if n.Type != html.ElementNode {
			continue
		}
		for _, name := range []string{"aria-labelledby", "aria-describedby"} {
			for _, ref := range strings.Fields(attr(n, name)) {
				if ids[ref] == nil {
					t.Errorf("%s references missing id %q", name, ref)
				}
			}
		}
		switch n.Data {
		case "input", "select", "textarea":
			if n.Data == "input" {
				typ := strings.ToLower(attr(n, "type"))
				if typ == "hidden" || typ == "submit" || typ == "button" || typ == "reset" || typ == "image" {
					continue
				}
			}
			labelled := attr(n, "aria-label") != "" || attr(n, "aria-labelledby") != ""
			if id := attr(n, "id"); id != "" && findHTML(root, func(candidate *html.Node) bool {
				return candidate.Type == html.ElementNode && candidate.Data == "label" && attr(candidate, "for") == id
			}) != nil {
				labelled = true
			}
			if !labelled {
				t.Errorf("%s has no accessible label", n.Data)
			}
		case "button":
			if textContent(n) == "" && attr(n, "aria-label") == "" && attr(n, "aria-labelledby") == "" {
				t.Error("button has no accessible name")
			}
		case "a":
			if hasAttr(n, "href") && textContent(n) == "" && attr(n, "aria-label") == "" && attr(n, "aria-labelledby") == "" {
				t.Error("link has no accessible name")
			}
		case "img":
			if !hasAttr(n, "alt") {
				t.Error("image has no alt attribute")
			}
		case "table":
			if findHTML(n, func(candidate *html.Node) bool { return candidate.Type == html.ElementNode && candidate.Data == "th" }) == nil {
				t.Error("table has no header cell")
			}
		}
		if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			level := int(n.Data[1] - '0')
			if previousHeadingLevel != 0 && level > previousHeadingLevel+1 {
				t.Errorf("heading %s skips level from h%d", n.Data, previousHeadingLevel)
			}
			previousHeadingLevel = level
		}
	}
}

func attr(node *html.Node, name string) string {
	for _, a := range node.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func hasAttr(node *html.Node, name string) bool {
	for _, a := range node.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}
