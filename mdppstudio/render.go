package mdppstudio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"strings"
)

// Target is an output format.
type Target string

const (
	// TargetSite is HTML for a web page.
	TargetSite Target = "site"
	// TargetEmail is email HTML plus its plain text alternative.
	TargetEmail Target = "email"
	// TargetText is plain text only.
	TargetText Target = "text"
)

// RenderInput is one render request.
type RenderInput struct {
	Source string
	Target Target
	// Locale is a consumer-defined tag, for example "es".
	Locale string
	// Template is the Template ID the source belongs to, or "". The renderer
	// receives Source already composed with the template's locked parts.
	Template string
}

// RenderOutput is one render result.
type RenderOutput struct {
	// HTML is set for TargetSite and TargetEmail.
	HTML string
	// Text is set for TargetText and TargetEmail.
	Text        string
	Diagnostics []Diagnostic
	// Version identifies the renderer and its settings. It must change
	// whenever the output for the same source can change.
	Version string
}

// Blocked reports whether any diagnostic has SeverityError.
func (o RenderOutput) Blocked() bool {
	for _, d := range o.Diagnostics {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Renderer turns source into safe output. Implementations sanitize or reject
// raw HTML and unsafe URLs; mdppstudio does not sanitize. Return an error
// only when rendering could not run; report source problems as diagnostics.
type Renderer interface {
	Render(ctx context.Context, in RenderInput) (RenderOutput, error)
}

// RendererFunc adapts a function to Renderer.
type RendererFunc func(ctx context.Context, in RenderInput) (RenderOutput, error)

// Render calls f.
func (f RendererFunc) Render(ctx context.Context, in RenderInput) (RenderOutput, error) {
	return f(ctx, in)
}

// OutputHash returns the lower-case hex SHA-256 of
// Version + "\x00" + HTML + "\x00" + Text.
func OutputHash(o RenderOutput) string {
	sum := sha256.Sum256([]byte(o.Version + "\x00" + o.HTML + "\x00" + o.Text))
	return hex.EncodeToString(sum[:])
}

// PreviewDocument wraps rendered HTML in a complete document for an
// <iframe sandbox srcdoc>. For TargetText it wraps escaped text in <pre>.
func PreviewDocument(out RenderOutput, target Target, lang string, stylesheets []string) string {
	if lang == "" {
		lang = "en"
	}
	var b strings.Builder
	b.Grow(len(out.HTML) + len(out.Text) + 256)
	b.WriteString("<!doctype html><html lang=\"")
	b.WriteString(html.EscapeString(lang))
	b.WriteString("\"><head><meta charset=\"utf-8\"><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src https: data:; style-src 'self' https: 'unsafe-inline'\">")
	for _, href := range stylesheets {
		if strings.TrimSpace(href) == "" {
			continue
		}
		b.WriteString("<link rel=\"stylesheet\" href=\"")
		b.WriteString(html.EscapeString(href))
		b.WriteString("\">")
	}
	b.WriteString("</head><body>")
	if target == TargetText {
		b.WriteString("<pre>")
		b.WriteString(html.EscapeString(out.Text))
		b.WriteString("</pre>")
	} else {
		b.WriteString(out.HTML)
	}
	b.WriteString("</body></html>")
	return b.String()
}

func validTarget(target Target) bool {
	switch target {
	case TargetSite, TargetEmail, TargetText:
		return true
	default:
		return false
	}
}

func validateTarget(target Target) error {
	if !validTarget(target) {
		return fmt.Errorf("mdppstudio: unsupported target %q", target)
	}
	return nil
}
