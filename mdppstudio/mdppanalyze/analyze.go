// Package mdppanalyze is the only package in gosx-admin that bridges mdpp
// diagnostics and syntax trees into the mdppstudio schema.
package mdppanalyze

import (
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"m31labs.dev/gosx-admin/mdppstudio"
	"m31labs.dev/mdpp"
	"m31labs.dev/mdpp/lint"
)

// Options controls Analyze.
type Options struct {
	Required []string
	MaxBytes int
}

// Analyze parses source with mdpp.Parse, runs lint.Lint, and returns spans,
// diagnostics, headings, links, and a word count. It converts mdpp byte ranges
// to UTF-16 offsets and columns and maps severity by name.
func Analyze(source string, opts Options) mdppstudio.Analysis {
	a := mdppstudio.Analysis{Spans: []mdppstudio.Span{}, Diagnostics: []mdppstudio.Diagnostic{}, Headings: []mdppstudio.Heading{}, Links: []mdppstudio.Link{}, Engine: mdpp.Version}
	if opts.MaxBytes > 0 && len(source) > opts.MaxBytes {
		a.Diagnostics = append(a.Diagnostics, mdppstudio.Diagnostic{From: 0, To: utf16Len(source), Line: 1, Col: 1, Severity: mdppstudio.SeverityError, Code: "too-large", Message: "This text is too large to check."})
		return a
	}
	doc, err := mdpp.Parse([]byte(source))
	if err != nil {
		a.Diagnostics = append(a.Diagnostics, mdppstudio.Diagnostic{From: 0, To: utf16Len(source), Line: 1, Col: 1, Severity: mdppstudio.SeverityError, Code: "parse", Message: "The text could not be parsed."})
		return a
	}
	a.Words = doc.WordCount()
	a.Diagnostics = append(a.Diagnostics, FromMDPP(source, doc.Diagnostics())...)
	for _, d := range lint.Lint(doc) {
		from, to, line, col := rangePosition(source, d.Range.StartByte, d.Range.EndByte)
		a.Diagnostics = append(a.Diagnostics, mdppstudio.Diagnostic{From: from, To: to, Line: line, Col: col, Severity: lintSeverity(d.Severity), Code: "mdpp/" + d.Code, Message: d.Message})
	}
	if doc.Root != nil {
		doc.Root.Walk(func(n *mdpp.Node) bool {
			from, to, line, col := rangePosition(source, n.Range.StartByte, n.Range.EndByte)
			span := mdppstudio.Span{From: from, To: to}
			switch n.Type {
			case mdpp.NodeHeading:
				span.Kind = mdppstudio.SpanHeading
				span.Level = n.Level()
				if span.Level == 0 {
					span.Level = 1
				}
				a.Headings = append(a.Headings, mdppstudio.Heading{From: from, To: to, Level: span.Level, Text: n.Text(), ID: mdpp.Slugify(n.Text())})
			case mdpp.NodeEmphasis:
				span.Kind = mdppstudio.SpanEmphasis
			case mdpp.NodeStrong:
				span.Kind = mdppstudio.SpanStrong
			case mdpp.NodeStrikethrough:
				span.Kind = mdppstudio.SpanStrike
			case mdpp.NodeLink:
				span.Kind = mdppstudio.SpanLink
				href := n.Attr("href")
				if href != "" {
					kind := "link"
					raw := byteSlice(source, n.Range.StartByte, n.Range.EndByte)
					if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
						kind = "autolink"
					}
					a.Links = append(a.Links, mdppstudio.Link{From: from, To: to, Kind: kind, Href: href, Text: n.Text()})
				}
			case mdpp.NodeImage:
				span.Kind = mdppstudio.SpanImage
				a.Links = append(a.Links, mdppstudio.Link{From: from, To: to, Kind: "image", Href: n.Attr("src"), Alt: n.Attr("alt"), Text: n.Attr("alt")})
			case mdpp.NodeCodeSpan:
				span.Kind = mdppstudio.SpanCode
			case mdpp.NodeCodeBlock, mdpp.NodeDiagram:
				span.Kind = mdppstudio.SpanCodeBlock
			case mdpp.NodeBlockquote:
				span.Kind = mdppstudio.SpanQuote
			case mdpp.NodeHTMLBlock, mdpp.NodeHTMLInline:
				span.Kind = mdppstudio.SpanHTML
			case mdpp.NodeFrontmatter:
				span.Kind = mdppstudio.SpanFrontmatter
			case mdpp.NodeAdmonition, mdpp.NodeContainerDirective, mdpp.NodeTableOfContents:
				span.Kind = mdppstudio.SpanDirective
			}
			if span.Kind != "" && to >= from {
				a.Spans = append(a.Spans, span)
			}
			_ = line
			_ = col
			return true
		})
	}
	a.Spans = append(a.Spans, listMarkerSpans(source)...)
	for _, required := range opts.Required {
		found := false
		for _, h := range a.Headings {
			if strings.EqualFold(strings.TrimSpace(h.Text), strings.TrimSpace(required)) {
				found = true
				break
			}
		}
		if !found {
			a.Diagnostics = append(a.Diagnostics, mdppstudio.Diagnostic{From: 0, To: 0, Line: 1, Col: 1, Severity: mdppstudio.SeverityError, Code: "template-heading", Message: "Add the required heading: " + required + "."})
		}
	}
	sort.SliceStable(a.Spans, func(i, j int) bool {
		if a.Spans[i].From != a.Spans[j].From {
			return a.Spans[i].From < a.Spans[j].From
		}
		return a.Spans[i].To-a.Spans[i].From > a.Spans[j].To-a.Spans[j].From
	})
	return a
}

// FromMDPP converts mdpp parse diagnostics to mdppstudio diagnostics.
func FromMDPP(source string, diags []mdpp.Diagnostic) []mdppstudio.Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]mdppstudio.Diagnostic, 0, len(diags))
	for _, d := range diags {
		from, to, line, col := rangePosition(source, d.Range.StartByte, d.Range.EndByte)
		out = append(out, mdppstudio.Diagnostic{From: from, To: to, Line: line, Col: col, Severity: parseSeverity(d.Severity), Code: d.Code, Message: d.Message})
	}
	return out
}

// Analyzer returns a function for mdppstudio.Service.Analyzer.
func Analyzer(maxBytes int) func(source string, required []string) mdppstudio.Analysis {
	return func(source string, required []string) mdppstudio.Analysis {
		return Analyze(source, Options{Required: required, MaxBytes: maxBytes})
	}
}

func parseSeverity(s mdpp.Severity) mdppstudio.Severity {
	switch s {
	case mdpp.SeverityError:
		return mdppstudio.SeverityError
	case mdpp.SeverityWarning:
		return mdppstudio.SeverityWarning
	default:
		return mdppstudio.SeverityInfo
	}
}

func lintSeverity(s lint.Severity) mdppstudio.Severity {
	switch s {
	case lint.SeverityError:
		return mdppstudio.SeverityError
	case lint.SeverityWarning:
		return mdppstudio.SeverityWarning
	case lint.SeverityHint:
		return mdppstudio.SeverityHint
	default:
		return mdppstudio.SeverityInfo
	}
}

func byteSlice(source string, from, to int) string {
	if from < 0 {
		from = 0
	}
	if to > len(source) {
		to = len(source)
	}
	if from > to {
		from = to
	}
	for from > 0 && from < len(source) && !utf8.RuneStart(source[from]) {
		from--
	}
	for to < len(source) && to > 0 && !utf8.RuneStart(source[to]) {
		to++
	}
	return source[from:to]
}

func rangePosition(source string, fromByte, toByte int) (from, to, line, col int) {
	fromByte = clampByte(source, fromByte)
	toByte = clampByte(source, toByte)
	if toByte < fromByte {
		toByte = fromByte
	}
	from = utf16Len(source[:fromByte])
	to = utf16Len(source[:toByte])
	line, col = 1, 1
	for _, r := range source[:fromByte] {
		if r == '\n' {
			line++
			col = 1
		} else if r > 0xffff {
			col += 2
		} else {
			col++
		}
	}
	return
}

func clampByte(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		return len(source)
	}
	for offset > 0 && offset < len(source) && !utf8.RuneStart(source[offset]) {
		offset--
	}
	return offset
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

func listMarkerSpans(source string) []mdppstudio.Span {
	var out []mdppstudio.Span
	byteOffset := 0
	for _, line := range strings.SplitAfter(source, "\n") {
		trim := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trim)
		marker := 0
		if len(trim) > 0 && strings.ContainsRune("-*+", rune(trim[0])) && (len(trim) == 1 || trim[1] == ' ' || trim[1] == '\t') {
			marker = 1
		} else {
			i := 0
			for i < len(trim) && trim[i] >= '0' && trim[i] <= '9' {
				i++
			}
			if i > 0 && i+1 < len(trim) && (trim[i] == '.' || trim[i] == ')') && (trim[i+1] == ' ' || trim[i+1] == '\t') {
				marker = i + 1
			}
		}
		if marker > 0 {
			from, to, _, _ := rangePosition(source, byteOffset+indent, byteOffset+indent+marker)
			out = append(out, mdppstudio.Span{From: from, To: to, Kind: mdppstudio.SpanListMarker})
		}
		byteOffset += len(line)
	}
	return out
}
