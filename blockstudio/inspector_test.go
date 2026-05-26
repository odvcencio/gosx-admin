package blockstudio

import (
	"strings"
	"testing"

	"m31labs.dev/gosx"
)

func TestRenderInspectorFields(t *testing.T) {
	definition := Definition{
		Key:   "hero",
		Label: "Hero",
		Fields: []FieldDefinition{
			{Name: "headline", Label: "Headline", Kind: FieldText, Required: true, Placeholder: "Opening line"},
			{Name: "body", Label: "Body", Kind: FieldTextarea},
			{Name: "image", Label: "Image", Kind: FieldImage},
			{Name: "visible", Label: "Visible", Kind: FieldBoolean},
			{Name: "tone", Label: "Tone", Kind: FieldSelect, Options: []FieldOption{{Value: "warm", Label: "Warm"}, {Value: "plain", Label: "Plain"}}},
		},
	}
	node := RenderInspectorFields(BlockInstance{Key: "hero", Values: Values{
		"headline": {Kind: FieldText, String: "Forest school"},
		"visible":  {Kind: FieldBoolean, Bool: true},
		"tone":     {Kind: FieldSelect, String: "warm"},
		"image":    {Kind: FieldImage, Media: &MediaValue{URL: "/media/forest.jpg"}},
	}}, definition, InspectorOptions{Prefix: "home", Index: 2})
	html := gosx.RenderHTML(node)
	for _, want := range []string{
		`class="block-inspector"`,
		`name="homeBlockField2_headline"`,
		`required`,
		`placeholder="Opening line"`,
		`<textarea`,
		`name="homeBlockField2_image.alt"`,
		`checked`,
		`<select`,
		`selected`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in inspector html: %s", want, html)
		}
	}
}

func TestRenderInspectorFieldsAllowsCustomClassAndDisabled(t *testing.T) {
	node := RenderInspectorFields(BlockInstance{Key: "cta"}, Definition{
		Key: "cta",
		Fields: []FieldDefinition{
			{Name: "href", Label: "URL", Kind: FieldURL},
		},
	}, InspectorOptions{Prefix: "body", Class: "custom-inspector", Disabled: true})
	html := gosx.RenderHTML(node)
	if !strings.Contains(html, `class="custom-inspector"`) || !strings.Contains(html, `disabled`) {
		t.Fatalf("expected custom class and disabled attr: %s", html)
	}
}
