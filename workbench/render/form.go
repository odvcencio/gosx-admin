package render

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/server"
)

// FormProps configures Form.
type FormProps struct {
	// Resource supplies the slug and default Fields.
	Resource workbench.Resource
	Access   Access
	// Fields are the fields to edit. Defaults to Resource.Fields.
	Fields []workbench.Field
	// Action supplies the POST URL, CSRF token, key, record, and revision.
	Action ActionForm
	// Values are saved values keyed by field name.
	Values map[string]string
	// Feedback comes from ReadFeedback. Its Values replace Values, its Errors
	// mark fields, and its Conflict renders the conflict view and revision.
	Feedback Feedback
	State    State
	Copy     StateCopy
	// HeadingLevel is the error summary and conflict heading level. Default 2.
	HeadingLevel int
}

// Form renders a GoSX managed form that also works as a native POST. Its
// hidden inputs appear in this order: csrf_token, gxa_key, gxa_record,
// gxa_revision, __gosx_return_to, then Action.Hidden sorted by name. Controls
// use the matching HTML control for each FieldKind. Read-only fields render
// read-only; redacted fields show "Hidden" with no control; hidden fields are
// omitted. Help and field-error ids appear in aria-describedby in that order.
// A field error adds an error summary that links to its control. A conflict
// renders ConflictView before the fields and submits the conflict revision.
//
// Text and slug use text inputs; textarea uses a textarea; money uses a text
// input with decimal input mode; boolean uses a true-valued checkbox; select
// and relation use a select with a leading empty option; datetime uses a
// datetime-local input; image uses a URL input. The control id is the action
// id followed by a hyphen and the field name.
func Form(ctx context.Context, props FormProps) gosx.Node {
	if props.State != StateReady {
		return StateBlock(props.State, props.Copy)
	}
	fields := props.Fields
	if fields == nil {
		fields = props.Resource.Fields
	}
	fieldID := actionID(props.Action)
	values := props.Values
	if props.Feedback.Values != nil {
		values = props.Feedback.Values
	}
	if values == nil {
		values = map[string]string{}
	}
	if props.Feedback.Conflict != nil {
		props.Action.Revision = props.Feedback.Conflict.Revision
	}
	level := props.HeadingLevel
	if level < 1 || level > 6 {
		level = 2
	}
	nodes := make([]gosx.Node, 0, len(fields)+8)
	if len(props.Feedback.Errors) > 0 {
		items := make([]gosx.Node, 0, len(props.Feedback.Errors))
		for _, field := range fields {
			if props.Access.Field(ctx, props.Resource.Slug, field.Name) != FieldVisible || field.ReadOnly {
				continue
			}
			if message := props.Feedback.Errors[field.Name]; message != "" {
				items = append(items, gosx.El("li", nil,
					gosx.El("a", gosx.Attrs(gosx.Attr("href", "#"+fieldID+"-"+field.Name)), gosx.Text(message))))
			}
		}
		if props.Action.Action.Confirm && props.Feedback.Errors[ConfirmField] != "" {
			items = append(items, gosx.El("li", nil,
				gosx.El("a", gosx.Attrs(gosx.Attr("href", "#"+fieldID+"-confirm")), gosx.Text(props.Feedback.Errors[ConfirmField]))))
		}
		if len(items) > 0 {
			nodes = append(nodes, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-error-summary"), gosx.Attr("role", "alert"), gosx.Attr("tabindex", "-1"), gosx.BoolAttr("autofocus")),
				heading(level, "", "There is a problem"), gosx.El("ul", nil, gosx.Fragment(items...))))
		}
	}
	if props.Feedback.Conflict != nil {
		nodes = append(nodes, ConflictView(ctx, props.Access, props.Resource, *props.Feedback.Conflict, fieldID+"-conflict", level))
	}
	nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "form-status gxa-form__status"))))
	nodes = append(nodes, actionHiddenInputs(props.Action)...)
	for _, field := range fields {
		access := props.Access.Field(ctx, props.Resource.Slug, field.Name)
		if !validFieldAccess(access) || access == FieldHidden {
			continue
		}
		id := fieldID + "-" + field.Name
		if access == FieldRedacted {
			nodes = append(nodes, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-field gxa-field--redacted")),
				gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-field__label")), gosx.Text(field.Label)),
				gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-field__value")), gosx.Text("Hidden"))))
			continue
		}
		fieldChildren := []gosx.Node{}
		labelChildren := []gosx.Node{gosx.Text(field.Label)}
		if field.Required {
			labelChildren = append(labelChildren, gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-required")), gosx.Text(" (required)")))
		}
		fieldChildren = append(fieldChildren, gosx.El("label", gosx.Attrs(gosx.Attr("for", id)), gosx.Fragment(labelChildren...)))
		described := []string{}
		if field.Help != "" {
			described = append(described, id+"-help")
			fieldChildren = append(fieldChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-help"), gosx.Attr("id", id+"-help")), gosx.Text(field.Help)))
		}
		described = append(described, id+"-error")
		if errText := props.Feedback.Errors[field.Name]; errText != "" {
			fieldChildren = append(fieldChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "form-error gxa-field-error"), gosx.Attr("id", id+"-error"), gosx.Attr("data-gosx-field-error", field.Name)), gosx.Text(errText)))
		} else {
			fieldChildren = append(fieldChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "form-error gxa-field-error"), gosx.Attr("id", id+"-error"), gosx.Attr("data-gosx-field-error", field.Name))))
		}
		errText := props.Feedback.Errors[field.Name]
		control := fieldControl(field, id, values[field.Name], described, field.Required, field.ReadOnly, errText != "")
		fieldChildren = append(fieldChildren, control)
		nodes = append(nodes, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-field")), gosx.Fragment(fieldChildren...)))
	}
	if props.Action.Action.Confirm {
		nodes = append(nodes, confirmationFields(props.Action, props.Feedback.Errors[ConfirmField]))
	}
	submit := props.Action.Submit
	if submit == "" {
		submit = props.Action.Action.Label
	}
	if submit == "" {
		submit = "Save"
	}
	nodes = append(nodes, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-form__actions")),
		gosx.El("button", gosx.Attrs(gosx.Attr("class", "gxa-button"), gosx.Attr("type", "submit")), gosx.Text(submit))))
	return server.Form(gosx.Attrs(gosx.Attr("class", "gxa-form"), gosx.Attr("method", "post"), gosx.Attr("action", props.Action.URL), gosx.BoolAttr("novalidate")), gosx.Fragment(nodes...))
}

// ParseForm validates submitted values and returns clean values and field
// errors. It keeps only fields that are writable and FieldVisible, so a
// crafted POST cannot write a hidden, redacted, read-only, or unknown field.
// It trims space; checks Required; counts MaxLength in Unicode code points;
// checks select and relation values against Options; normalizes booleans to
// "true" or "false"; parses datetime as "2006-01-02T15:04"; and accepts
// money matching ^[0-9]+(\.[0-9]{1,2})?$. Error text is plain text.
func ParseForm(ctx context.Context, access Access, resource string, fields []workbench.Field, form map[string]string) (values, errs map[string]string) {
	values = make(map[string]string)
	errs = make(map[string]string)
	for _, field := range fields {
		if field.ReadOnly || access.Field(ctx, resource, field.Name) != FieldVisible {
			continue
		}
		raw := strings.TrimSpace(form[field.Name])
		if field.Kind == workbench.FieldBoolean {
			if field.Required && raw != "true" {
				errs[field.Name] = "Check this box."
				continue
			}
			values[field.Name] = strconv.FormatBool(raw == "true")
			continue
		}
		values[field.Name] = raw
		if field.Required && raw == "" {
			errs[field.Name] = "Enter a " + articleLabel(field.Label, field.Name) + "."
			continue
		}
		if raw == "" {
			continue
		}
		if field.MaxLength > 0 && utf8.RuneCountInString(raw) > field.MaxLength {
			errs[field.Name] = fmt.Sprintf("Use %d characters or fewer.", field.MaxLength)
			continue
		}
		switch field.Kind {
		case workbench.FieldSelect, workbench.FieldRelation:
			if !contains(field.Options, raw) {
				errs[field.Name] = "Choose an available option."
			}
		case workbench.FieldDateTime:
			if _, err := time.Parse("2006-01-02T15:04", raw); err != nil {
				errs[field.Name] = "Enter a valid date and time."
			}
		case workbench.FieldMoney:
			if !moneyPattern.MatchString(raw) {
				errs[field.Name] = "Enter an amount with up to two decimal places."
			}
		}
	}
	if len(errs) == 0 {
		errs = nil
	}
	if len(values) == 0 {
		values = nil
	}
	return values, errs
}

var moneyPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

func fieldControl(field workbench.Field, id, value string, described []string, required, readOnly, invalid bool) gosx.Node {
	attrs := gosx.Attrs(gosx.Attr("id", id), gosx.Attr("name", field.Name))
	if len(described) > 0 {
		attrs = append(attrs, gosx.Attr("aria-describedby", strings.Join(described, " ")))
	}
	if required {
		attrs = append(attrs, gosx.BoolAttr("required"))
	}
	if field.MaxLength > 0 && field.Kind != workbench.FieldBoolean && field.Kind != workbench.FieldSelect && field.Kind != workbench.FieldRelation {
		attrs = append(attrs, gosx.Attr("maxlength", field.MaxLength))
	}
	readOnlyAttribute := readOnly && field.Kind != workbench.FieldBoolean && field.Kind != workbench.FieldSelect && field.Kind != workbench.FieldRelation
	if readOnlyAttribute {
		attrs = append(attrs, gosx.BoolAttr("readonly"))
	}
	if invalid {
		attrs = append(attrs, gosx.Attr("aria-invalid", "true"))
	}
	if field.Kind == workbench.FieldTextarea {
		attrs = append(attrs, gosx.Attr("class", "gxa-control"))
		return gosx.El("textarea", attrs, gosx.Text(value))
	}
	switch field.Kind {
	case workbench.FieldBoolean:
		attrs = append(attrs, gosx.Attr("class", "gxa-control gxa-control--check"), gosx.Attr("type", "checkbox"), gosx.Attr("value", "true"))
		if readOnly {
			attrs = append(attrs, gosx.BoolAttr("disabled"), gosx.Attr("aria-readonly", "true"))
		}
		if value == "true" {
			attrs = append(attrs, gosx.BoolAttr("checked"))
		}
	case workbench.FieldMoney:
		attrs = append(attrs, gosx.Attr("class", "gxa-control"), gosx.Attr("type", "text"), gosx.Attr("inputmode", "decimal"), gosx.Attr("value", value))
	case workbench.FieldDateTime:
		attrs = append(attrs, gosx.Attr("class", "gxa-control"), gosx.Attr("type", "datetime-local"), gosx.Attr("value", value))
	case workbench.FieldImage:
		attrs = append(attrs, gosx.Attr("class", "gxa-control"), gosx.Attr("type", "url"), gosx.Attr("value", value))
	case workbench.FieldSelect, workbench.FieldRelation:
		options := []gosx.Node{gosx.El("option", gosx.Attrs(gosx.Attr("value", "")), gosx.Text("Choose an option"))}
		for _, option := range field.Options {
			optionAttrs := gosx.Attrs(gosx.Attr("value", option))
			if option == value {
				optionAttrs = append(optionAttrs, gosx.BoolAttr("selected"))
			}
			options = append(options, gosx.El("option", optionAttrs, gosx.Text(option)))
		}
		attrs = append(attrs, gosx.Attr("class", "gxa-control"))
		if readOnly {
			attrs = append(attrs, gosx.BoolAttr("disabled"), gosx.Attr("aria-readonly", "true"))
		}
		return gosx.El("select", attrs, gosx.Fragment(options...))
	default:
		attrs = append(attrs, gosx.Attr("class", "gxa-control"), gosx.Attr("type", "text"), gosx.Attr("value", value))
	}
	return gosx.El("input", attrs)
}

func actionHiddenInputs(f ActionForm) []gosx.Node {
	inputs := []gosx.Node{
		hiddenInput(CSRFField, f.CSRF),
		hiddenInput(KeyField, f.Key),
		hiddenInput(RecordField, f.RecordID),
		hiddenInput(RevisionField, strconv.FormatInt(f.Revision, 10)),
		hiddenInput(returnTargetField, f.ReturnTo),
	}
	keys := make([]string, 0, len(f.Hidden))
	for key := range f.Hidden {
		if !reservedActionField(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		inputs = append(inputs, hiddenInput(key, f.Hidden[key]))
	}
	return inputs
}

func reservedActionField(key string) bool {
	switch key {
	case CSRFField, KeyField, RecordField, RevisionField, ConfirmField, returnTargetField:
		return true
	default:
		return false
	}
}

func confirmationFields(f ActionForm, errorText string) gosx.Node {
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
	nodes := []gosx.Node{}
	if confirmation.Summary != "" {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-confirm__summary")), gosx.Text(confirmation.Summary)))
	}
	if consequence != "" {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-confirm__consequence")), gosx.Text(consequence)))
	}
	if errorText != "" {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "form-error gxa-field-error"), gosx.Attr("id", id+"-error"), gosx.Attr("data-gosx-field-error", ConfirmField)), gosx.Text(errorText)))
	} else {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "form-error gxa-field-error"), gosx.Attr("id", id+"-error"), gosx.Attr("data-gosx-field-error", ConfirmField))))
	}
	confirmAttrs := gosx.Attrs(gosx.Attr("id", id), gosx.Attr("name", ConfirmField), gosx.Attr("type", "checkbox"), gosx.Attr("value", "yes"), gosx.Attr("aria-describedby", id+"-error"), gosx.BoolAttr("required"))
	if errorText != "" {
		confirmAttrs = append(confirmAttrs, gosx.Attr("aria-invalid", "true"))
	}
	nodes = append(nodes, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-check")),
		gosx.El("input", confirmAttrs),
		gosx.El("label", gosx.Attrs(gosx.Attr("for", id)), gosx.Text(check))))
	return gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-confirm__fields")), gosx.Fragment(nodes...))
}

func actionID(f ActionForm) string {
	if f.ID != "" {
		return safeID(f.ID)
	}
	base := "gxa-" + f.Resource + "-" + f.Action.Name
	if f.RecordID != "" {
		base += "-" + f.RecordID
	}
	return safeID(base)
}

func safeID(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "gxa-action"
	}
	return b.String()
}

func articleLabel(label, name string) string {
	if label == "" {
		label = name
	}
	if label == "" {
		return "value"
	}
	first, size := utf8.DecodeRuneInString(label)
	return strings.ToLower(string(first)) + label[size:]
}

func heading(level int, id, text string) gosx.Node {
	tag := "h" + strconv.Itoa(level)
	attrs := gosx.Attrs(nil)
	if id != "" {
		attrs = append(attrs, gosx.Attr("id", id))
	}
	return gosx.El(tag, attrs, gosx.Text(text))
}
