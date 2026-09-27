package mdppanalyze

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"m31labs.dev/gosx-admin/mdppstudio"
	"m31labs.dev/mdpp"
	"m31labs.dev/mdpp/lint"
)

func TestAnalyzeOffsetsUTF16(t *testing.T) {
	source := "😀 café 你好\n# A heading\n\n**bold** and ![](image.png)"
	analysis := Analyze(source, Options{})
	if len(analysis.Spans) == 0 {
		t.Fatal("expected syntax spans")
	}
	for _, span := range analysis.Spans {
		assertUTF16Range(t, source, span.From, span.To)
	}
	for _, heading := range analysis.Headings {
		assertUTF16Range(t, source, heading.From, heading.To)
	}
	for _, link := range analysis.Links {
		assertUTF16Range(t, source, link.From, link.To)
	}
	for _, diagnostic := range analysis.Diagnostics {
		assertUTF16Range(t, source, diagnostic.From, diagnostic.To)
	}
	strong := findSpan(analysis.Spans, mdppstudio.SpanStrong)
	if strong == nil || strong.From != 24 {
		t.Fatalf("strong span = %#v; want UTF-16 offset 24", strong)
	}
}

func TestAnalyzeSeverityMapping(t *testing.T) {
	if parseSeverity(mdpp.SeverityInfo) != mdppstudio.SeverityInfo || parseSeverity(mdpp.SeverityWarning) != mdppstudio.SeverityWarning || parseSeverity(mdpp.SeverityError) != mdppstudio.SeverityError {
		t.Fatal("mdpp parse severities do not map by name")
	}
	if lintSeverity(lint.SeverityError) != mdppstudio.SeverityError || lintSeverity(lint.SeverityWarning) != mdppstudio.SeverityWarning || lintSeverity(lint.SeverityInfo) != mdppstudio.SeverityInfo || lintSeverity(lint.SeverityHint) != mdppstudio.SeverityHint {
		t.Fatal("mdpp lint severities do not map by name")
	}
	a := Analyze("![ ](image.png)", Options{})
	foundWarning := false
	for _, d := range a.Diagnostics {
		if d.Severity == mdppstudio.SeverityWarning && d.Code == "mdpp/MD045" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected the image-alt warning, got %#v", a.Diagnostics)
	}
}

func TestAnalyzeHeadingsLinksImages(t *testing.T) {
	source := "# Heading One\n\n[site](https://example.test)\n\n![garden](https://example.test/a.png)\n\n<https://example.test/auto>"
	a := Analyze(source, Options{})
	if len(a.Headings) != 1 || a.Headings[0].Level != 1 || a.Headings[0].ID != "heading-one" {
		t.Fatalf("headings = %#v", a.Headings)
	}
	if len(a.Links) != 3 {
		t.Fatalf("links = %#v", a.Links)
	}
	if a.Links[0].Kind != "link" || a.Links[1].Kind != "image" || a.Links[1].Alt != "garden" || a.Links[2].Kind != "autolink" {
		t.Fatalf("link kinds = %#v", a.Links)
	}
	if a.Words < 4 || a.Engine == "" {
		t.Fatalf("analysis metadata = %#v", a)
	}
}

func TestAnalyzeRequiredHeadings(t *testing.T) {
	a := Analyze("# Present\n", Options{Required: []string{"Present", "Next update"}})
	if !hasCode(a.Diagnostics, "template-heading") {
		t.Fatalf("required heading missing diagnostic: %#v", a.Diagnostics)
	}
	for _, d := range a.Diagnostics {
		if d.Code == "template-heading" && d.Severity != mdppstudio.SeverityError {
			t.Fatalf("required heading severity = %q", d.Severity)
		}
	}
}

func TestAnalyzeTooLarge(t *testing.T) {
	a := Analyze("four", Options{MaxBytes: 3})
	if len(a.Diagnostics) != 1 || a.Diagnostics[0].Code != "too-large" || a.Diagnostics[0].Severity != mdppstudio.SeverityError || len(a.Spans) != 0 {
		t.Fatalf("too-large analysis = %#v", a)
	}
}

func TestFromMDPPMatchesAnalyze(t *testing.T) {
	source := "---\nmdpp: [\n---\n\n# Heading\n"
	doc, err := mdpp.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	converted := FromMDPP(source, doc.Diagnostics())
	a := Analyze(source, Options{})
	var parsed []mdppstudio.Diagnostic
	for _, d := range a.Diagnostics {
		if strings.HasPrefix(d.Code, "MDPP-PARSE-") {
			parsed = append(parsed, d)
		}
	}
	if !reflect.DeepEqual(parsed, converted) {
		t.Fatalf("parse diagnostics differ:\nAnalyze: %#v\nFromMDPP: %#v", parsed, converted)
	}
}

func TestAnalyzerMaxBytes(t *testing.T) {
	a := Analyzer(2)("too long", nil)
	if !hasCode(a.Diagnostics, "too-large") {
		t.Fatalf("Analyzer() = %#v", a)
	}
}

func findSpan(spans []mdppstudio.Span, kind mdppstudio.SpanKind) *mdppstudio.Span {
	for i := range spans {
		if spans[i].Kind == kind {
			return &spans[i]
		}
	}
	return nil
}
func hasCode(diags []mdppstudio.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func assertUTF16Range(t *testing.T, source string, from, to int) {
	t.Helper()
	units := utf16.Encode([]rune(source))
	if from < 0 || to < from || to > len(units) {
		t.Fatalf("range [%d,%d) outside %d UTF-16 units", from, to, len(units))
	}
	byteFrom, okFrom := byteIndexForUTF16(source, from)
	byteTo, okTo := byteIndexForUTF16(source, to)
	if !okFrom || !okTo {
		t.Fatalf("range [%d,%d) splits a UTF-16 surrogate pair", from, to)
	}
	if strings.ToValidUTF8(source[byteFrom:byteTo], "�") == "" && from != to {
		t.Fatalf("range [%d,%d) produced no source text", from, to)
	}
}

func byteIndexForUTF16(source string, index int) (int, bool) {
	if index == 0 {
		return 0, true
	}
	units := 0
	for byteAt, r := range source {
		if units == index {
			return byteAt, true
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units < index && index < units+width {
			return 0, false
		}
		units += width
		if units == index {
			return byteAt + len(string(r)), true
		}
	}
	return len(source), units == index
}
