package mdppstudio

// Severity grades a diagnostic.
type Severity string

const (
	// SeverityError blocks publishing and sending.
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
	SeverityHint    Severity = "hint"
)

// Diagnostic is one problem in the source, in plain language.
// Offsets are UTF-16 code unit offsets into the source. From is inclusive and
// To is exclusive. Line and Col are 1-based; Col counts UTF-16 code units.
type Diagnostic struct {
	From     int      `json:"from"`
	To       int      `json:"to"`
	Line     int      `json:"line"`
	Col      int      `json:"col"`
	Severity Severity `json:"severity"`
	// Code is stable, for example "mdpp/heading-increment".
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SpanKind names a syntax cue.
type SpanKind string

const (
	SpanHeading     SpanKind = "heading"
	SpanEmphasis    SpanKind = "emphasis"
	SpanStrong      SpanKind = "strong"
	SpanStrike      SpanKind = "strike"
	SpanLink        SpanKind = "link"
	SpanImage       SpanKind = "image"
	SpanCode        SpanKind = "code"
	SpanCodeBlock   SpanKind = "code-block"
	SpanQuote       SpanKind = "quote"
	SpanListMarker  SpanKind = "list-marker"
	SpanHTML        SpanKind = "html"
	SpanFrontmatter SpanKind = "frontmatter"
	SpanDirective   SpanKind = "directive"
)

// Span is one syntax cue. Spans are ordered by From, then by length,
// longest first. Inline spans may sit inside block spans.
type Span struct {
	From int      `json:"from"`
	To   int      `json:"to"`
	Kind SpanKind `json:"kind"`
	// Level is the heading level for SpanHeading, else 0.
	Level int `json:"level,omitempty"`
}

// Heading is one heading in the source.
type Heading struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Level int    `json:"level"`
	Text  string `json:"text"`
	// ID is the anchor the site renderer gives the heading.
	ID string `json:"id"`
}

// Link is one link or image in the source.
type Link struct {
	From int `json:"from"`
	To   int `json:"to"`
	// Kind is "link", "image", or "autolink".
	Kind string `json:"kind"`
	Href string `json:"href"`
	Text string `json:"text"`
	// Alt is the image alt text; empty for links.
	Alt string `json:"alt,omitempty"`
}

// Analysis is the lint result for one source.
type Analysis struct {
	Spans       []Span       `json:"spans"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Headings    []Heading    `json:"headings"`
	Links       []Link       `json:"links"`
	Words       int          `json:"words"`
	// Engine is the mdpp version that produced it.
	Engine string `json:"engine"`
}
