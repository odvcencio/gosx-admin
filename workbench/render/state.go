package render

import "m31labs.dev/gosx"

// State is what a screen shows in place of, or before, its data.
type State int

const (
	// StateReady renders the data.
	StateReady State = iota
	// StateEmpty renders an empty block, not an error.
	StateEmpty
	// StateError renders an error block with retry and optional help links.
	StateError
	// StateLoading renders the fallback for deferred server content.
	StateLoading
)

// Link is a labeled URL.
type Link struct {
	// Label is the visible and accessible link text.
	Label string
	// Href is the link destination.
	Href string
}

// StateCopy is the text for a screen's non-ready states.
type StateCopy struct {
	// EmptyTitle defaults to "Nothing here yet.".
	EmptyTitle string
	EmptyBody  string
	// EmptyAction is omitted when Href is empty.
	EmptyAction Link
	// ErrorTitle defaults to "We could not load this.".
	ErrorTitle string
	// ErrorBody defaults to "Try again.".
	ErrorBody string
	// Retry defaults to label "Try again" and href "". An empty href reloads
	// the current URL and query.
	Retry Link
	// Help is omitted when Href is empty.
	Help Link
	// LoadingLabel defaults to "Loading".
	LoadingLabel string
}

// StateBlock renders one non-ready state and returns an empty node for
// StateReady. Empty renders a plain block because zero results are not an
// error. Error renders role="alert" with a retry link and optional help link.
// Loading renders role="status" with visible text and an aria-hidden progress
// element, for use as the fallback of server.PageState.Defer.
func StateBlock(state State, copy StateCopy) gosx.Node {
	switch state {
	case StateReady:
		return gosx.Fragment()
	case StateEmpty:
		title := copy.EmptyTitle
		if title == "" {
			title = "Nothing here yet."
		}
		nodes := []gosx.Node{gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-state__title")), gosx.Text(title))}
		if copy.EmptyBody != "" {
			nodes = append(nodes, gosx.El("p", nil, gosx.Text(copy.EmptyBody)))
		}
		if copy.EmptyAction.Href != "" {
			nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-state__actions")),
				gosx.El("a", gosx.Attrs(gosx.Attr("class", "gxa-button"), gosx.Attr("href", copy.EmptyAction.Href)), gosx.Text(copy.EmptyAction.Label))))
		}
		return gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-state gxa-state--empty")), gosx.Fragment(nodes...))
	case StateLoading:
		label := copy.LoadingLabel
		if label == "" {
			label = "Loading"
		}
		return gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-state gxa-state--loading"), gosx.Attr("role", "status")),
			gosx.El("progress", gosx.Attrs(gosx.Attr("class", "gxa-progress"), gosx.BoolAttr("aria-hidden"))),
			gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-state__title")), gosx.Text(label)))
	default:
		title := copy.ErrorTitle
		if title == "" {
			title = "We could not load this."
		}
		body := copy.ErrorBody
		if body == "" {
			body = "Try again."
		}
		retry := copy.Retry
		if retry.Label == "" {
			retry.Label = "Try again"
		}
		nodes := []gosx.Node{
			gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-state__title")), gosx.Text(title)),
			gosx.El("p", nil, gosx.Text(body)),
		}
		links := []gosx.Node{gosx.El("a", gosx.Attrs(gosx.Attr("class", "gxa-button"), gosx.Attr("href", retry.Href)), gosx.Text(retry.Label))}
		if copy.Help.Href != "" {
			label := copy.Help.Label
			if label == "" {
				label = "Get help"
			}
			links = append(links, gosx.El("a", gosx.Attrs(gosx.Attr("class", "gxa-link"), gosx.Attr("href", copy.Help.Href)), gosx.Text(label)))
		}
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-state__actions")), gosx.Fragment(links...)))
		return gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-state gxa-state--error"), gosx.Attr("role", "alert")), gosx.Fragment(nodes...))
	}
}
