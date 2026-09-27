package render

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const (
	// CSRFField is the field session.Manager.Protect reads.
	CSRFField = "csrf_token"
	// KeyField carries the idempotency key.
	KeyField = "gxa_key"
	// RevisionField carries the revision the operator saw. "0" creates.
	RevisionField = "gxa_revision"
	// RecordField carries the record id. Empty creates.
	RecordField = "gxa_record"
	// ConfirmField is the confirmation checkbox. Its checked value is "yes".
	ConfirmField      = "gxa_confirm"
	returnTargetField = action.ReturnTargetField
)

// CSRFToken returns the session CSRF token for r. It returns "" when r has
// no session.
func CSRFToken(r *http.Request) string { return session.Token(r) }

// NewKey returns a fresh lower-case 32-character idempotency key.
func NewKey() string {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic("render: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(key[:])
}

// ValidKey reports whether key is exactly 32 characters of [0-9a-f].
func ValidKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, c := range key {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Confirmation is the confirmation step of an action marked
// workbench.Action.Confirm.
type Confirmation struct {
	// Summary names the exact target, amount, time, or count.
	Summary string
	// Consequence says what happens next. Defaults to the action Description.
	Consequence string
	// Check labels the required checkbox. Default "I checked the details.".
	Check string
	// Open renders the disclosure expanded.
	Open bool
}

// ActionForm describes one action form.
type ActionForm struct {
	// Resource is the workbench slug.
	Resource string
	Action   workbench.Action
	// URL is the POST target.
	URL string
	// CSRF is CSRFToken(r).
	CSRF string
	// Key is NewKey(), fresh per page view.
	Key string
	// RecordID is empty for a create action.
	RecordID string
	// Revision is the revision the operator sees. Zero creates.
	Revision int64
	// ReturnTo is the same-origin page to return to.
	ReturnTo string
	// Confirm supplies confirmation text for a Confirm action. Nil uses
	// Confirmation defaults.
	Confirm *Confirmation
	// Submit is the button label. Defaults to Action.Label.
	Submit string
	// ID prefixes element ids. Defaults to the resource, action, and record
	// identifiers with characters outside [A-Za-z0-9_-] replaced by hyphens.
	ID string
	// Hidden adds hidden inputs, for example a filter to preserve.
	Hidden map[string]string
}

// Action renders f as a standalone form. It returns an empty node when access
// cannot run the action. A plain action renders a native POST form with the
// hidden contract inputs and a submit button. A Confirm action renders a
// native <details> disclosure: its summary is the first step, and its form
// shows the exact target, consequence, and required gxa_confirm=yes checkbox.
// Confirmation.Open renders the disclosure open. The form works without
// JavaScript; Guard rejects a request that omits the checkbox.
func Action(ctx context.Context, access Access, f ActionForm) gosx.Node {
	if !access.CanAct(ctx, f.Resource, f.Action.Name) {
		return gosx.Fragment()
	}
	label := f.Submit
	if label == "" {
		label = f.Action.Label
	}
	if label == "" {
		label = "Save"
	}
	buttonClass := "gxa-button"
	if f.Action.Confirm {
		buttonClass += " gxa-button--danger"
	}
	button := gosx.El("button", gosx.Attrs(gosx.Attr("class", buttonClass), gosx.Attr("type", "submit")), gosx.Text(label))
	hidden := actionHiddenInputs(f)
	if !f.Action.Confirm {
		nodes := append(hidden, button)
		return server.Form(gosx.Attrs(gosx.Attr("class", "gxa-action"), gosx.Attr("method", "post"), gosx.Attr("action", f.URL)), gosx.Fragment(nodes...))
	}
	confirmation := Confirmation{}
	if f.Confirm != nil {
		confirmation = *f.Confirm
	}
	consequence := confirmation.Consequence
	if consequence == "" {
		consequence = f.Action.Description
	}
	check := confirmation.Check
	if check == "" {
		check = "I checked the details."
	}
	id := actionID(f) + "-confirm"
	formChildren := append([]gosx.Node{}, hidden...)
	if confirmation.Summary != "" {
		formChildren = append(formChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-confirm__summary")), gosx.Text(confirmation.Summary)))
	}
	if consequence != "" {
		formChildren = append(formChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-confirm__consequence")), gosx.Text(consequence)))
	}
	formChildren = append(formChildren,
		gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-check")),
			gosx.El("input", gosx.Attrs(gosx.Attr("id", id), gosx.Attr("name", ConfirmField), gosx.Attr("type", "checkbox"), gosx.Attr("value", "yes"), gosx.BoolAttr("required"))),
			gosx.El("label", gosx.Attrs(gosx.Attr("for", id)), gosx.Text(check))),
		button,
	)
	form := server.Form(gosx.Attrs(gosx.Attr("class", "gxa-confirm__form"), gosx.Attr("method", "post"), gosx.Attr("action", f.URL)), gosx.Fragment(formChildren...))
	detailsAttrs := gosx.Attrs(gosx.Attr("class", "gxa-confirm"))
	if confirmation.Open {
		detailsAttrs = append(detailsAttrs, gosx.BoolAttr("open"))
	}
	return gosx.El("details", detailsAttrs,
		gosx.El("summary", gosx.Attrs(gosx.Attr("class", "gxa-button gxa-button--secondary")), gosx.Text(label)),
		form,
	)
}

func hiddenInput(name, value string) gosx.Node {
	return gosx.El("input", gosx.Attrs(gosx.Attr("type", "hidden"), gosx.Attr("name", name), gosx.Attr("value", value)))
}
