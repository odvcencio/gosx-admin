package render

import (
	"context"
	"encoding/json"
	"net/http"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/session"
)

const conflictFlashKey = "gxa.conflict"

// Conflict is the state behind the 409 view.
type Conflict struct {
	// Revision is the current revision. The next attempt submits it.
	Revision int64
	// Current maps field names to saved display values.
	Current map[string]string
	// Entered maps field names to the values submitted by the operator.
	Entered map[string]string
}

// Feedback is the outcome of an action, read on the page after it.
type Feedback struct {
	// Action is the action name.
	Action string
	// Status is the action HTTP status.
	Status  int
	OK      bool
	Message string
	// Values are submitted values after a conflict, validation error, or
	// dependency failure.
	Values map[string]string
	// Errors are field errors after validation.
	Errors map[string]string
	// Conflict is set after a 409 response.
	Conflict *Conflict
}

// ReadFeedback returns the flashed outcome of the named action for r. A native
// submission reads action.State; a managed conflict reads the session flash
// written by Guard. It returns false when the action has no outcome.
func ReadFeedback(r *http.Request, name string) (Feedback, bool) {
	if view, ok := action.State(r, name); ok {
		feedback := Feedback{
			Action:  name,
			Status:  view.Status,
			OK:      view.Result.OK,
			Message: view.Result.Message,
			Values:  safeValues(view.Result.Values),
			Errors:  view.Result.FieldErrors,
		}
		if view.Status == http.StatusConflict && len(view.Result.Data) > 0 {
			var data struct {
				Revision int64             `json:"revision"`
				Current  map[string]string `json:"current"`
			}
			if json.Unmarshal(view.Result.Data, &data) == nil {
				feedback.Conflict = &Conflict{Revision: data.Revision, Current: data.Current, Entered: feedback.Values}
			}
		}
		return feedback, true
	}
	for _, raw := range session.FlashValues(r)[conflictFlashKey] {
		data, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var feedback Feedback
		if json.Unmarshal(data, &feedback) != nil || feedback.Action != name || feedback.Conflict == nil {
			continue
		}
		feedback.Values = safeValues(feedback.Values)
		feedback.Conflict.Entered = safeValues(feedback.Conflict.Entered)
		return feedback, true
	}
	return Feedback{}, false
}

// ConflictView renders a 409 panel for fields that differ between
// conflict.Current and conflict.Entered, skipping fields the principal cannot
// see. A zero or out-of-range headingLevel uses 2. Form calls it; consumers
// can use it on custom screens.
func ConflictView(ctx context.Context, access Access, resource workbench.Resource, conflict Conflict, id string, headingLevel int) gosx.Node {
	if headingLevel < 1 || headingLevel > 6 {
		headingLevel = 2
	}
	items := make([]gosx.Node, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		if access.Field(ctx, resource.Slug, field.Name) != FieldVisible {
			continue
		}
		current, entered := conflict.Current[field.Name], conflict.Entered[field.Name]
		if current == entered {
			continue
		}
		items = append(items, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-conflict__item")),
			gosx.El("dt", nil, gosx.Text(field.Label)),
			gosx.El("dd", nil,
				gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-conflict__tag")), gosx.Text("Saved now:")),
				gosx.Text(" "+current)),
			gosx.El("dd", nil,
				gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-conflict__tag")), gosx.Text("You entered:")),
				gosx.Text(" "+entered))))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-conflict"), gosx.Attr("role", "alert"), gosx.Attr("tabindex", "-1"), gosx.BoolAttr("autofocus"), gosx.Attr("aria-labelledby", id+"-title")),
		heading(headingLevel, id+"-title", "This record changed"),
		gosx.El("p", nil, gosx.Text("Someone saved a change after you opened this form. Your entries are still in the form. Check the saved values, then save again.")),
		gosx.El("dl", gosx.Attrs(gosx.Attr("class", "gxa-conflict__list")), gosx.Fragment(items...)),
	)
}

func safeValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if key == CSRFField || key == KeyField || key == returnTargetField {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
