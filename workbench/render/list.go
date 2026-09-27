package render

import (
	"context"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/server"
)

// Row is one record in a list.
type Row struct {
	// ID is the record identifier.
	ID string
	// Href links the row header cell to the record's detail page.
	Href string
	// Cells maps a column name to its display text.
	Cells map[string]string
	// Actions render in a final Actions cell. Actions the principal cannot
	// run are omitted.
	Actions []ActionForm
}

// Pager links to adjacent pages. Omitted when both links are empty.
type Pager struct {
	// Prev links to the previous page. Omitted when Href is empty.
	Prev Link
	// Next links to the next page. Omitted when Href is empty.
	Next Link
	// Label is visible page context, for example "Page 2 of 5".
	Label string
}

// ListProps configures List.
type ListProps struct {
	// Resource supplies the slug and Columns.
	Resource workbench.Resource
	Access   Access
	// Caption names the table. Default Resource.Label.
	Caption string
	Rows    []Row
	// Toolbar renders above the table, for example a GET filter form.
	Toolbar gosx.Node
	Pager   Pager
	State   State
	Copy    StateCopy
}

// List renders a resource table with a caption and explicit table, row, and
// cell roles. The first visible column is a row header, <th scope="row">,
// linked to Row.Href. Each cell has data-label for the theme's narrow layout.
// Hidden columns are omitted; redacted cells show "Hidden". A non-ready State,
// or no rows with StateReady, renders StateBlock instead of the table. The
// caller checks Access.CanRead before calling List.
func List(ctx context.Context, props ListProps) gosx.Node {
	if props.State != StateReady {
		nodes := []gosx.Node{StateBlock(props.State, props.Copy)}
		if !nodeEmpty(props.Toolbar) {
			nodes = append([]gosx.Node{props.Toolbar}, nodes...)
		}
		return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-list")), gosx.Fragment(nodes...))
	}
	if len(props.Rows) == 0 {
		nodes := []gosx.Node{StateBlock(StateEmpty, props.Copy)}
		if !nodeEmpty(props.Toolbar) {
			nodes = append([]gosx.Node{props.Toolbar}, nodes...)
		}
		return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-list")), gosx.Fragment(nodes...))
	}
	columns := make([]workbench.Column, 0, len(props.Resource.Columns))
	for _, column := range props.Resource.Columns {
		access := props.Access.Field(ctx, props.Resource.Slug, column.Name)
		if validFieldAccess(access) && access != FieldHidden {
			columns = append(columns, column)
		}
	}
	if len(columns) == 0 {
		columns = append(columns, workbench.Column{Name: "", Label: "Record"})
	}
	withActions := false
	for _, row := range props.Rows {
		for _, actionForm := range row.Actions {
			if props.Access.CanAct(ctx, actionForm.Resource, actionForm.Action.Name) {
				withActions = true
				break
			}
		}
	}
	caption := props.Caption
	if caption == "" {
		caption = props.Resource.Label
	}
	if caption == "" {
		caption = "Records"
	}
	headers := make([]gosx.Node, 0, len(columns)+1)
	for _, column := range columns {
		headers = append(headers, gosx.El("th", gosx.Attrs(gosx.Attr("role", "columnheader"), gosx.Attr("scope", "col")), gosx.Text(column.Label)))
	}
	if withActions {
		headers = append(headers, gosx.El("th", gosx.Attrs(gosx.Attr("role", "columnheader"), gosx.Attr("scope", "col")), gosx.Text("Actions")))
	}
	head := gosx.El("thead", gosx.Attrs(gosx.Attr("role", "rowgroup")),
		gosx.El("tr", gosx.Attrs(gosx.Attr("role", "row")), gosx.Fragment(headers...)))

	bodyRows := make([]gosx.Node, 0, len(props.Rows))
	for _, row := range props.Rows {
		cells := make([]gosx.Node, 0, len(columns)+1)
		for i, column := range columns {
			value := row.Cells[column.Name]
			if column.Name == "" {
				value = row.ID
			}
			if props.Access.Field(ctx, props.Resource.Slug, column.Name) == FieldRedacted {
				value = "Hidden"
			}
			cellAttrs := gosx.Attrs(gosx.Attr("role", "cell"), gosx.Attr("data-label", column.Label))
			if i == 0 {
				var contents gosx.Node
				if row.Href != "" {
					contents = server.Link(row.Href, gosx.Text(value))
				} else {
					contents = gosx.Text(value)
				}
				cells = append(cells, gosx.El("th", gosx.Attrs(gosx.Attr("role", "rowheader"), gosx.Attr("scope", "row"), gosx.Attr("data-label", column.Label)), contents))
				continue
			}
			cells = append(cells, gosx.El("td", cellAttrs, gosx.Text(value)))
		}
		if withActions {
			actions := make([]gosx.Node, 0, len(row.Actions))
			for _, actionForm := range row.Actions {
				node := Action(ctx, props.Access, actionForm)
				if gosx.RenderHTML(node) != "" {
					actions = append(actions, node)
				}
			}
			cells = append(cells, gosx.El("td", gosx.Attrs(gosx.Attr("role", "cell"), gosx.Attr("data-label", "Actions")), gosx.Fragment(actions...)))
		}
		bodyRows = append(bodyRows, gosx.El("tr", gosx.Attrs(gosx.Attr("role", "row")), gosx.Fragment(cells...)))
	}
	table := gosx.El("table", gosx.Attrs(gosx.Attr("class", "gxa-table"), gosx.Attr("role", "table")),
		gosx.El("caption", nil, gosx.Text(caption)),
		head,
		gosx.El("tbody", gosx.Attrs(gosx.Attr("role", "rowgroup")), gosx.Fragment(bodyRows...)),
	)
	nodes := []gosx.Node{table}
	if !nodeEmpty(props.Toolbar) {
		nodes = append([]gosx.Node{props.Toolbar}, nodes...)
	}
	if props.Pager.Prev.Href != "" || props.Pager.Next.Href != "" {
		label := props.Pager.Label
		links := make([]gosx.Node, 0, 2)
		if props.Pager.Prev.Href != "" {
			prevLabel := props.Pager.Prev.Label
			if prevLabel == "" {
				prevLabel = "Previous"
			}
			links = append(links, server.Link(props.Pager.Prev.Href, gosx.Attrs(gosx.Attr("class", "gxa-pager__link"), gosx.Attr("aria-label", prevLabel)), gosx.Text(prevLabel)))
		}
		if props.Pager.Next.Href != "" {
			nextLabel := props.Pager.Next.Label
			if nextLabel == "" {
				nextLabel = "Next"
			}
			links = append(links, server.Link(props.Pager.Next.Href, gosx.Attrs(gosx.Attr("class", "gxa-pager__link"), gosx.Attr("aria-label", nextLabel)), gosx.Text(nextLabel)))
		}
		if label != "" {
			pageLabel := gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-pager__label")), gosx.Text(label))
			if len(links) > 1 {
				links = append(links[:1], append([]gosx.Node{pageLabel}, links[1:]...)...)
			} else {
				links = append(links, pageLabel)
			}
		}
		nodes = append(nodes, gosx.El("nav", gosx.Attrs(gosx.Attr("class", "gxa-pager"), gosx.Attr("aria-label", "Pagination")), gosx.Fragment(links...)))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-list")), gosx.Fragment(nodes...))
}
