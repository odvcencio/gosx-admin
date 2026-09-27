package render

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

var (
	// ErrNotFound makes Guard answer 404. Use it when the record does not
	// exist or is outside the principal's scope.
	ErrNotFound = errors.New("render: not found")
	// ErrUnauthenticated makes Guard redirect to GuardOptions.SignInURL.
	ErrUnauthenticated = errors.New("render: sign-in required")
)

// ConflictError makes Guard answer 409 with the conflict view.
type ConflictError struct {
	// Revision is the current revision.
	Revision int64
	// Current maps field names to saved display values.
	Current map[string]string
}

func (e *ConflictError) Error() string { return "record revision conflict" }

// ValidationError makes Guard answer 422 with field errors.
type ValidationError struct {
	// Message is the plain status text. Empty uses the default validation text.
	Message string
	// Fields maps field names to their validation messages.
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if e == nil || e.Message == "" {
		return "field validation failed"
	}
	return e.Message
}

// ActionInput is what Guard passes to an ActionFunc after its checks pass.
type ActionInput struct {
	// Principal is the signed-in operator.
	Principal Principal
	// Resource is the workbench slug.
	Resource string
	// Action is the action name.
	Action string
	// RecordID is empty for a create action.
	RecordID string
	// Key is a valid idempotency key. The store records it with the change and
	// replays the first result for a repeated key.
	Key string
	// Revision is the revision the operator saw. Zero creates.
	Revision int64
	// Values holds parsed fields and the trimmed GuardOptions.Inputs.
	Values map[string]string
	// Audit is the success event. Set NewRevision and write it with the change.
	Audit AuditEvent
	// Request is the originating request.
	Request *http.Request
}

// ActionResult is a successful action's outcome.
type ActionResult struct {
	// Revision is the revision after the write.
	Revision int64
	// Replayed reports that Key was already used and the first result is
	// returned again.
	Replayed bool
	// Message is the status text. Default "Saved.".
	Message string
	// Redirect is the POST-redirect-GET target. Empty uses the submitted return
	// target, then GuardOptions.Fallback.
	Redirect string
}

// ActionFunc performs one admin action.
type ActionFunc func(ctx context.Context, in ActionInput) (ActionResult, error)

// GuardOptions configures Guard.
type GuardOptions struct {
	// Resource and Action identify the guarded workbench action.
	Resource workbench.Resource
	Action   workbench.Action
	// Fields are validated with ParseForm and passed in ActionInput.Values.
	// Leave nil for actions without fields.
	Fields []workbench.Field
	// Inputs names extra values to pass through trimmed.
	Inputs     []string
	Authorizer Authorizer
	// Principal returns the signed-in operator. ErrUnauthenticated redirects
	// to SignInURL.
	Principal func(r *http.Request) (Principal, error)
	// SignInURL is the redirect target for ErrUnauthenticated.
	SignInURL string
	// Fallback is used when the form has no valid return target. Default "/".
	Fallback string
	// Auditor records non-success outcomes. Optional.
	Auditor Auditor
	// OnError receives errors that become 503. Nil uses action.ErrorLogger.
	OnError func(error)
	// Now supplies the audit time. Nil uses time.Now.
	Now func() time.Time
}

const conflictValueLimit = 1536

// Guard returns a GoSX action handler. It checks, in order: Principal;
// session and CSRF; Authorizer.CanAct; ValidKey and a non-negative revision;
// required confirmation; and ParseForm validation. It then calls fn with
// parsed fields and trimmed Inputs.
//
// ErrUnauthenticated redirects to SignInURL. Missing session or bad CSRF
// returns 403; denied action or ErrNotFound returns 404; malformed key or
// revision, missing confirmation, and invalid fields return 422; stale
// revision returns 409; other failures call OnError and return 503. Success
// and replay return 303 with the handler message or "Saved.". Redirect uses
// ActionResult.Redirect when set, otherwise the submitted return target and
// then Fallback.
//
// A conflict returns entered values and current-value data filtered to fields
// the principal may see. Managed conflicts with at most 1,536 JSON bytes are
// flashed in the session and return a redirect target. Larger managed
// conflicts keep the page in place without a redirect. Larger native
// conflicts drop values and current-value data and return the plain size-limit
// message. Native validation failures over the same limit drop Values.
// Entered values never echo csrf_token, gxa_key, or __gosx_return_to. Guard
// audits denied, invalid, conflict, and failed attempts; the action handler
// records success in its own transaction.
func Guard(opts GuardOptions, fn ActionFunc) action.Handler {
	return func(c *action.Context) error {
		r := c.Request
		form := c.FormData
		if form == nil {
			form = map[string]string{}
		}
		base := AuditEvent{
			At:        guardNow(opts),
			RequestID: server.RequestID(r),
			Resource:  opts.Resource.Slug,
			Action:    opts.Action.Name,
			RecordID:  form[RecordField],
			Key:       form[KeyField],
		}
		principal := Principal{}
		if opts.Principal == nil {
			return action.Redirect(defaultSignIn(opts.SignInURL))
		}
		p, err := opts.Principal(r)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				return action.Redirect(defaultSignIn(opts.SignInURL))
			}
			guardOnError(opts, err)
			recordAudit(r, opts, base, AuditFailed, "error")
			return responseError(http.StatusServiceUnavailable, "We could not save this yet. Please try again.", nil, nil, nil, "")
		}
		principal = p
		base.ActorID = p.ID

		if r == nil || session.Current(r) == nil {
			recordAudit(r, opts, base, AuditDenied, "csrf")
			return responseError(http.StatusForbidden, "Your session expired. Reload the page and try again.", nil, nil, nil, "")
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			token = form[CSRFField]
		}
		expected := session.Token(r)
		if token == "" || expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			recordAudit(r, opts, base, AuditDenied, "csrf")
			return responseError(http.StatusForbidden, "Your session expired. Reload the page and try again.", nil, nil, nil, "")
		}

		access := Access{Principal: principal, Authorizer: opts.Authorizer}
		if !access.CanAct(r.Context(), opts.Resource.Slug, opts.Action.Name) {
			recordAudit(r, opts, base, AuditDenied, "forbidden")
			return responseError(http.StatusNotFound, "Not found.", nil, nil, nil, "")
		}
		if !ValidKey(form[KeyField]) {
			recordAudit(r, opts, base, AuditInvalid, "key")
			return responseError(http.StatusUnprocessableEntity, "Reload the page and try again.", nil, nil, nil, "")
		}
		revision, err := strconv.ParseInt(form[RevisionField], 10, 64)
		if err != nil || revision < 0 {
			recordAudit(r, opts, base, AuditInvalid, "revision")
			return responseError(http.StatusUnprocessableEntity, "Reload the page and try again.", nil, nil, nil, "")
		}
		base.Revision = revision
		if opts.Action.Confirm && form[ConfirmField] != "yes" {
			recordAudit(r, opts, base, AuditInvalid, "confirm")
			return responseError(http.StatusUnprocessableEntity, "Check the highlighted fields.", nil, map[string]string{ConfirmField: "Check this box to confirm."}, nil, "")
		}

		values, fieldErrors := ParseForm(r.Context(), access, opts.Resource.Slug, opts.Fields, form)
		for _, name := range opts.Inputs {
			if reservedActionField(name) {
				continue
			}
			if value, ok := form[name]; ok {
				if values == nil {
					values = make(map[string]string)
				}
				values[name] = strings.TrimSpace(value)
			}
		}
		entered := safeValues(values)
		if len(fieldErrors) > 0 {
			recordAudit(r, opts, base, AuditInvalid, "fields")
			if !action.WantsJSON(r) && encodedSize(entered) > conflictValueLimit {
				entered = nil
			}
			return responseError(http.StatusUnprocessableEntity, "Check the highlighted fields.", entered, fieldErrors, nil, "")
		}
		base.RecordID = form[RecordField]
		base.Key = form[KeyField]
		in := ActionInput{
			Principal: principal,
			Resource:  opts.Resource.Slug,
			Action:    opts.Action.Name,
			RecordID:  form[RecordField],
			Key:       form[KeyField],
			Revision:  revision,
			Values:    values,
			Audit: AuditEvent{
				At:        base.At,
				RequestID: base.RequestID,
				ActorID:   principal.ID,
				Resource:  base.Resource,
				Action:    base.Action,
				RecordID:  base.RecordID,
				Key:       base.Key,
				Revision:  base.Revision,
				Outcome:   AuditOK,
			},
			Request: r,
		}
		if fn == nil {
			err = errors.New("render: action function is nil")
		} else {
			var result ActionResult
			result, err = fn(r.Context(), in)
			if err == nil {
				message := result.Message
				if message == "" {
					message = "Saved."
				}
				if result.Redirect != "" {
					c.RedirectWithMessage(result.Redirect, message)
				} else {
					c.RedirectBackWithMessage(defaultFallback(opts.Fallback), message)
				}
				return nil
			}
		}

		var validation *ValidationError
		if errors.As(err, &validation) {
			message := validation.Message
			if message == "" {
				message = "Check the highlighted fields."
			}
			recordAudit(r, opts, base, AuditInvalid, "fields")
			return responseError(http.StatusUnprocessableEntity, message, entered, validation.Fields, nil, "")
		}
		var conflict *ConflictError
		if errors.As(err, &conflict) {
			recordAudit(r, opts, base, AuditConflict, "conflict")
			return conflictResult(r, opts, access, entered, conflict)
		}
		if errors.Is(err, ErrNotFound) {
			recordAudit(r, opts, base, AuditDenied, "not_found")
			return responseError(http.StatusNotFound, "Not found.", nil, nil, nil, "")
		}
		guardOnError(opts, err)
		recordAudit(r, opts, base, AuditFailed, "error")
		return responseError(http.StatusServiceUnavailable, "We could not save this yet. Please try again.", entered, nil, nil, "")
	}
}

func conflictResult(r *http.Request, opts GuardOptions, access Access, entered map[string]string, conflict *ConflictError) error {
	copyCurrent := visibleConflictValues(r.Context(), access, opts.Resource, conflict.Current)
	copyEntered := cloneValues(entered)
	feedback := Feedback{
		Action:  opts.Action.Name,
		Status:  http.StatusConflict,
		OK:      false,
		Message: "This record changed after you opened it. Check the saved values, then save again.",
		Values:  copyEntered,
		Conflict: &Conflict{
			Revision: conflict.Revision,
			Current:  copyCurrent,
			Entered:  copyEntered,
		},
	}
	data := map[string]any{"revision": conflict.Revision, "current": copyCurrent}
	managed := action.WantsJSON(r)
	size := encodedConflictSize(copyEntered, copyCurrent)
	if managed {
		if size <= conflictValueLimit {
			session.AddFlash(r, conflictFlashKey, feedback)
			return responseError(http.StatusConflict, feedback.Message, copyEntered, nil, data, requestedTarget(r, opts.Fallback))
		}
		message := "This record changed after you opened it. Your entries are still in the form. Open the page in a new tab to see the saved values."
		return responseError(http.StatusConflict, message, copyEntered, nil, nil, "")
	}
	if size > conflictValueLimit {
		feedback.Message = "This record changed. Your entries were too long to keep, so check the saved values and enter your change again."
		copyEntered = nil
		data = nil
		feedback.Values = nil
		feedback.Conflict = nil
		return responseError(http.StatusConflict, feedback.Message, nil, nil, nil, "")
	}
	return responseError(http.StatusConflict, feedback.Message, copyEntered, nil, data, "")
}

func visibleConflictValues(ctx context.Context, access Access, resource workbench.Resource, values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	visible := make(map[string]string)
	for _, field := range resource.Fields {
		if access.Field(ctx, resource.Slug, field.Name) != FieldVisible {
			continue
		}
		if value, ok := values[field.Name]; ok {
			visible[field.Name] = value
		}
	}
	if len(visible) == 0 {
		return nil
	}
	return visible
}

func responseError(status int, message string, values, fields map[string]string, data any, redirect string) error {
	var raw json.RawMessage
	if data != nil {
		encoded, err := json.Marshal(data)
		if err == nil {
			raw = encoded
		}
	}
	return &action.ResultError{Status: status, Result: action.Result{
		OK:          false,
		Message:     message,
		Data:        raw,
		FieldErrors: fields,
		Values:      values,
		Redirect:    redirect,
	}}
}

func recordAudit(r *http.Request, opts GuardOptions, event AuditEvent, outcome AuditOutcome, reason string) {
	if opts.Auditor == nil {
		return
	}
	event.Outcome = outcome
	event.Reason = reason
	if err := opts.Auditor.Record(requestContext(r), event); err != nil {
		guardOnError(opts, err)
	}
}

func requestContext(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}

func guardOnError(opts GuardOptions, err error) {
	if err == nil {
		return
	}
	if opts.OnError != nil {
		opts.OnError(err)
	} else if action.ErrorLogger != nil {
		action.ErrorLogger(err)
	}
}

func guardNow(opts GuardOptions) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now()
}

func defaultFallback(value string) string {
	if normalized, ok := action.NormalizeReturnTarget(value); ok {
		return normalized
	}
	return "/"
}

func defaultSignIn(value string) string {
	if normalized, ok := action.NormalizeReturnTarget(value); ok {
		return normalized
	}
	return "/"
}

func requestedTarget(r *http.Request, fallback string) string {
	if r != nil {
		if raw := r.FormValue(returnTargetField); raw != "" {
			if target, ok := action.NormalizeReturnTarget(raw); ok {
				return target
			}
		}
		if ref := r.Referer(); ref != "" {
			parsed, err := url.Parse(ref)
			if err == nil && parsed.Host == "" {
				if target, ok := action.NormalizeReturnTarget(parsed.RequestURI()); ok {
					return target
				}
			} else if err == nil && r.URL != nil && (parsed.Host == r.Host || parsed.Hostname() == r.URL.Hostname()) {
				candidate := parsed.EscapedPath()
				if parsed.RawQuery != "" {
					candidate += "?" + parsed.RawQuery
				}
				if target, ok := action.NormalizeReturnTarget(candidate); ok {
					return target
				}
			}
		}
	}
	return defaultFallback(fallback)
}

func encodedConflictSize(entered, current map[string]string) int {
	data, _ := json.Marshal(struct {
		Entered map[string]string `json:"entered"`
		Current map[string]string `json:"current"`
	}{entered, current})
	return len(data)
}

func encodedSize(values map[string]string) int {
	data, _ := json.Marshal(values)
	return len(data)
}

func cloneValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
