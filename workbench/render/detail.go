package render

import (
	"context"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
)

// Record is one resource record prepared for display.
type Record struct {
	// ID is the record identifier.
	ID string
	// Revision is the current record revision.
	Revision int64
	// Values maps a field name to its display text.
	Values map[string]string
}

// DetailProps configures Detail.
type DetailProps struct {
	// Resource supplies the slug and Fields.
	Resource workbench.Resource
	Access   Access
	Record   Record
	// Actions render after the field list. Actions the principal cannot run
	// are omitted.
	Actions []ActionForm
	State   State
	Copy    StateCopy
}

// Detail renders a record as a description list in Resource.Fields order,
// with one <div class="gxa-detail__item"><dt>Label</dt><dd>Value</dd></div>
// per visible field. Hidden fields are omitted; redacted fields show "Hidden".
// Actions the principal cannot run are omitted after the field list. A
// non-ready State renders StateBlock instead.
func Detail(ctx context.Context, props DetailProps) gosx.Node {
	if props.State != StateReady {
		return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-detail")), StateBlock(props.State, props.Copy))
	}
	items := make([]gosx.Node, 0, len(props.Resource.Fields))
	for _, field := range props.Resource.Fields {
		access := props.Access.Field(ctx, props.Resource.Slug, field.Name)
		if !validFieldAccess(access) || access == FieldHidden {
			continue
		}
		value := props.Record.Values[field.Name]
		if access == FieldRedacted {
			value = "Hidden"
		}
		items = append(items, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-detail__item")),
			gosx.El("dt", nil, gosx.Text(field.Label)),
			gosx.El("dd", nil, gosx.Text(value))))
	}
	actions := make([]gosx.Node, 0, len(props.Actions))
	for _, actionForm := range props.Actions {
		if node := Action(ctx, props.Access, actionForm); gosx.RenderHTML(node) != "" {
			actions = append(actions, node)
		}
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-detail")),
		gosx.El("dl", gosx.Attrs(gosx.Attr("class", "gxa-detail__list")), gosx.Fragment(items...)),
		gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-actions")), gosx.Fragment(actions...)),
	)
}
