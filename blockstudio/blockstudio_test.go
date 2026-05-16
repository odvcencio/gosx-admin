package blockstudio

import "testing"

func TestNormalizeOrdersAndDefaultsBlocks(t *testing.T) {
	catalog := []Definition{
		{Key: "hero", Label: "Hero", DefaultOn: true},
		{Key: "products", Label: "Products", DefaultOn: true},
		{Key: "blog", Label: "Blog", DefaultOn: false},
	}
	blocks := Normalize([]BlockInput{
		{Key: "blog", Enabled: true, Order: 1},
		{Key: "hero", Enabled: false, Order: 2},
		{Key: "missing", Enabled: true, Order: 3},
	}, catalog)
	if len(blocks) != 3 {
		t.Fatalf("expected all catalog blocks, got %#v", blocks)
	}
	if blocks[0].Key != "blog" || !blocks[0].Enabled {
		t.Fatalf("expected blog first and enabled, got %#v", blocks)
	}
	if blocks[1].Key != "hero" || blocks[1].Enabled {
		t.Fatalf("expected hero second and disabled, got %#v", blocks)
	}
	if blocks[2].Key != "products" || !blocks[2].Enabled {
		t.Fatalf("expected missing products to keep default enabled, got %#v", blocks)
	}
}

func TestFormMapsUsePrefixAndLabels(t *testing.T) {
	forms := FormMaps([]Block{{Key: "hero", Enabled: true}}, []Definition{{Key: "hero", Label: "Hero", Summary: "Lead section", DefaultOn: true}}, FormOptions{Prefix: "homeSection"})
	if len(forms) != 1 {
		t.Fatalf("expected one form block, got %#v", forms)
	}
	if forms[0]["keyName"] != "homeSectionKey0" || forms[0]["enabledName"] != "homeSectionEnabled0" || forms[0]["orderName"] != "homeSectionOrder0" {
		t.Fatalf("unexpected field names: %#v", forms[0])
	}
	if forms[0]["hasSummary"] != true || forms[0]["dragLabel"] != "Reorder Hero" {
		t.Fatalf("unexpected form metadata: %#v", forms[0])
	}
}

func TestNormalizeDocumentOrdersDefaultsAndDropsUnknown(t *testing.T) {
	catalog := []Definition{
		{Key: "hero", Label: "Hero", DefaultOn: true},
		{Key: "products", Label: "Products", DefaultOn: true},
		{Key: "blog", Label: "Blog", DefaultOn: false},
	}
	doc := NormalizeDocument(Document{Blocks: []BlockInstance{
		{Key: "blog", Enabled: true, Order: 1},
		{Key: "hero", Enabled: false, Order: 2},
		{Key: "missing", Enabled: true, Order: 3},
	}}, catalog)
	if doc.Version != 1 {
		t.Fatalf("expected default version, got %#v", doc)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("expected all catalog blocks, got %#v", doc.Blocks)
	}
	if doc.Blocks[0].Key != "blog" || !doc.Blocks[0].Enabled || doc.Blocks[0].Order != 1 {
		t.Fatalf("expected blog first and enabled, got %#v", doc.Blocks)
	}
	if doc.Blocks[1].Key != "hero" || doc.Blocks[1].Enabled || doc.Blocks[1].Order != 2 {
		t.Fatalf("expected hero second and disabled, got %#v", doc.Blocks)
	}
	if doc.Blocks[2].Key != "products" || !doc.Blocks[2].Enabled || doc.Blocks[2].Order != 3 {
		t.Fatalf("expected missing products to keep default enabled, got %#v", doc.Blocks)
	}
}

func TestNormalizeDocumentRepeatableAllowsDuplicateKeys(t *testing.T) {
	catalog := []Definition{
		{Key: "hero", Label: "Hero", DefaultOn: true},
		{Key: "cta", Label: "CTA", Repeatable: true},
	}
	doc := NormalizeDocument(Document{Blocks: []BlockInstance{
		{Key: "cta", Enabled: true, Order: 2},
		{Key: "cta", Enabled: false, Order: 1},
		{Key: "hero", Enabled: true, Order: 3},
	}}, catalog)
	if len(doc.Blocks) != 3 {
		t.Fatalf("expected repeatable duplicate to survive, got %#v", doc.Blocks)
	}
	if doc.Blocks[0].Key != "cta" || doc.Blocks[1].Key != "cta" || doc.Blocks[0].ID == doc.Blocks[1].ID {
		t.Fatalf("expected duplicate cta blocks with distinct IDs, got %#v", doc.Blocks)
	}

	doc = NormalizeDocument(Document{Blocks: []BlockInstance{
		{Key: "hero", Enabled: true, Order: 1},
		{Key: "hero", Enabled: false, Order: 2},
	}}, catalog)
	if len(doc.Blocks) != 2 || doc.Blocks[0].Key != "hero" || doc.Blocks[1].Key != "cta" {
		t.Fatalf("expected non-repeatable duplicate to collapse and missing cta to be added, got %#v", doc.Blocks)
	}
}

func TestDocumentBlockCompatibilityRoundTrip(t *testing.T) {
	blocks := []Block{{Key: "hero", Enabled: true}, {Key: "products", Enabled: false}}
	doc := DocumentFromBlocks(blocks, DocumentOptions{Kind: "home"})
	if doc.Kind != "home" || doc.Version != 1 || len(doc.Blocks) != 2 {
		t.Fatalf("unexpected document: %#v", doc)
	}
	roundTrip := BlocksFromDocument(doc)
	if len(roundTrip) != len(blocks) || roundTrip[1].Key != "products" || roundTrip[1].Enabled {
		t.Fatalf("unexpected block round trip: %#v", roundTrip)
	}
	inputs := InputsFromDocument(doc, FormOptions{})
	if len(inputs) != 2 || inputs[1].Order != 2 {
		t.Fatalf("unexpected inputs: %#v", inputs)
	}
}

func TestParseDocumentFormAcceptsLegacyMuddyNames(t *testing.T) {
	catalog := []Definition{
		{Key: "hero", Label: "Hero", DefaultOn: true},
		{Key: "products", Label: "Products", DefaultOn: true},
	}
	doc, errs := ParseDocumentForm(map[string]string{
		"homeSectionKey0":     "products",
		"homeSectionEnabled0": "on",
		"homeSectionOrder0":   "1",
		"homeSectionKey1":     "hero",
		"homeSectionEnabled1": "off",
		"homeSectionOrder1":   "2",
	}, catalog, FormOptions{Prefix: "homeSection"})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	if len(doc.Blocks) != 2 || doc.Blocks[0].Key != "products" || !doc.Blocks[0].Enabled || doc.Blocks[1].Key != "hero" || doc.Blocks[1].Enabled {
		t.Fatalf("unexpected parsed document: %#v", doc.Blocks)
	}
}

func TestParseDocumentFormParsesFieldsAndValidation(t *testing.T) {
	catalog := []Definition{{
		Key:   "hero",
		Label: "Hero",
		Fields: []FieldDefinition{
			{Name: "headline", Label: "Headline", Kind: FieldText, Required: true},
			{Name: "tone", Label: "Tone", Kind: FieldSelect, Options: []FieldOption{{Value: "quiet", Label: "Quiet"}}},
			{Name: "image", Label: "Image", Kind: FieldImage},
		},
	}}
	doc, errs := ParseDocumentForm(map[string]string{
		"homeBlockCount":            "1",
		"homeBlockKey0":             "hero",
		"homeBlockEnabled0":         "on",
		"homeBlockOrder0":           "1",
		"homeBlockField0_tone":      "loud",
		"homeBlockField0_image":     "/media/hero.jpg",
		"homeBlockField0_image.alt": "Hero image",
	}, catalog, FormOptions{Prefix: "home"})
	if errs["blocks[0].values.headline"] == "" || errs["blocks[0].values.tone"] == "" {
		t.Fatalf("expected required and select errors, got %#v", errs)
	}
	image := doc.Blocks[0].Values["image"].Media
	if image == nil || image.URL != "/media/hero.jpg" || image.Alt != "Hero image" {
		t.Fatalf("unexpected image value: %#v", doc.Blocks[0].Values["image"])
	}
}

func TestFormDocumentUsesDocumentFieldNames(t *testing.T) {
	catalog := []Definition{{
		Key:   "hero",
		Label: "Hero",
		Fields: []FieldDefinition{
			{Name: "headline", Label: "Headline", Kind: FieldText},
		},
	}}
	form := FormDocument(Document{}, catalog, FormOptions{Prefix: "home"})
	if form.CountName != "homeBlockCount" || len(form.Blocks) != 1 {
		t.Fatalf("unexpected form document: %#v", form)
	}
	block := form.Blocks[0]
	if block.KeyName != "homeBlockKey0" || block.EnabledName != "homeBlockEnabled0" || block.OrderName != "homeBlockOrder0" {
		t.Fatalf("unexpected block names: %#v", block)
	}
	if len(block.Fields) != 1 || block.Fields[0].InputName != "homeBlockField0_headline" {
		t.Fatalf("unexpected field names: %#v", block.Fields)
	}
}
