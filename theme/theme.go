// Package theme serves the one stylesheet that every gosx-admin surface uses:
// workbench/render screens, schedule views, and the mdppstudio editor.
//
// A consumer themes gosx-admin only through the --gxa-* CSS custom properties
// listed by Tokens. The stylesheet declares their defaults in a :where(:root)
// block, which has zero specificity, so any consumer :root rule wins.
//
// The stylesheet is built from the files in css/, joined in file-name order.
// Each package that ships markup owns one file: 00-tokens.css and 10-base.css
// hold the tokens and the shared rules, and later files hold the rules for one
// package each. Only 00-tokens.css may contain color literals.
package theme

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Path is the URL path where a consumer mounts Handler.
const Path = "/_gxa/admin.css"

// VersionParam is the query parameter that marks a versioned stylesheet URL.
// Handler serves a request whose value matches Version with an immutable
// cache policy.
const VersionParam = "v"

// Token is one CSS custom property that a consumer may override.
type Token struct {
	// Name is the custom property name, for example "--gxa-color-ink".
	Name string
	// Default is the value the stylesheet declares.
	Default string
	// Use says what the token styles.
	Use string
}

var tokens = []Token{
	{"--gxa-color-ink", "#1f2328", "body text"},
	{"--gxa-color-muted", "#59636e", "secondary text"},
	{"--gxa-color-paper", "#f6f8fa", "page background"},
	{"--gxa-color-panel", "#ffffff", "cards, forms, bars"},
	{"--gxa-color-line", "#d1d9e0", "decorative separators only"},
	{"--gxa-color-control", "#6e7781", "input and button borders"},
	{"--gxa-color-accent", "#0b5394", "primary buttons, current nav item"},
	{"--gxa-color-accent-ink", "#ffffff", "text on accent"},
	{"--gxa-color-focus", "#b3261e", "focus ring, drawn 2 px outside the control"},
	{"--gxa-color-danger", "#b42318", "errors, destructive buttons"},
	{"--gxa-color-success", "#1a7f37", "success text"},
	{"--gxa-color-badge", "#b42318", "badge background"},
	{"--gxa-color-badge-ink", "#ffffff", "badge text"},
	{"--gxa-color-cue-heading", "#ddf4ff", "editor cue background"},
	{"--gxa-color-cue-link", "#fff8c5", "editor cue background"},
	{"--gxa-color-cue-code", "#eff2f5", "editor cue background"},
	{"--gxa-font-body", "system-ui, sans-serif", "text"},
	{"--gxa-font-mono", "ui-monospace, SFMono-Regular, Menlo, monospace", "editor source"},
	{"--gxa-radius", "0.375rem", "corners"},
	{"--gxa-space", "1rem", "base spacing unit"},
	{"--gxa-target", "44px", "minimum target size"},
	{"--gxa-focus-width", "3px", "focus ring width"},
	{"--gxa-max-width", "78rem", "content width"},
}

// Tokens returns the custom properties the stylesheet declares, in
// declaration order. The caller may modify the returned slice.
func Tokens() []Token {
	out := make([]Token, len(tokens))
	copy(out, tokens)
	return out
}

//go:embed css/*.css
var files embed.FS

var (
	buildOnce  sync.Once
	stylesheet []byte
	version    string
)

func build() {
	names, err := fs.Glob(files, "css/*.css")
	if err != nil {
		panic("theme: " + err.Error())
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		data, err := files.ReadFile(name)
		if err != nil {
			panic("theme: " + err.Error())
		}
		b.WriteString("/* " + strings.TrimPrefix(name, "css/") + " */\n")
		b.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	stylesheet = []byte(b.String())
	sum := sha256.Sum256(stylesheet)
	version = hex.EncodeToString(sum[:])[:12]
}

// Stylesheet returns the complete stylesheet. The caller must not modify the
// returned slice.
func Stylesheet() []byte {
	buildOnce.Do(build)
	return stylesheet
}

// Version returns a short content hash of the stylesheet. It changes whenever
// any byte of the stylesheet changes.
func Version() string {
	buildOnce.Do(build)
	return version
}

// Href returns Path with the current Version, for use in a <link> element.
// Handler serves this URL with an immutable cache policy.
func Href() string {
	return Path + "?" + VersionParam + "=" + Version()
}

// Handler serves the stylesheet for GET and HEAD. It sets a strong ETag and
// answers a matching If-None-Match with 304. A request whose VersionParam
// equals Version gets "public, max-age=31536000, immutable"; any other request
// gets "no-cache", so it revalidates. Other methods get 405 with an Allow
// header.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body := Stylesheet()
		etag := `"` + Version() + `"`
		h := w.Header()
		h.Set("Content-Type", "text/css; charset=utf-8")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("ETag", etag)
		if r.URL.Query().Get(VersionParam) == Version() {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	})
}

func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
