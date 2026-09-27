package mdppstudio

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/session"
)

const (
	FieldDraft    = "gxa_draft"
	FieldSource   = "gxa_source"
	FieldRevision = "gxa_revision"
	FieldKey      = "gxa_key"
	FieldAutosave = "gxa_autosave"
	FieldTarget   = "gxa_target"
	FieldVersion  = "gxa_version"
)

// Service runs the editor's server side. Store, Renderer, Actor, and CanEdit
// are required.
type Service struct {
	Store       DraftStore
	Renderer    Renderer
	Analyzer    func(source string, required []string) Analysis
	Media       MediaPolicy
	Actor       func(r *http.Request) (string, error)
	CanEdit     func(ctx context.Context, actor string, d Draft) bool
	TargetFor   func(d Draft) Target
	Templates   []Template
	MaxBytes    int
	Checkpoint  time.Duration
	Stylesheets []string
	Zone        *time.Location
	Now         func() time.Time
	OnError     func(error)
}

// SaveRequest saves new source for a draft.
type SaveRequest struct {
	DraftID  string
	Expected int64
	Source   string
	Key      string
	Autosave bool
	Actor    string
	At       time.Time
}

// SaveResult is a save's outcome.
type SaveResult struct {
	Draft    Draft
	Replayed bool
	Version  *Version
}

// ConflictError is returned when the draft changed after Expected.
type ConflictError struct{ Current Draft }

func (e *ConflictError) Error() string        { return ErrConflict.Error() }
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// Save writes source if the stored revision equals Expected.
func (s *Service) Save(ctx context.Context, req SaveRequest) (SaveResult, error) {
	return s.saveAs(ctx, req, VersionSave, "")
}

func (s *Service) saveAs(ctx context.Context, req SaveRequest, kind VersionKind, restoredFrom string) (SaveResult, error) {
	if s == nil || s.Store == nil || s.Renderer == nil {
		return SaveResult{}, errors.New("mdppstudio: service is not configured")
	}
	limit := s.MaxBytes
	if limit <= 0 {
		limit = 100_000
	}
	if len(req.Source) > limit {
		return SaveResult{}, ErrTooLarge
	}
	if req.At.IsZero() {
		req.At = s.now()
	}
	if req.Actor == "" {
		return SaveResult{}, errNoAccess
	}
	if req.Key != "" {
		if receipt, err := s.Store.Receipt(ctx, req.Key); err == nil {
			if receipt.DraftID != req.DraftID {
				return SaveResult{}, ErrDuplicateKey
			}
			d, err := s.Store.Draft(ctx, req.DraftID)
			if err != nil {
				return SaveResult{}, err
			}
			return SaveResult{Draft: d, Replayed: true}, nil
		} else if !errors.Is(err, ErrNotFound) {
			return SaveResult{}, err
		}
	}
	d, err := s.Store.Draft(ctx, req.DraftID)
	if err != nil {
		return SaveResult{}, err
	}
	if s.CanEdit == nil || !s.CanEdit(ctx, req.Actor, d) {
		return SaveResult{}, errNoAccess
	}
	if d.Locked {
		return SaveResult{}, ErrLocked
	}
	if d.Revision != req.Expected {
		return SaveResult{}, &ConflictError{Current: d}
	}
	if req.Autosave && kind == VersionSave {
		checkpoint := s.Checkpoint
		if checkpoint <= 0 {
			checkpoint = 5 * time.Minute
		}
		versions, e := s.Store.Versions(ctx, d.ID, 1)
		if e != nil && !errors.Is(e, ErrNotFound) {
			return SaveResult{}, e
		}
		if len(versions) == 0 || versions[0].Author != req.Actor || versions[0].At.IsZero() || req.At.Sub(versions[0].At) >= checkpoint {
			kind = VersionAutosave
		} else {
			kind = ""
		}
	}
	next := d
	next.Source = req.Source
	next.Updated = req.At
	next.UpdatedBy = req.Actor
	version := &Version{ID: randomID(), DraftID: d.ID, Kind: kind, Source: req.Source, Author: req.Actor, At: req.At, RestoredFrom: restoredFrom}
	var versionPtr *Version
	if !req.Autosave || kind != "" {
		versionPtr = version
	}
	if !req.Autosave {
		target := s.targetFor(d)
		preview, e := s.render(ctx, d, req.Source, target, d.Locale)
		if e != nil {
			return SaveResult{}, e
		}
		version.RendererVersion = preview.Output.Version
		version.OutputHash = preview.Hash
		if !preview.Output.Blocked() {
			next.Preview = Preview{Target: target, RendererVersion: preview.Output.Version, Hash: preview.Hash, HTML: preview.Output.HTML, Text: preview.Output.Text, Revision: d.Revision + 1, At: req.At}
		}
	}
	if versionPtr != nil {
		version.Revision = d.Revision + 1
	}
	updated, err := s.Store.PutDraft(ctx, DraftWrite{Draft: next, Expected: req.Expected, Version: versionPtr, Key: req.Key})
	if errors.Is(err, ErrConflict) {
		current, readErr := s.Store.Draft(ctx, d.ID)
		if readErr != nil {
			return SaveResult{}, readErr
		}
		return SaveResult{}, &ConflictError{Current: current}
	}
	if errors.Is(err, ErrDuplicateKey) && req.Key != "" {
		receipt, readErr := s.Store.Receipt(ctx, req.Key)
		if readErr == nil && receipt.DraftID == req.DraftID {
			current, readErr := s.Store.Draft(ctx, d.ID)
			if readErr != nil {
				return SaveResult{}, readErr
			}
			return SaveResult{Draft: current, Replayed: true}, nil
		}
	}
	if err != nil {
		return SaveResult{}, err
	}
	if versionPtr == nil {
		return SaveResult{Draft: updated}, nil
	}
	version.Revision = updated.Revision
	return SaveResult{Draft: updated, Version: version}, nil
}

// PreviewRequest renders unsaved source.
type PreviewRequest struct {
	DraftID string
	Source  string
	Target  Target
	Actor   string
}

// PreviewResult is a preview's outcome.
type PreviewResult struct {
	Output   RenderOutput
	Analysis Analysis
	Hash     string
	Document string
	Changed  bool
}

// Preview composes the source with the draft's template, renders it, runs
// Analyzer and Media, and merges their diagnostics. It does not write.
func (s *Service) Preview(ctx context.Context, req PreviewRequest) (PreviewResult, error) {
	if s == nil || s.Store == nil || s.Renderer == nil {
		return PreviewResult{}, errors.New("mdppstudio: service is not configured")
	}
	d, err := s.Store.Draft(ctx, req.DraftID)
	if err != nil {
		return PreviewResult{}, err
	}
	if req.Actor == "" || s.CanEdit == nil || !s.CanEdit(ctx, req.Actor, d) {
		return PreviewResult{}, errNoAccess
	}
	target := req.Target
	if target == "" {
		target = TargetSite
	}
	if err := validateTarget(target); err != nil {
		return PreviewResult{}, err
	}
	result, err := s.render(ctx, d, req.Source, target, d.Locale)
	if err != nil {
		return PreviewResult{}, err
	}
	styles := []string(nil)
	if target == TargetSite {
		styles = s.Stylesheets
	}
	result.Document = PreviewDocument(result.Output, target, d.Locale, styles)
	result.Changed = d.Preview.Hash != "" && d.Preview.RendererVersion != result.Output.Version && d.Preview.Hash != result.Hash
	return result, nil
}

func (s *Service) render(ctx context.Context, d Draft, source string, target Target, locale string) (PreviewResult, error) {
	return s.renderWithUse(ctx, d, source, target, locale, MediaUse{DraftID: d.ID, Target: target, Locale: locale, Kind: d.Kind})
}

func (s *Service) renderWithUse(ctx context.Context, d Draft, source string, target Target, locale string, use MediaUse) (PreviewResult, error) {
	if err := validateTarget(target); err != nil {
		return PreviewResult{}, err
	}
	composed := s.compose(d.Template, source)
	output, err := s.Renderer.Render(ctx, RenderInput{Source: composed, Target: target, Locale: locale, Template: d.Template})
	if err != nil {
		return PreviewResult{}, err
	}
	analysis := Analysis{}
	if s.Analyzer != nil {
		analysis = s.Analyzer(source, s.required(d.Template))
	}
	mediaAnalysis := analysis
	if composed != source && s.Analyzer != nil {
		mediaAnalysis = s.Analyzer(composed, nil)
	}
	if use.DraftID == "" {
		use.DraftID = d.ID
	}
	if use.Locale == "" {
		use.Locale = locale
	}
	if use.Kind == "" {
		use.Kind = d.Kind
	}
	use.Target = target
	media, err := CheckMedia(ctx, s.Media, mediaAnalysis, use)
	if err != nil {
		return PreviewResult{Output: RenderOutput{Diagnostics: append(output.Diagnostics, Diagnostic{Severity: SeverityError, Code: "media-permission", Message: "Image permission could not be checked."})}, Analysis: analysis}, err
	}
	output.Diagnostics = append(output.Diagnostics, analysis.Diagnostics...)
	output.Diagnostics = append(output.Diagnostics, media...)
	analysis.Diagnostics = append(analysis.Diagnostics, media...)
	if output.Blocked() {
		output.HTML, output.Text = "", ""
	}
	hash := OutputHash(output)
	styles := []string(nil)
	if target == TargetSite {
		styles = s.Stylesheets
	}
	return PreviewResult{Output: output, Analysis: analysis, Hash: hash, Document: PreviewDocument(output, target, locale, styles)}, nil
}

// RestoreRequest copies a version's source into a new revision.
type RestoreRequest struct {
	DraftID   string
	VersionID string
	Expected  int64
	Key       string
	Actor     string
	At        time.Time
}

// Restore saves the selected version as a new revision and keeps all history.
func (s *Service) Restore(ctx context.Context, req RestoreRequest) (SaveResult, error) {
	if s == nil || s.Store == nil {
		return SaveResult{}, errors.New("mdppstudio: service is not configured")
	}
	d, err := s.Store.Draft(ctx, req.DraftID)
	if err != nil {
		return SaveResult{}, err
	}
	if req.Actor == "" || s.CanEdit == nil || !s.CanEdit(ctx, req.Actor, d) {
		return SaveResult{}, errNoAccess
	}
	v, err := s.Store.Version(ctx, req.DraftID, req.VersionID)
	if err != nil {
		return SaveResult{}, err
	}
	return s.saveAs(ctx, SaveRequest{DraftID: d.ID, Expected: req.Expected, Source: v.Source, Key: req.Key, Actor: req.Actor, At: req.At}, VersionRestore, v.ID)
}

// GateResult says whether a saved draft may be published or sent.
type GateResult struct {
	Output  RenderOutput
	Hash    string
	Blocked bool
	Changed bool
}

// Gate renders the saved draft and compares its output with reviewedHash.
func (s *Service) Gate(ctx context.Context, draftID string, use MediaUse, reviewedHash string) (GateResult, error) {
	if s == nil || s.Store == nil || s.Renderer == nil {
		return GateResult{Blocked: true}, errors.New("mdppstudio: service is not configured")
	}
	d, err := s.Store.Draft(ctx, draftID)
	if err != nil {
		return GateResult{Blocked: true}, err
	}
	target := use.Target
	if target == "" {
		target = s.targetFor(d)
	}
	r, err := s.renderWithUse(ctx, d, d.Source, target, d.Locale, use)
	if err != nil {
		return GateResult{Output: r.Output, Blocked: true}, err
	}
	blocked := r.Output.Blocked()
	return GateResult{Output: r.Output, Hash: r.Hash, Blocked: blocked, Changed: reviewedHash != "" && reviewedHash != r.Hash}, nil
}

// SaveResponse is action.Result.Data for save and restore.
type SaveResponse struct {
	Revision        int64            `json:"revision"`
	SavedAt         time.Time        `json:"savedAt"`
	Replayed        bool             `json:"replayed,omitempty"`
	Conflict        *ConflictPayload `json:"conflict,omitempty"`
	ConflictVersion string           `json:"conflictVersion,omitempty"`
	Diagnostics     []Diagnostic     `json:"diagnostics,omitempty"`
	Source          string           `json:"source,omitempty"`
	Versions        []VersionSummary `json:"versions,omitempty"`
}

// ConflictPayload is the stored draft shown beside typed text.
type ConflictPayload struct {
	Revision  int64     `json:"revision"`
	Source    string    `json:"source"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PreviewResponse is action.Result.Data for the preview action.
type PreviewResponse struct {
	Target          Target   `json:"target"`
	Document        string   `json:"document"`
	Text            string   `json:"text,omitempty"`
	RendererVersion string   `json:"rendererVersion"`
	Hash            string   `json:"hash"`
	Blocked         bool     `json:"blocked"`
	Changed         bool     `json:"changed"`
	Analysis        Analysis `json:"analysis"`
}

// SaveAction handles the editor's save POST.
func (s *Service) SaveAction() action.Handler {
	return func(c *action.Context) error {
		if c.Request.Method != http.MethodPost {
			return action.Error(http.StatusMethodNotAllowed, "Method not allowed.")
		}
		form, ok := editorForm(c)
		if !ok {
			return action.Error(http.StatusForbidden, "Your session expired. Reload the page and try again.")
		}
		draftID, source, key, expected, autosave, err := parseSave(form)
		if err != nil {
			return editorReply(c, http.StatusUnprocessableEntity, false, "Reload the page and try again.", nil, "")
		}
		actor, _, status := s.editorActorDraft(c.Request, draftID)
		if status != 0 {
			return action.Error(status, editorStatusMessage(status))
		}
		result, err := s.Save(c.Request.Context(), SaveRequest{DraftID: draftID, Expected: expected, Source: source, Key: key, Autosave: autosave, Actor: actor})
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				response := SaveResponse{Revision: conflict.Current.Revision, Conflict: conflictPayload(conflict.Current)}
				if !action.WantsJSON(c.Request) {
					versionID := randomID()
					version := Version{ID: versionID, DraftID: draftID, Revision: expected, Kind: VersionConflict, Source: source, Author: actor, At: s.now()}
					if e := s.Store.AddVersion(c.Request.Context(), version); e != nil {
						s.report(e)
						return editorReply(c, http.StatusServiceUnavailable, false, "We could not save this yet. Please try again.", nil, "")
					}
					response.ConflictVersion = versionID
				}
				return editorReply(c, http.StatusConflict, false, "This record changed. Compare the saved version with your text.", response, "")
			}
			return s.editorError(c, err)
		}
		response := s.saveResponse(c.Request.Context(), result)
		response.Replayed = result.Replayed
		if !action.WantsJSON(c.Request) {
			c.RedirectBackWithMessage(".", s.savedMessage(result.Draft.Updated))
			return nil
		}
		return editorReply(c, http.StatusOK, true, "Saved.", response, "")
	}
}

// ReadSaveResult returns the flashed SaveResponse for a save or restore action.
func ReadSaveResult(r *http.Request, name string) (SaveResponse, bool) {
	view, ok := action.State(r, name)
	if !ok || len(view.Result.Data) == 0 {
		return SaveResponse{}, false
	}
	var result SaveResponse
	if json.Unmarshal(view.Result.Data, &result) != nil {
		return SaveResponse{}, false
	}
	return result, true
}

// PreviewAction handles the editor's managed preview POST.
func (s *Service) PreviewAction() action.Handler {
	return func(c *action.Context) error {
		if c.Request.Method != http.MethodPost {
			return action.Error(http.StatusMethodNotAllowed, "Method not allowed.")
		}
		if !action.WantsJSON(c.Request) {
			return action.Error(http.StatusBadRequest, "Preview requires the editor script.")
		}
		form, ok := editorForm(c)
		if !ok {
			return action.Error(http.StatusForbidden, "Your session expired. Reload the page and try again.")
		}
		draftID, source, _, _, _, err := parseSave(form)
		if err != nil {
			return editorReply(c, http.StatusUnprocessableEntity, false, "Reload the page and try again.", nil, "")
		}
		actor, _, status := s.editorActorDraft(c.Request, draftID)
		if status != 0 {
			return action.Error(status, editorStatusMessage(status))
		}
		target := Target(form[FieldTarget])
		if target == "" {
			target = TargetSite
		}
		result, err := s.Preview(c.Request.Context(), PreviewRequest{DraftID: draftID, Source: source, Target: target, Actor: actor})
		if err != nil {
			return s.editorError(c, err)
		}
		response := PreviewResponse{Target: target, Document: result.Document, Text: result.Output.Text, RendererVersion: result.Output.Version, Hash: result.Hash, Blocked: result.Output.Blocked(), Changed: result.Changed, Analysis: result.Analysis}
		return editorReply(c, http.StatusOK, true, "Preview updated.", response, "")
	}
}

// RestoreAction handles a restore POST and saves any differing posted text first.
func (s *Service) RestoreAction() action.Handler {
	return func(c *action.Context) error {
		if c.Request.Method != http.MethodPost {
			return action.Error(http.StatusMethodNotAllowed, "Method not allowed.")
		}
		form, ok := editorForm(c)
		if !ok {
			return action.Error(http.StatusForbidden, "Your session expired. Reload the page and try again.")
		}
		draftID := strings.TrimSpace(form[FieldDraft])
		expected, err := strconv.ParseInt(form[FieldRevision], 10, 64)
		key, versionID := form[FieldKey], strings.TrimSpace(form[FieldVersion])
		if draftID == "" || err != nil || expected < 1 || !validKey(key) || versionID == "" {
			return editorReply(c, http.StatusUnprocessableEntity, false, "Reload the page and try again.", nil, "")
		}
		actor, d, status := s.editorActorDraft(c.Request, draftID)
		if status != 0 {
			return action.Error(status, editorStatusMessage(status))
		}
		if source, posted := form[FieldSource]; posted && source != d.Source {
			preSave, saveErr := s.Save(c.Request.Context(), SaveRequest{DraftID: draftID, Expected: expected, Source: source, Key: randomID(), Actor: actor})
			if saveErr != nil {
				var conflict *ConflictError
				if errors.As(saveErr, &conflict) {
					return s.restoreConflict(c, draftID, source, expected, actor, conflict.Current)
				}
				return s.editorError(c, saveErr)
			}
			expected = preSave.Draft.Revision
		}
		result, err := s.Restore(c.Request.Context(), RestoreRequest{DraftID: draftID, VersionID: versionID, Expected: expected, Key: key, Actor: actor})
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				return s.restoreConflict(c, draftID, form[FieldSource], expected, actor, conflict.Current)
			}
			return s.editorError(c, err)
		}
		response := s.saveResponse(c.Request.Context(), result)
		response.Source = result.Draft.Source
		if !action.WantsJSON(c.Request) {
			c.RedirectBackWithMessage(".", s.savedMessage(result.Draft.Updated))
			return nil
		}
		return editorReply(c, http.StatusOK, true, "Version restored.", response, "")
	}
}

var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func validKey(key string) bool { return keyPattern.MatchString(key) }

var errNoAccess = errors.New("mdppstudio: not allowed")

func (s *Service) editorActorDraft(r *http.Request, id string) (string, Draft, int) {
	if s == nil || s.Store == nil || s.Actor == nil || s.CanEdit == nil {
		return "", Draft{}, http.StatusForbidden
	}
	actor, err := s.Actor(r)
	if err != nil || actor == "" {
		return "", Draft{}, http.StatusForbidden
	}
	d, err := s.Store.Draft(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		return "", Draft{}, http.StatusNotFound
	}
	if err != nil {
		s.report(err)
		return "", Draft{}, http.StatusServiceUnavailable
	}
	if !s.CanEdit(r.Context(), actor, d) {
		return "", Draft{}, http.StatusForbidden
	}
	return actor, d, 0
}

func editorForm(c *action.Context) (map[string]string, bool) {
	if c == nil || c.Request == nil || session.Current(c.Request) == nil {
		return nil, false
	}
	expected := session.Token(c.Request)
	actual := c.Request.Header.Get("X-CSRF-Token")
	if actual == "" {
		actual = c.Request.FormValue("csrf_token")
	}
	if expected == "" || len(expected) != len(actual) || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return nil, false
	}
	if c.FormData == nil {
		c.FormData = map[string]string{}
	}
	if len(c.FormData) == 0 {
		if err := c.Request.ParseForm(); err == nil {
			for k, values := range c.Request.Form {
				if len(values) > 0 {
					c.FormData[k] = values[0]
				}
			}
		}
	}
	return c.FormData, true
}

func parseSave(form map[string]string) (draftID, source, key string, expected int64, autosave bool, err error) {
	draftID, source, key = strings.TrimSpace(form[FieldDraft]), form[FieldSource], form[FieldKey]
	if draftID == "" || !validKey(key) {
		return "", "", "", 0, false, ErrNotFound
	}
	expected, err = strconv.ParseInt(form[FieldRevision], 10, 64)
	if err != nil || expected < 1 {
		return "", "", "", 0, false, ErrConflict
	}
	autosave = form[FieldAutosave] == "1"
	return draftID, source, key, expected, autosave, nil
}

func editorReply(c *action.Context, status int, ok bool, message string, data any, redirect string) error {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		raw = b
	}
	c.SetResult(action.Result{OK: ok, Message: message, Data: raw, Redirect: redirect})
	c.SetStatus(status)
	return nil
}

func (s *Service) restoreConflict(c *action.Context, draftID, source string, expected int64, actor string, current Draft) error {
	versionID := ""
	if !action.WantsJSON(c.Request) && len(source) <= s.maxBytes() {
		versionID = randomID()
		if err := s.Store.AddVersion(c.Request.Context(), Version{ID: versionID, DraftID: draftID, Revision: expected, Kind: VersionConflict, Source: source, Author: actor, At: s.now()}); err != nil {
			return s.editorError(c, err)
		}
	}
	response := SaveResponse{Revision: current.Revision, Conflict: conflictPayload(current), ConflictVersion: versionID}
	return editorReply(c, http.StatusConflict, false, "This record changed. Compare the saved version with your text.", response, "")
}

func (s *Service) saveResponse(ctx context.Context, result SaveResult) SaveResponse {
	r := SaveResponse{Revision: result.Draft.Revision, SavedAt: result.Draft.Updated, Replayed: result.Replayed}
	needsDiagnostics := result.Replayed || result.Version != nil && result.Version.Kind != VersionAutosave
	if s.Renderer != nil && needsDiagnostics {
		preview, err := s.render(ctx, result.Draft, result.Draft.Source, s.targetFor(result.Draft), result.Draft.Locale)
		if err == nil {
			r.Diagnostics = preview.Output.Diagnostics
		}
	}
	if versions, err := s.Store.Versions(ctx, result.Draft.ID, 20); err == nil {
		r.Versions = make([]VersionSummary, 0, len(versions))
		for _, v := range versions {
			r.Versions = append(r.Versions, v.Summary())
		}
	}
	return r
}

func conflictPayload(d Draft) *ConflictPayload {
	return &ConflictPayload{Revision: d.Revision, Source: d.Source, UpdatedBy: d.UpdatedBy, UpdatedAt: d.Updated}
}

func (s *Service) editorError(c *action.Context, err error) error {
	if errors.Is(err, ErrTooLarge) || errors.Is(err, ErrLocked) {
		return editorReply(c, http.StatusUnprocessableEntity, false, "Check the highlighted fields.", nil, "")
	}
	if errors.Is(err, errNoAccess) {
		return action.Error(http.StatusForbidden, "Not found.")
	}
	if errors.Is(err, ErrNotFound) {
		return action.Error(http.StatusNotFound, "Not found.")
	}
	if errors.Is(err, ErrConflict) {
		return editorReply(c, http.StatusConflict, false, "This record changed. Reload the page and try again.", nil, "")
	}
	s.report(err)
	return editorReply(c, http.StatusServiceUnavailable, false, "We could not save this yet. Please try again.", nil, "")
}

func editorStatusMessage(status int) string {
	switch status {
	case http.StatusNotFound:
		return "Not found."
	case http.StatusServiceUnavailable:
		return "We could not save this yet. Please try again."
	default:
		return "Not found."
	}
}

func (s *Service) report(err error) {
	if err == nil {
		return
	}
	if s != nil && s.OnError != nil {
		s.OnError(err)
	} else if action.ErrorLogger != nil {
		action.ErrorLogger(err)
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) maxBytes() int {
	if s.MaxBytes > 0 {
		return s.MaxBytes
	}
	return 100_000
}
func (s *Service) targetFor(d Draft) Target {
	if s.TargetFor != nil {
		if t := s.TargetFor(d); t != "" {
			return t
		}
	}
	return TargetSite
}
func (s *Service) template(id string) *Template {
	for i := range s.Templates {
		if s.Templates[i].ID == id {
			return &s.Templates[i]
		}
	}
	return nil
}
func (s *Service) required(id string) []string {
	if t := s.template(id); t != nil {
		return t.Required
	}
	return nil
}
func (s *Service) compose(id, source string) string {
	if t := s.template(id); t != nil {
		return t.Compose(source)
	}
	return source
}
func (s *Service) savedMessage(at time.Time) string {
	zone := s.Zone
	if zone == nil {
		zone = time.UTC
	}
	return "Saved at " + at.In(zone).Format("15:04") + "."
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("mdppstudio: generate identifier: %w", err))
	}
	return hex.EncodeToString(b[:])
}
