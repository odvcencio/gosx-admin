package workbench

import "fmt"

func View(workspace Workspace) map[string]any {
	resources := make([]map[string]any, 0, len(workspace.Resources))
	for _, resource := range workspace.Resources {
		resources = append(resources, ResourceView(resource))
	}
	tools := make([]map[string]any, 0, len(workspace.Tools))
	for _, tool := range workspace.Tools {
		tools = append(tools, ToolView(tool))
	}
	return map[string]any{
		"resources": resources,
		"tools":     tools,
	}
}

func ResourceView(resource Resource) map[string]any {
	return map[string]any{
		"slug":           resource.Slug,
		"label":          resource.Label,
		"singular":       resource.Singular,
		"description":    resource.Description,
		"route":          resource.Route,
		"count":          resource.Count,
		"countLabel":     fmt.Sprintf("%d records", resource.Count),
		"mutable":        resource.Mutable,
		"mutableLabel":   mutableLabel(resource.Mutable),
		"generated":      resource.Generated,
		"generatedLabel": generatedLabel(resource.Generated),
		"capabilities":   StringsView(resource.Capabilities),
		"columns":        ColumnViews(resource.Columns),
		"fields":         FieldViews(resource.Fields),
		"actions":        ActionViews(resource.Actions),
		"hasActions":     len(resource.Actions) > 0,
	}
}

func ToolView(tool Tool) map[string]any {
	return map[string]any{
		"slug":        tool.Slug,
		"label":       tool.Label,
		"description": tool.Description,
		"route":       tool.Route,
		"kind":        tool.Kind,
		"actions":     ActionViews(tool.Actions),
		"hasActions":  len(tool.Actions) > 0,
	}
}

func FieldViews(fields []Field) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		out = append(out, map[string]any{
			"name":          field.Name,
			"label":         field.Label,
			"kind":          string(field.Kind),
			"required":      field.Required,
			"requiredLabel": requiredLabel(field.Required),
			"readOnly":      field.ReadOnly,
			"readOnlyLabel": readOnlyLabel(field.ReadOnly),
			"options":       StringsView(field.Options),
			"hasOptions":    len(field.Options) > 0,
		})
	}
	return out
}

func ColumnViews(columns []Column) []map[string]any {
	out := make([]map[string]any, 0, len(columns))
	for _, column := range columns {
		out = append(out, map[string]any{
			"name":  column.Name,
			"label": column.Label,
			"kind":  string(column.Kind),
		})
	}
	return out
}

func ActionViews(actions []Action) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		out = append(out, map[string]any{
			"name":        action.Name,
			"label":       action.Label,
			"description": action.Description,
			"kind":        action.Kind,
		})
	}
	return out
}

func StringsView(values []string) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{"value": value})
	}
	return out
}

func mutableLabel(ok bool) string {
	if ok {
		return "Writable"
	}
	return "Read-only"
}

func generatedLabel(ok bool) string {
	if ok {
		return "Generated surface"
	}
	return "Bespoke surface"
}

func requiredLabel(ok bool) string {
	if ok {
		return "Required"
	}
	return "Optional"
}

func readOnlyLabel(ok bool) string {
	if ok {
		return "Read-only"
	}
	return "Editable"
}
