//go:build e2e

package mdppstudio_test

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/mdppstudio"
	"m31labs.dev/gosx-admin/mdppstudio/mdppanalyze"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const e2eEditorPath = "/administration/editor/draft"

type editorE2EApp struct {
	store    *mdppstudio.MemoryDraftStore
	service  *mdppstudio.Service
	sessions *session.Manager
	server   *httptest.Server
	saves    atomic.Int64
	assets   atomic.Int64
	keys     atomic.Uint64
	pages    atomic.Int64
}

func newEditorE2EApp(t *testing.T) *editorE2EApp {
	t.Helper()
	store := mdppstudio.NewMemoryDraftStore()
	_, err := store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: mdppstudio.Draft{ID: "editor-draft", Kind: "weekly-update", Status: "draft", Locale: "en", Title: "Weekly update", Source: "# Weekly update\n\n", Author: "editor", UpdatedBy: "editor", Updated: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	app := &editorE2EApp{store: store}
	app.service = &mdppstudio.Service{
		Store: store,
		Renderer: mdppstudio.RendererFunc(func(_ context.Context, in mdppstudio.RenderInput) (mdppstudio.RenderOutput, error) {
			out := mdppstudio.RenderOutput{Version: "e2e-renderer-1"}
			switch in.Target {
			case mdppstudio.TargetText:
				out.Text = in.Source
			case mdppstudio.TargetEmail:
				out.HTML, out.Text = "<pre>"+html.EscapeString(in.Source)+"</pre>", in.Source
			default:
				out.HTML = "<pre>" + html.EscapeString(in.Source) + "</pre>"
			}
			return out, nil
		}),
		Analyzer:    mdppanalyze.Analyzer(100_000),
		Actor:       func(*http.Request) (string, error) { return "editor", nil },
		CanEdit:     func(context.Context, string, mdppstudio.Draft) bool { return true },
		TargetFor:   func(mdppstudio.Draft) mdppstudio.Target { return mdppstudio.TargetSite },
		Stylesheets: []string{"/theme.css"},
	}
	app.sessions, err = session.New("mdppstudio-chromedp-session-secret", session.Options{AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	app.serve(t)
	return app
}

func (a *editorE2EApp) serve(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+e2eEditorPath, a.editor)
	mux.HandleFunc("GET /plain", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><body><main>Plain route</main></body></html>"))
	})
	mux.HandleFunc("POST /administration/editor/save", func(w http.ResponseWriter, r *http.Request) {
		a.saves.Add(1)
		action.ServeHandler(w, r, a.service.SaveAction())
	})
	mux.HandleFunc("POST /administration/editor/preview", func(w http.ResponseWriter, r *http.Request) { action.ServeHandler(w, r, a.service.PreviewAction()) })
	mux.HandleFunc("POST /administration/editor/restore", func(w http.ResponseWriter, r *http.Request) { action.ServeHandler(w, r, a.service.RestoreAction()) })
	mux.Handle("/_gxa/mdppstudio/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.assets.Add(1)
		mdppstudio.Assets().ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /theme.css", func(w http.ResponseWriter, _ *http.Request) {
		_, file, _, _ := runtime.Caller(0)
		css, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "theme", "css", "40-mdppstudio.css"))
		if err != nil {
			http.Error(w, "stylesheet unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write([]byte(":root{font-size:16px;--gxa-color-ink:#1f2328;--gxa-color-muted:#59636e;--gxa-color-paper:#f6f8fa;--gxa-color-panel:#fff;--gxa-color-line:#d1d9e0;--gxa-color-control:#6e7781;--gxa-color-accent:#0b5394;--gxa-color-accent-ink:#fff;--gxa-color-focus:#b3261e;--gxa-color-danger:#b42318;--gxa-color-success:#1a7f37;--gxa-color-cue-heading:#ddf4ff;--gxa-color-cue-link:#fff8c5;--gxa-color-cue-code:#eff2f5;--gxa-font-body:system-ui,sans-serif;--gxa-font-mono:ui-monospace,monospace;--gxa-radius:.375rem;--gxa-space:1rem;--gxa-target:44px;--gxa-focus-width:3px;--gxa-max-width:78rem}body{margin:0;padding:1rem;font-family:var(--gxa-font-body)}"))
		_, _ = w.Write(css)
	})
	a.server = httptest.NewServer(a.sessions.Middleware(mux))
	t.Cleanup(a.server.Close)
}

func (a *editorE2EApp) editor(w http.ResponseWriter, r *http.Request) {
	a.pages.Add(1)
	draft, err := a.store.Draft(r.Context(), "editor-draft")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	result, hasResult := mdppstudio.ReadSaveResult(r, "save")
	var conflict *mdppstudio.EditorConflict
	if hasResult && result.Conflict != nil && result.ConflictVersion != "" {
		version, e := a.store.Version(r.Context(), draft.ID, result.ConflictVersion)
		if e == nil {
			conflict = &mdppstudio.EditorConflict{Saved: *result.Conflict, Mine: version.Source}
		}
	}
	targets := []mdppstudio.Target{mdppstudio.TargetSite, mdppstudio.TargetEmail, mdppstudio.TargetText}
	config := mdppstudio.EditorConfig{ID: "gxa-editor", Draft: draft, SaveURL: "/administration/editor/save", PreviewURL: "/administration/editor/preview", RestoreURL: "/administration/editor/restore", CSRF: session.Token(r), Key: fmt.Sprintf("%032x", a.keys.Add(1)), ReturnTo: e2eEditorPath, Targets: targets, Analysis: mdppanalyze.Analyze(draft.Source, mdppanalyze.Options{MaxBytes: 100_000}), Versions: versionSummaries(r.Context(), a.store, draft.ID), Stylesheets: []string{"/theme.css"}, Conflict: conflict}
	if r.URL.Query().Get("worker") == "missing" {
		config.Worker = &mdppstudio.WorkerAssets{ScriptURL: "/_gxa/mdppstudio/missing-worker.js", WASMURL: "/missing.wasm", ExecURL: "/missing-wasm_exec.js"}
	}
	page := serverPage()
	markup := gosx.RenderHTML(mdppstudio.Editor(page, config))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><link rel=\"stylesheet\" href=\"/theme.css\"></head><body><h1>Editor test</h1>%s<script defer src=\"%s\"></script></body></html>", markup, mdppstudio.EditorScriptURL())
}

func serverPage() *server.PageState { return server.NewPageState() }

func versionSummaries(ctx context.Context, store mdppstudio.DraftStore, id string) []mdppstudio.VersionSummary {
	versions, err := store.Versions(ctx, id, 20)
	if err != nil {
		return nil
	}
	out := make([]mdppstudio.VersionSummary, 0, len(versions))
	for _, v := range versions {
		out = append(out, v.Summary())
	}
	return out
}

func newChrome(t *testing.T) (context.Context, func()) {
	t.Helper()
	allocator, stopAllocator := chromedp.NewExecAllocator(context.Background(), chromedp.ExecPath("/usr/bin/google-chrome"), chromedp.Headless, chromedp.NoSandbox, chromedp.Flag("disable-dev-shm-usage", true), chromedp.Flag("disable-gpu", true))
	browser, stopBrowser := chromedp.NewContext(allocator)
	ctx, stopTimeout := context.WithTimeout(browser, 45*time.Second)
	return ctx, func() { stopTimeout(); stopBrowser(); stopAllocator() }
}

func waitForRevision(t *testing.T, app *editorE2EApp, revision int64) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		draft, err := app.store.Draft(context.Background(), "editor-draft")
		if err == nil && draft.Revision >= revision {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	draft, _ := app.store.Draft(context.Background(), "editor-draft")
	t.Fatalf("draft did not reach revision %d; current revision is %d", revision, draft.Revision)
}

func waitForPageLoads(t *testing.T, app *editorE2EApp, count int64) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if app.pages.Load() >= count {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("editor page did not load %d times; got %d", count, app.pages.Load())
}

func TestE2EEditorTypingKeepsNativeTextarea(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	var result string
	err := chromedp.Run(ctx,
		chromedp.Navigate(app.server.URL+e2eEditorPath),
		chromedp.WaitVisible("#gxa-editor-source"),
		chromedp.Focus("#gxa-editor-source"),
		chromedp.SendKeys("#gxa-editor-source", "Hello"),
		chromedp.Evaluate(`(() => { const e=document.querySelector('#gxa-editor-source'); e.setSelectionRange(0, 5); const selected=e.selectionStart===0&&e.selectionEnd===5; const native=e.spellcheck&&document.activeElement===e; e.setSelectionRange(e.value.length,e.value.length); document.execCommand('insertText',false,' undo-test'); const inserted=e.value.endsWith(' undo-test'); document.execCommand('undo'); return JSON.stringify({selected,native,inserted,undone:!e.value.endsWith(' undo-test'),cueFocused:document.activeElement===document.querySelector('.gxa-editor__cues')}); })()`, &result),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"selected":true`) || !strings.Contains(result, `"native":true`) || !strings.Contains(result, `"inserted":true`) || !strings.Contains(result, `"undone":true`) || strings.Contains(result, `"cueFocused":true`) {
		t.Fatalf("textarea interaction = %s", result)
	}
}

func TestE2EAutosaveAfterTwoSecondsIdle(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	if err := chromedp.Run(ctx, chromedp.Navigate(app.server.URL+e2eEditorPath), chromedp.WaitVisible("#gxa-editor-source"), chromedp.Focus("#gxa-editor-source"), chromedp.SendKeys("#gxa-editor-source", "Idle save"), chromedp.Sleep(1200*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got := app.saves.Load(); got != 0 {
		t.Fatalf("save fired before two seconds: %d", got)
	}
	waitForRevision(t, app, 2)
	if got := app.saves.Load(); got != 1 {
		t.Fatalf("autosave requests = %d, want one", got)
	}
}

func TestE2EOfflineKeepsText(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(app.server.URL+e2eEditorPath),
		chromedp.WaitVisible("#gxa-editor-source"),
		chromedp.Evaluate(`(() => { window.__gxaFetch = window.fetch; window.fetch = () => Promise.reject(new TypeError('Failed to fetch')); Object.defineProperty(navigator, 'onLine', { configurable: true, value: false }); window.dispatchEvent(new Event('offline')); })()`, nil),
		chromedp.Focus("#gxa-editor-source"),
		chromedp.SendKeys("#gxa-editor-source", "Offline text"),
		chromedp.PollFunction(`() => document.querySelector('.gxa-editor__saved').textContent.includes('offline')`, nil, chromedp.WithPollingInterval(100*time.Millisecond), chromedp.WithPollingTimeout(8*time.Second)),
	); err != nil {
		var state string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({status:document.querySelector('.gxa-editor__saved').textContent,online:navigator.onLine,text:document.querySelector('#gxa-editor-source').value})`, &state))
		t.Fatalf("offline state not shown: %v; browser state %s; saves=%d", err, state, app.saves.Load())
	}
	var stillThere bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#gxa-editor-source').value.includes('Offline text')`, &stillThere)); err != nil || !stillThere {
		t.Fatalf("offline text was lost: %v %v", stillThere, err)
	}
	if got := app.saves.Load(); got != 0 {
		t.Fatalf("save reached the server while offline: %d", got)
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`(() => { window.fetch = window.__gxaFetch; Object.defineProperty(navigator, 'onLine', { configurable: true, value: true }); window.dispatchEvent(new Event('online')); })()`, nil),
		chromedp.PollFunction(`() => document.querySelector('.gxa-editor__saved').textContent.startsWith('Saved at')`, nil, chromedp.WithPollingInterval(100*time.Millisecond), chromedp.WithPollingTimeout(8*time.Second)),
	); err != nil {
		t.Fatalf("save did not retry online: %v", err)
	}
	waitForRevision(t, app, 2)
	if got := app.saves.Load(); got != 1 {
		t.Fatalf("retry save requests = %d, want one", got)
	}
}

func TestE2EConflictTwoTabs(t *testing.T) {
	app := newEditorE2EApp(t)
	first, closeChrome := newChrome(t)
	defer closeChrome()
	if err := chromedp.Run(first, chromedp.Navigate(app.server.URL+e2eEditorPath), chromedp.WaitVisible("#gxa-editor-source")); err != nil {
		t.Fatal(err)
	}
	second, closeSecond := chromedp.NewContext(first)
	defer closeSecond()
	if err := chromedp.Run(second, chromedp.Navigate(app.server.URL+e2eEditorPath), chromedp.WaitVisible("#gxa-editor-source")); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(first, chromedp.Focus("#gxa-editor-source"), chromedp.SendKeys("#gxa-editor-source", "winner")); err != nil {
		t.Fatal(err)
	}
	waitForRevision(t, app, 2)
	if err := chromedp.Run(second, chromedp.Focus("#gxa-editor-source"), chromedp.SendKeys("#gxa-editor-source", "loser typed text"), chromedp.PollFunction(`() => !!document.querySelector('[data-gxa-conflict]')`, nil, chromedp.WithPollingInterval(100*time.Millisecond), chromedp.WithPollingTimeout(10*time.Second))); err != nil {
		t.Fatalf("second tab did not show conflict: %v", err)
	}
	if err := chromedp.Run(second, chromedp.Evaluate(`document.querySelector('[data-gxa-resolve="mine"]').click()`, nil)); err != nil {
		t.Fatalf("keep mine button did not run: %v", err)
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		draft, err := app.store.Draft(context.Background(), "editor-draft")
		if err == nil && draft.Revision >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	draft, err := app.store.Draft(context.Background(), "editor-draft")
	if err != nil || !strings.Contains(draft.Source, "loser typed text") || draft.Revision != 3 {
		var state string
		_ = chromedp.Run(second, chromedp.Evaluate(`JSON.stringify({status:document.querySelector('.gxa-editor__saved').textContent,revision:document.querySelector('[name="gxa_revision"]').value,text:document.querySelector('#gxa-editor-source').value,conflict:!!document.querySelector('[data-gxa-conflict]')})`, &state))
		t.Fatalf("stored conflict resolution = %#v, %v; saves=%d; browser=%s", draft, err, app.saves.Load(), state)
	}
}

func TestE2ENoJavaScriptSaveAndConflict(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error { return emulation.SetScriptExecutionDisabled(true).Do(ctx) }), chromedp.Navigate(app.server.URL+e2eEditorPath), chromedp.WaitVisible("#gxa-editor-source"), chromedp.SetValue("#gxa-editor-source", "Native saved text")); err != nil {
		t.Fatalf("native save failed: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Click(`.gxa-editor__bar button[type="submit"]`)); err != nil {
		t.Fatalf("native save submit failed: %v", err)
	}
	waitForRevision(t, app, 2)
	waitForPageLoads(t, app, 2)
	if err := chromedp.Run(ctx, chromedp.WaitVisible("#gxa-editor-source")); err != nil {
		t.Fatalf("native save redirect failed: %v", err)
	}
	draft, err := app.store.Draft(context.Background(), "editor-draft")
	if err != nil || !strings.HasSuffix(draft.Source, "Native saved text") || draft.Revision != 2 {
		t.Fatalf("native saved draft = %#v, %v", draft, err)
	}
	if _, err = app.service.Save(context.Background(), mdppstudio.SaveRequest{DraftID: draft.ID, Expected: draft.Revision, Source: "External writer", Actor: "editor"}); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.SetValue("#gxa-editor-source", "Native conflict text")); err != nil {
		t.Fatalf("set native conflict text: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Click(`.gxa-editor__bar button[type="submit"]`)); err != nil {
		t.Fatalf("native conflict navigation failed: %v", err)
	}
	waitForPageLoads(t, app, 3)
	if err := chromedp.Run(ctx, chromedp.WaitVisible("#gxa-editor-source"), chromedp.PollFunction(`() => !!document.querySelector('[data-gxa-conflict]')`, nil, chromedp.WithPollingInterval(100*time.Millisecond), chromedp.WithPollingTimeout(10*time.Second))); err != nil {
		t.Fatalf("native conflict failed: %v", err)
	}
	var state string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({text:document.querySelector('#gxa-editor-source').value,buttons:document.querySelector('.gxa-editor__conflict-actions').hidden,native:document.querySelector('.gxa-editor__conflict-native').textContent})`, &state)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state, "Native conflict text") || !strings.Contains(state, `"buttons":true`) || !strings.Contains(state, "Saving now makes your text the new version.") {
		t.Fatalf("native conflict fallback = %s", state)
	}
	versions, _ := app.store.Versions(context.Background(), draft.ID, 0)
	found := false
	for _, v := range versions {
		if v.Kind == mdppstudio.VersionConflict && v.Source == "Native conflict text" {
			found = true
		}
	}
	if !found {
		t.Fatal("native conflict did not keep typed text in a VersionConflict")
	}
}

func TestE2EPaneSwitchNarrow(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	var state string
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error { return emulation.SetDeviceMetricsOverride(320, 844, 1, false).Do(ctx) }), chromedp.Navigate(app.server.URL+e2eEditorPath), chromedp.WaitVisible("#gxa-editor-source"), chromedp.Evaluate(`document.documentElement.style.fontSize='200%'`, nil), chromedp.PollFunction(`() => !document.querySelector('.gxa-editor__switch').hidden`, nil, chromedp.WithPollingInterval(100*time.Millisecond), chromedp.WithPollingTimeout(10*time.Second)), chromedp.Click(`.gxa-editor__switch button[data-gxa-pane="preview"]`), chromedp.Evaluate(`JSON.stringify({pressed:document.querySelector('.gxa-editor__switch button[data-gxa-pane="preview"]').getAttribute('aria-pressed'),write:getComputedStyle(document.querySelector('.gxa-editor__pane--write')).display,scroll:document.documentElement.scrollWidth,width:window.innerWidth})`, &state)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state, `"pressed":"true"`) || !strings.Contains(state, `"write":"none"`) || !strings.Contains(state, `"scroll":320`) {
		t.Fatalf("narrow editor = %s", state)
	}
}

func TestE2ENonEditorRouteRequests(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	var editorRequests atomic.Int64
	chromedp.ListenTarget(ctx, func(event any) {
		request, ok := event.(*network.EventRequestWillBeSent)
		if !ok {
			return
		}
		if strings.Contains(request.Request.URL, mdppstudio.AssetPrefix) || strings.HasSuffix(request.Request.URL, ".wasm") {
			editorRequests.Add(1)
		}
	})
	if err := chromedp.Run(ctx, network.Enable(), chromedp.Navigate(app.server.URL+"/plain"), chromedp.WaitVisible("main")); err != nil {
		t.Fatal(err)
	}
	if got := editorRequests.Load(); got != 0 {
		t.Fatalf("non-editor route made %d editor or WASM requests", got)
	}
	if got := app.assets.Load(); got != 0 {
		t.Fatalf("non-editor route requested %d editor assets", got)
	}
}

func TestE2EWorkerFallback(t *testing.T) {
	app := newEditorE2EApp(t)
	ctx, closeChrome := newChrome(t)
	defer closeChrome()
	if err := chromedp.Run(ctx, chromedp.Navigate(app.server.URL+e2eEditorPath+"?worker=missing"), chromedp.WaitVisible("#gxa-editor-source"), chromedp.PollFunction(`() => !document.querySelector('.gxa-editor__fallback').hidden`, nil, chromedp.WithPollingInterval(100*time.Millisecond)), chromedp.PollFunction(`() => document.querySelector('.gxa-editor__frame').srcdoc.includes('<pre>')`, nil, chromedp.WithPollingInterval(100*time.Millisecond))); err != nil {
		t.Fatalf("worker fallback did not use server preview: %v", err)
	}
}
