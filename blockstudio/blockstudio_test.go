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
