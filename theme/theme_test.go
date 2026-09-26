package theme

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestStylesheetDeclaresEveryToken(t *testing.T) {
	css := string(Stylesheet())
	for _, tok := range Tokens() {
		decl := tok.Name + ": " + tok.Default + ";"
		if got := strings.Count(css, tok.Name+":"); got != 1 {
			t.Errorf("%s declared %d times, want 1", tok.Name, got)
		}
		if !strings.Contains(css, decl) {
			t.Errorf("stylesheet lacks %q", decl)
		}
	}
	if len(Tokens()) == 0 {
		t.Fatal("no tokens")
	}
}

var colorLiteral = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(`)

func TestStylesheetHasNoColorLiteralsOutsideTokens(t *testing.T) {
	names, err := files.ReadDir("css")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range names {
		if entry.Name() == "00-tokens.css" {
			continue
		}
		data, err := files.ReadFile("css/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if loc := colorLiteral.FindIndex(data); loc != nil {
			t.Errorf("%s has a color literal %q; use a --gxa-* token", entry.Name(), data[loc[0]:loc[1]])
		}
	}
}

func TestStylesheetHandlerCaching(t *testing.T) {
	h := Handler()

	versioned := httptest.NewRecorder()
	h.ServeHTTP(versioned, httptest.NewRequest(http.MethodGet, Href(), nil))
	if versioned.Code != http.StatusOK {
		t.Fatalf("versioned GET = %d", versioned.Code)
	}
	if got := versioned.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("versioned Cache-Control = %q", got)
	}
	if got := versioned.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Errorf("Content-Type = %q", got)
	}
	if versioned.Body.String() != string(Stylesheet()) {
		t.Error("body differs from Stylesheet()")
	}

	plain := httptest.NewRecorder()
	h.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, Path, nil))
	if got := plain.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("unversioned Cache-Control = %q", got)
	}
	stale := httptest.NewRecorder()
	h.ServeHTTP(stale, httptest.NewRequest(http.MethodGet, Path+"?v=000000000000", nil))
	if got := stale.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("stale-version Cache-Control = %q", got)
	}

	etag := versioned.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	req.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	h.ServeHTTP(notModified, req)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("If-None-Match = %d with %d body bytes, want 304 and none", notModified.Code, notModified.Body.Len())
	}

	head := httptest.NewRecorder()
	h.ServeHTTP(head, httptest.NewRequest(http.MethodHead, Path, nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD = %d with %d body bytes", head.Code, head.Body.Len())
	}

	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, Path, nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST = %d, Allow %q", post.Code, post.Header().Get("Allow"))
	}
}

func TestVersionTracksContent(t *testing.T) {
	if len(Version()) != 12 {
		t.Fatalf("Version() = %q, want 12 hex characters", Version())
	}
	if !strings.HasSuffix(Href(), "?v="+Version()) || !strings.HasPrefix(Href(), Path) {
		t.Errorf("Href() = %q", Href())
	}
	if !strings.Contains(string(Stylesheet()), "/* 00-tokens.css */") {
		t.Error("stylesheet does not start from the token file")
	}
}
