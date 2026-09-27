package mdppstudio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx/server"
)

const (
	// AssetPrefix is the path prefix where a consumer mounts Assets.
	AssetPrefix = "/_gxa/mdppstudio/"
	// SurfaceName is the GoSX runtime surface registered by the editor script.
	SurfaceName = "gosx-admin/mdpp-editor"
)

// Snippet is short text an author can insert at the cursor.
type Snippet struct {
	ID    string
	Label string
	// Body is inserted as typed text. The first "${cursor}" marks the caret.
	Body string
}

// Template gives a draft locked parts. Header and Footer are not editable.
type Template struct {
	ID       string
	Label    string
	Header   string
	Body     string
	Footer   string
	Required []string
}

// Compose joins the non-empty locked parts and body with one blank line.
func (t Template) Compose(body string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{t.Header, body, t.Footer} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n\n")
}

// EditorLabels is the editor's visible text. Empty fields use English defaults.
type EditorLabels struct {
	Source      string
	Preview     string
	Problems    string
	History     string
	Insert      string
	Save        string
	Saved       string
	Saving      string
	Unsaved     string
	Offline     string
	Conflict    string
	KeepMine    string
	UseTheirs   string
	Restore     string
	ShowPane    string
	LockedAbove string
	LockedBelow string
}

// EditorConfig configures Editor.
type EditorConfig struct {
	ID            string
	Draft         Draft
	SaveURL       string
	PreviewURL    string
	RestoreURL    string
	CSRF          string
	Key           string
	ReturnTo      string
	Snippets      []Snippet
	Template      *Template
	Targets       []Target
	Analysis      Analysis
	Versions      []VersionSummary
	Worker        *WorkerAssets
	AutosaveDelay time.Duration
	PreviewDelay  time.Duration
	MaxBytes      int
	NoLocalBackup bool
	Stylesheets   []string
	HeadingLevel  int
	Zone          *time.Location
	Conflict      *EditorConflict
	Labels        EditorLabels
}

// EditorConflict is a conflict shown after a native save failed.
type EditorConflict struct {
	Saved ConflictPayload
	Mine  string
}

type editorRuntimeConfig struct {
	Protocol    int              `json:"protocol"`
	DraftID     string           `json:"draftID"`
	Revision    int64            `json:"revision"`
	SaveURL     string           `json:"saveURL"`
	PreviewURL  string           `json:"previewURL"`
	RestoreURL  string           `json:"restoreURL"`
	AutosaveMS  int64            `json:"autosaveMs"`
	PreviewMS   int64            `json:"previewMs"`
	MaxBytes    int              `json:"maxBytes"`
	LocalBackup bool             `json:"localBackup"`
	Targets     []Target         `json:"targets"`
	Required    []string         `json:"required"`
	Snippets    []Snippet        `json:"snippets"`
	Worker      *WorkerAssets    `json:"worker"`
	Labels      editorLabelsJSON `json:"labels"`
}

type editorLabelsJSON struct {
	Source      string `json:"source"`
	Preview     string `json:"preview"`
	Problems    string `json:"problems"`
	History     string `json:"history"`
	Insert      string `json:"insert"`
	Save        string `json:"save"`
	Saved       string `json:"saved"`
	Saving      string `json:"saving"`
	Unsaved     string `json:"unsaved"`
	Offline     string `json:"offline"`
	Conflict    string `json:"conflict"`
	KeepMine    string `json:"keepMine"`
	UseTheirs   string `json:"useTheirs"`
	Restore     string `json:"restore"`
	ShowPane    string `json:"showPane"`
	LockedAbove string `json:"lockedAbove"`
	LockedBelow string `json:"lockedBelow"`
}

var defaultEditorLabels = EditorLabels{
	Source: "Text", Preview: "Preview", Problems: "Problems", History: "History", Insert: "Insert", Save: "Save",
	Saved: "Saved at %s", Saving: "Saving…", Unsaved: "Not saved yet",
	Offline:  "Not saved: you are offline. Your text is kept on this device.",
	Conflict: "Not saved: this draft changed somewhere else. Compare the two versions.",
	KeepMine: "Save my text as the new version", UseTheirs: "Replace my text with the saved version",
	Restore: "Restore this version", ShowPane: "Show", LockedAbove: "Fixed text above (cannot be changed)",
	LockedBelow: "Fixed text below (cannot be changed)",
}

// Editor renders the editor and registers its browser runtime on page.
func Editor(page *server.PageState, cfg EditorConfig) gosx.Node {
	if page == nil {
		return gosx.Text("")
	}
	if cfg.ID == "" {
		cfg.ID = "gxa-editor"
	}
	if cfg.HeadingLevel < 2 || cfg.HeadingLevel > 5 {
		cfg.HeadingLevel = 2
	}
	if cfg.AutosaveDelay <= 0 {
		cfg.AutosaveDelay = 2 * time.Second
	}
	if cfg.PreviewDelay <= 0 {
		cfg.PreviewDelay = 800 * time.Millisecond
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 100_000
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = []Target{TargetSite}
	}
	labels := mergeLabels(cfg.Labels)
	page.Runtime().EnableBootstrap()
	page.Runtime().LifecycleScript(EditorScriptURL())
	revision := cfg.Draft.Revision
	source := cfg.Draft.Source
	if cfg.Conflict != nil {
		revision = cfg.Conflict.Saved.Revision
		source = cfg.Conflict.Mine
	}
	if cfg.Key == "" {
		cfg.Key = newPageKey()
	}
	if cfg.ReturnTo == "" {
		cfg.ReturnTo = "."
	}
	required := []string(nil)
	if cfg.Template != nil {
		required = append(required, cfg.Template.Required...)
	}
	config := editorRuntimeConfig{
		Protocol: WorkerProtocol, DraftID: cfg.Draft.ID, Revision: revision,
		SaveURL: cfg.SaveURL, PreviewURL: cfg.PreviewURL, RestoreURL: cfg.RestoreURL,
		AutosaveMS: cfg.AutosaveDelay.Milliseconds(), PreviewMS: cfg.PreviewDelay.Milliseconds(), MaxBytes: cfg.MaxBytes,
		LocalBackup: !cfg.NoLocalBackup, Targets: cfg.Targets, Required: required, Snippets: cfg.Snippets, Worker: cfg.Worker,
		Labels: labelsJSON(labels),
	}
	configNode := page.JSONScript(cfg.ID+"-config", config)
	heading := func(level int, id, class, text string) gosx.Node {
		attrs := gosx.Attrs(gosx.Attr("id", id))
		if class != "" {
			attrs = append(attrs, gosx.Attr("class", class))
		}
		return gosx.El(fmt.Sprintf("h%d", level), attrs, gosx.Text(text))
	}
	pageTitle := cfg.Draft.Title
	if pageTitle == "" {
		pageTitle = labels.Source
	}
	formChildren := []gosx.Node{
		hiddenInput("csrf_token", cfg.CSRF), hiddenInput(FieldDraft, cfg.Draft.ID), hiddenInput(FieldRevision, fmt.Sprint(revision)),
		hiddenInput(FieldKey, cfg.Key), hiddenInput("__gosx_return_to", cfg.ReturnTo),
	}
	formChildren = append(formChildren, editorBar(cfg, labels)...)
	formChildren = append(formChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__live gxa-visually-hidden"), gosx.Attr("id", cfg.ID+"-live"), gosx.Attr("role", "status"))))
	if cfg.Conflict != nil {
		formChildren = append(formChildren, conflictMarkup(cfg, labels, *cfg.Conflict, heading(cfg.HeadingLevel+1, cfg.ID+"-conflict-title", "", "This draft changed somewhere else")))
	}
	if cfg.Template != nil && cfg.Template.Header != "" {
		formChildren = append(formChildren, lockedPart(cfg.ID+"-locked-above", labels.LockedAbove, cfg.Template.Header))
	}
	formChildren = append(formChildren, editorPanes(cfg, labels, source, heading)...)
	if cfg.Template != nil && cfg.Template.Footer != "" {
		formChildren = append(formChildren, lockedPart(cfg.ID+"-locked-below", labels.LockedBelow, cfg.Template.Footer))
	}
	formChildren = append(formChildren, problemsMarkup(cfg, labels, heading), historyMarkup(cfg, labels, heading))
	form := gosx.El("form", gosx.Attrs(gosx.Attr("class", "gxa-editor__form"), gosx.Attr("method", "post"), gosx.Attr("action", cfg.SaveURL), gosx.BoolAttr("data-gosx-native")), gosx.Fragment(formChildren...))
	rootAttrs := gosx.Attrs(gosx.Attr("class", "gxa-editor"), gosx.Attr("id", cfg.ID), gosx.Attr("aria-labelledby", cfg.ID+"-title"), gosx.Attr("data-gxa-config", cfg.ID+"-config"))
	root := gosx.RuntimeSurface("section", gosx.RuntimeSurfaceOptions{Name: SurfaceName, Version: "1", Fallback: "server"}, rootAttrs, heading(cfg.HeadingLevel, cfg.ID+"-title", "gxa-editor__title", pageTitle), configNode, form)
	return root
}

func editorBar(cfg EditorConfig, labels EditorLabels) []gosx.Node {
	saved := labels.Unsaved
	if !cfg.Draft.Updated.IsZero() {
		zone := cfg.Zone
		if zone == nil {
			zone = time.UTC
		}
		saved = fmt.Sprintf(labels.Saved, cfg.Draft.Updated.In(zone).Format("15:04"))
	}
	switcher := gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__switch"), gosx.Attr("role", "group"), gosx.Attr("aria-label", labels.ShowPane), gosx.BoolAttr("hidden")),
		gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("aria-pressed", "true"), gosx.Attr("aria-controls", cfg.ID+"-write"), gosx.Attr("data-gxa-pane", "write")), gosx.Text(labels.Source)),
		gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("aria-pressed", "false"), gosx.Attr("aria-controls", cfg.ID+"-preview"), gosx.Attr("data-gxa-pane", "preview")), gosx.Text(labels.Preview)))
	return []gosx.Node{gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__bar")),
		gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__saved"), gosx.Attr("id", cfg.ID+"-saved")), gosx.Text(saved)), switcher,
		gosx.El("button", gosx.Attrs(gosx.Attr("type", "submit"), gosx.Attr("class", "gxa-button")), gosx.Text(labels.Save)))}
}

func editorPanes(cfg EditorConfig, labels EditorLabels, source string, heading func(int, string, string, string) gosx.Node) []gosx.Node {
	writeChildren := []gosx.Node{heading(cfg.HeadingLevel+1, cfg.ID+"-write-title", "", labels.Source)}
	if len(cfg.Snippets) > 0 {
		items := make([]gosx.Node, 0, len(cfg.Snippets))
		for _, snippet := range cfg.Snippets {
			button := gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("data-gxa-snippet", snippet.ID), gosx.BoolAttr("hidden")), gosx.Text(snippet.Label))
			pre := gosx.El("pre", gosx.Attrs(gosx.Attr("class", "gxa-editor__snippet-text")), gosx.Text(snippet.Body))
			items = append(items, gosx.El("li", button, pre))
		}
		writeChildren = append(writeChildren, gosx.El("details", gosx.Attrs(gosx.Attr("class", "gxa-editor__snippets")), gosx.El("summary", gosx.Text(labels.Insert)), gosx.El("ul", gosx.Fragment(items...))))
	}
	writeChildren = append(writeChildren,
		gosx.El("label", gosx.Attrs(gosx.Attr("class", "gxa-visually-hidden"), gosx.Attr("for", cfg.ID+"-source")), gosx.Text(labels.Source)),
		gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__field")),
			gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__cues"), gosx.Attr("aria-hidden", "true"))),
			gosx.El("textarea", gosx.Attrs(gosx.Attr("id", cfg.ID+"-source"), gosx.Attr("name", FieldSource), gosx.Attr("class", "gxa-editor__source"), gosx.Attr("rows", "18"), gosx.Attr("spellcheck", "true"), gosx.Attr("autocapitalize", "sentences"), gosx.Attr("lang", cfg.Draft.Locale), gosx.Attr("aria-describedby", cfg.ID+"-saved "+cfg.ID+"-problems-count")), gosx.Text(source))))
	write := gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-editor__pane gxa-editor__pane--write"), gosx.Attr("id", cfg.ID+"-write"), gosx.Attr("aria-labelledby", cfg.ID+"-write-title")), gosx.Fragment(writeChildren...))
	previewChildren := []gosx.Node{heading(cfg.HeadingLevel+1, cfg.ID+"-preview-title", "", labels.Preview)}
	if len(cfg.Targets) > 1 {
		buttons := make([]gosx.Node, 0, len(cfg.Targets))
		for i, target := range cfg.Targets {
			buttons = append(buttons, gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("data-gxa-target", target), gosx.Attr("aria-pressed", fmt.Sprint(i == 0))), gosx.Text(targetLabel(target))))
		}
		previewChildren = append(previewChildren, gosx.El("div", gosx.Attrs(gosx.Attr("role", "group"), gosx.Attr("aria-label", "Preview as"), gosx.Attr("class", "gxa-editor__targets")), gosx.Fragment(buttons...)))
	}
	previewChildren = append(previewChildren,
		gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__changed"), gosx.BoolAttr("hidden")), gosx.Text("The output changed after an update. Review it before you send.")),
		gosx.El("iframe", gosx.Attrs(gosx.Attr("class", "gxa-editor__frame"), gosx.Attr("title", labels.Preview), gosx.BoolAttr("sandbox"), gosx.Attr("srcdoc", initialPreview(cfg)))),
	)
	preview := gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-editor__pane gxa-editor__pane--preview"), gosx.Attr("id", cfg.ID+"-preview"), gosx.Attr("aria-labelledby", cfg.ID+"-preview-title")), gosx.Fragment(previewChildren...))
	return []gosx.Node{gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__panes")), write, preview)}
}

func initialPreview(cfg EditorConfig) string {
	target := cfg.Draft.Preview.Target
	if target == "" {
		target = TargetSite
	}
	styles := []string(nil)
	if target == TargetSite {
		styles = cfg.Stylesheets
	}
	return PreviewDocument(RenderOutput{HTML: cfg.Draft.Preview.HTML, Text: cfg.Draft.Preview.Text}, target, cfg.Draft.Locale, styles)
}

func lockedPart(id, label, source string) gosx.Node {
	return gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__locked"), gosx.Attr("id", id)), gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__locked-label")), gosx.Text(label)), gosx.El("pre", gosx.Text(source)))
}

func conflictMarkup(cfg EditorConfig, labels EditorLabels, conflict EditorConflict, title gosx.Node) gosx.Node {
	updated := ""
	if !conflict.Saved.UpdatedAt.IsZero() {
		zone := cfg.Zone
		if zone == nil {
			zone = time.UTC
		}
		updated = " at " + conflict.Saved.UpdatedAt.In(zone).Format("15:04")
	}
	copy := fmt.Sprintf("Your text is still in the editor and is not saved. The saved version is revision %d", conflict.Saved.Revision)
	if conflict.Saved.UpdatedBy != "" {
		copy += ", by " + conflict.Saved.UpdatedBy
	}
	copy += updated + "."
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-editor__conflict"), gosx.Attr("role", "alert"), gosx.Attr("aria-labelledby", cfg.ID+"-conflict-title"), gosx.Attr("data-gxa-conflict", "true")), title,
		gosx.El("p", gosx.Text(copy)), gosx.El("label", gosx.Attrs(gosx.Attr("for", cfg.ID+"-theirs")), gosx.Text("Saved text")),
		gosx.El("textarea", gosx.Attrs(gosx.Attr("id", cfg.ID+"-theirs"), gosx.Attr("readonly", ""), gosx.Attr("rows", "8")), gosx.Text(conflict.Saved.Source)),
		gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__conflict-native")), gosx.Text("Saving now makes your text the new version.")),
		gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-editor__conflict-actions"), gosx.BoolAttr("hidden")),
			gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("data-gxa-resolve", "mine")), gosx.Text(labels.KeepMine)),
			gosx.El("button", gosx.Attrs(gosx.Attr("type", "button"), gosx.Attr("data-gxa-resolve", "theirs")), gosx.Text(labels.UseTheirs))))
}

func problemsMarkup(cfg EditorConfig, labels EditorLabels, heading func(int, string, string, string) gosx.Node) gosx.Node {
	count := fmt.Sprintf("(%d)", len(cfg.Analysis.Diagnostics))
	items := make([]gosx.Node, 0, len(cfg.Analysis.Diagnostics))
	for _, d := range cfg.Analysis.Diagnostics {
		class := "gxa-editor__problem"
		if d.Severity != "" {
			class += " gxa-editor__problem--" + string(d.Severity)
		}
		items = append(items, gosx.El("li", gosx.Attrs(gosx.Attr("class", class)), gosx.El("span", gosx.Attrs(gosx.Attr("data-from", d.From), gosx.Attr("data-to", d.To), gosx.Attr("data-code", d.Code)), gosx.Text(fmt.Sprintf("Line %d: %s", d.Line, d.Message)))))
	}
	if len(items) == 0 {
		items = append(items, gosx.El("li", gosx.Attrs(gosx.Attr("class", "gxa-editor__problem-empty")), gosx.Text("No problems found.")))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-editor__problems"), gosx.Attr("aria-labelledby", cfg.ID+"-problems-title")),
		heading(cfg.HeadingLevel+1, cfg.ID+"-problems-title", "", labels.Problems),
		gosx.El("span", gosx.Attrs(gosx.Attr("id", cfg.ID+"-problems-count"), gosx.Attr("class", "gxa-editor__count")), gosx.Text(count)),
		gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-editor__fallback")), gosx.Text("Checks update with the preview.")),
		gosx.El("ul", gosx.Attrs(gosx.Attr("class", "gxa-editor__problem-list")), gosx.Fragment(items...)))
}

func historyMarkup(cfg EditorConfig, labels EditorLabels, heading func(int, string, string, string) gosx.Node) gosx.Node {
	items := make([]gosx.Node, 0, len(cfg.Versions))
	zone := cfg.Zone
	if zone == nil {
		zone = time.UTC
	}
	for _, v := range cfg.Versions {
		stamp := v.At.In(zone).Format("15:04")
		items = append(items, gosx.El("li", gosx.El("time", gosx.Attrs(gosx.Attr("datetime", v.At.UTC().Format(time.RFC3339))), gosx.Text(stamp)), gosx.Text(" "+v.Author+", "+string(v.Kind)+" "),
			gosx.El("button", gosx.Attrs(gosx.Attr("type", "submit"), gosx.Attr("formaction", cfg.RestoreURL), gosx.Attr("name", FieldVersion), gosx.Attr("value", v.ID)), gosx.Text(labels.Restore))))
	}
	if len(items) == 0 {
		items = append(items, gosx.El("li", gosx.Text("No saved versions yet.")))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-editor__history"), gosx.Attr("aria-labelledby", cfg.ID+"-history-title")), heading(cfg.HeadingLevel+1, cfg.ID+"-history-title", "", labels.History), gosx.El("ol", gosx.Attrs(gosx.Attr("class", "gxa-editor__versions")), gosx.Fragment(items...)))
}

func hiddenInput(name, value string) gosx.Node {
	return gosx.El("input", gosx.Attrs(gosx.Attr("type", "hidden"), gosx.Attr("name", name), gosx.Attr("value", value)))
}
func targetLabel(t Target) string {
	switch t {
	case TargetSite:
		return "Site"
	case TargetEmail:
		return "Email"
	case TargetText:
		return "Text"
	default:
		return string(t)
	}
}

func mergeLabels(in EditorLabels) EditorLabels {
	out := defaultEditorLabels
	values := []struct {
		dst *string
		src string
	}{{&out.Source, in.Source}, {&out.Preview, in.Preview}, {&out.Problems, in.Problems}, {&out.History, in.History}, {&out.Insert, in.Insert}, {&out.Save, in.Save}, {&out.Saved, in.Saved}, {&out.Saving, in.Saving}, {&out.Unsaved, in.Unsaved}, {&out.Offline, in.Offline}, {&out.Conflict, in.Conflict}, {&out.KeepMine, in.KeepMine}, {&out.UseTheirs, in.UseTheirs}, {&out.Restore, in.Restore}, {&out.ShowPane, in.ShowPane}, {&out.LockedAbove, in.LockedAbove}, {&out.LockedBelow, in.LockedBelow}}
	for _, v := range values {
		if v.src != "" {
			*v.dst = v.src
		}
	}
	return out
}

func labelsJSON(v EditorLabels) editorLabelsJSON {
	return editorLabelsJSON{v.Source, v.Preview, v.Problems, v.History, v.Insert, v.Save, v.Saved, v.Saving, v.Unsaved, v.Offline, v.Conflict, v.KeepMine, v.UseTheirs, v.Restore, v.ShowPane, v.LockedAbove, v.LockedBelow}
}

func newPageKey() string { return randomID() }

// Keep a page's script URL content-addressed so immutable browser caches are safe.
func EditorScriptURL() string { return AssetPrefix + "editor.js?v=" + assetHash(editorScript) }
func WorkerScriptURL() string { return AssetPrefix + "lint-worker.js?v=" + assetHash(lintWorkerScript) }
func assetHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

var _ http.Handler = Assets()
