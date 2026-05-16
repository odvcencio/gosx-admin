package blockstudio

import (
	"strings"

	"github.com/odvcencio/gosx"
)

type InspectorOptions struct {
	Prefix   string
	Index    int
	Class    string
	Disabled bool
}

func RenderInspectorFields(instance BlockInstance, definition Definition, options InspectorOptions) gosx.Node {
	definitions := normalizedDefinitions([]Definition{definition})
	if len(definitions) == 0 {
		return gosx.Fragment()
	}
	definition = definitions[0]
	instance.Values = normalizeValues(instance.Values, definition)
	fields := make([]gosx.Node, 0, len(definition.Fields))
	for _, field := range definition.Fields {
		fields = append(fields, renderInspectorField(formField(instance, field, options.Index, FormOptions{Prefix: options.Prefix}), options))
	}
	className := strings.TrimSpace(options.Class)
	if className == "" {
		className = "block-inspector"
	}
	return gosx.El("div", gosx.Attrs(gosx.Attr("class", className)), gosx.Fragment(fields...))
}

func renderInspectorField(field FormField, options InspectorOptions) gosx.Node {
	children := []gosx.Node{
		gosx.El("span", gosx.Attrs(gosx.Attr("class", "block-inspector__label")), gosx.Text(field.Label)),
	}
	if strings.TrimSpace(field.Help) != "" {
		children = append(children, gosx.El("span", gosx.Attrs(gosx.Attr("class", "block-inspector__help")), gosx.Text(field.Help)))
	}
	children = append(children, inspectorControl(field, options))
	return gosx.El("label", gosx.Attrs(
		gosx.Attr("class", "block-inspector__field block-inspector__field--"+string(field.Kind)),
		gosx.Attr("data-field-name", field.Name),
	), gosx.Fragment(children...))
}

func inspectorControl(field FormField, options InspectorOptions) gosx.Node {
	switch field.Kind {
	case FieldTextarea:
		return gosx.El("textarea", inspectorAttrs(field, options, gosx.Attr("rows", "4")), gosx.Text(field.Value))
	case FieldBoolean:
		return gosx.El("input", inspectorAttrs(field, options,
			gosx.Attr("type", "checkbox"),
			gosx.Attr("value", "on"),
			gosx.Attr("checked", field.Checked != ""),
		))
	case FieldSelect:
		optionsNodes := make([]gosx.Node, 0, len(field.Options))
		for _, option := range field.Options {
			optionsNodes = append(optionsNodes, gosx.El("option", gosx.Attrs(
				gosx.Attr("value", option.Value),
				gosx.Attr("selected", option.Selected != ""),
			), gosx.Text(option.Label)))
		}
		return gosx.El("select", inspectorAttrs(field, options), gosx.Fragment(optionsNodes...))
	case FieldImage:
		nodes := []gosx.Node{
			gosx.El("input", inspectorAttrs(field, options, gosx.Attr("type", "url"))),
		}
		if field.AltName != "" {
			nodes = append(nodes, gosx.El("input", gosx.Attrs(
				gosx.Attr("type", "text"),
				gosx.Attr("name", field.AltName),
				gosx.Attr("placeholder", "Alt text"),
				gosx.Attr("disabled", options.Disabled),
			)))
		}
		return gosx.El("div", gosx.Attrs(gosx.Attr("class", "block-inspector__media")), gosx.Fragment(nodes...))
	case FieldURL:
		return gosx.El("input", inspectorAttrs(field, options, gosx.Attr("type", "url")))
	default:
		return gosx.El("input", inspectorAttrs(field, options, gosx.Attr("type", "text")))
	}
}

func inspectorAttrs(field FormField, options InspectorOptions, extra ...any) gosx.AttrList {
	pairs := []any{
		gosx.Attr("name", field.InputName),
		gosx.Attr("value", field.Value),
		gosx.Attr("placeholder", field.Placeholder),
		gosx.Attr("required", field.Required),
		gosx.Attr("disabled", options.Disabled),
	}
	pairs = append(pairs, extra...)
	return gosx.Attrs(pairs...)
}
