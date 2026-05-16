package workbench

import "testing"

func TestViewMapsResourcesAndTools(t *testing.T) {
	view := View(Workspace{
		Resources: []Resource{{
			Slug:         "products",
			Label:        "Products",
			Singular:     "Product",
			Description:  "Inventory.",
			Route:        "/admin/products",
			Count:        2,
			Mutable:      true,
			Generated:    false,
			Capabilities: []string{"list", "create"},
			Columns: []Column{
				{Name: "title", Label: "Title", Kind: FieldText},
			},
			Fields: []Field{
				{Name: "title", Label: "Title", Kind: FieldText, Required: true},
				{Name: "status", Label: "Status", Kind: FieldSelect, Options: []string{"draft", "published"}},
			},
			Actions: []Action{
				{Name: "save", Label: "Save", Kind: "form"},
			},
		}},
		Tools: []Tool{{
			Slug:        "graphql",
			Label:       "GraphQL",
			Description: "Headless API.",
			Route:       "/api/graphql",
			Kind:        "api",
		}},
	})
	resources := view["resources"].([]map[string]any)
	if len(resources) != 1 {
		t.Fatalf("expected resource view, got %#v", view)
	}
	resource := resources[0]
	if resource["countLabel"] != "2 records" || resource["mutableLabel"] != "Writable" || resource["generatedLabel"] != "Bespoke surface" {
		t.Fatalf("unexpected resource labels: %#v", resource)
	}
	fields := resource["fields"].([]map[string]any)
	if fields[0]["requiredLabel"] != "Required" || fields[1]["hasOptions"] != true {
		t.Fatalf("unexpected field views: %#v", fields)
	}
	tools := view["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["hasActions"] != false {
		t.Fatalf("unexpected tool views: %#v", tools)
	}
}

func TestReadOnlyAndGeneratedLabels(t *testing.T) {
	resource := ResourceView(Resource{
		Slug:      "contacts",
		Label:     "Contacts",
		Count:     1,
		Generated: true,
		Fields: []Field{
			{Name: "email", Label: "Email", Kind: FieldText, ReadOnly: true},
		},
	})
	if resource["mutableLabel"] != "Read-only" || resource["generatedLabel"] != "Generated surface" {
		t.Fatalf("unexpected resource labels: %#v", resource)
	}
	fields := resource["fields"].([]map[string]any)
	if fields[0]["readOnlyLabel"] != "Read-only" || fields[0]["requiredLabel"] != "Optional" {
		t.Fatalf("unexpected field labels: %#v", fields)
	}
}
