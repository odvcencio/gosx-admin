package calendar

import (
	"sort"
	"strings"
	"time"
)

type WidgetState string
type WidgetControlKind string
type WidgetReadinessStatus string

const (
	WidgetStateOpen      WidgetState = "open"
	WidgetStateFull      WidgetState = "full"
	WidgetStateWaitlist  WidgetState = "waitlist"
	WidgetStateClosed    WidgetState = "closed"
	WidgetStateSelected  WidgetState = "selected"
	WidgetStateToday     WidgetState = "today"
	WidgetStateStaffOnly WidgetState = "staff-only"
	WidgetStateDraft     WidgetState = "draft"

	WidgetControlChoice     WidgetControlKind = "choice"
	WidgetControlDensity    WidgetControlKind = "density"
	WidgetControlShape      WidgetControlKind = "shape"
	WidgetControlMode       WidgetControlKind = "mode"
	WidgetControlVisibility WidgetControlKind = "visibility"

	WidgetReadinessReady WidgetReadinessStatus = "ready"
	WidgetReadinessWatch WidgetReadinessStatus = "watch"
	WidgetReadinessNext  WidgetReadinessStatus = "next"
)

type ScheduleWidgetOptions struct {
	Key            string
	Label          string
	Summary        string
	PublicHref     string
	AdminHref      string
	RegisterAction string
}

type ScheduleWidgetContract struct {
	Key       string
	Label     string
	Summary   string
	PublicURL string
	AdminURL  string
	Recipe    WidgetRecipe
	States    []WidgetStateDescriptor
	Data      []WidgetDataField
	Actions   []WidgetAction
}

type WidgetRecipe struct {
	Key      string
	Label    string
	Summary  string
	Controls []WidgetControl
	Variants []WidgetVariant
}

type WidgetControl struct {
	Key      string
	Label    string
	Kind     WidgetControlKind
	Default  string
	Options  []WidgetOption
	Required bool
}

type WidgetOption struct {
	Value   string
	Label   string
	Summary string
}

type WidgetVariant struct {
	Key     string
	Label   string
	Values  map[string]string
	Summary string
}

type WidgetStateDescriptor struct {
	Key         WidgetState
	Label       string
	Summary     string
	ClassSuffix string
}

type WidgetDataField struct {
	Key      string
	Label    string
	Kind     string
	Required bool
}

type WidgetAction struct {
	Key        string
	Label      string
	Kind       string
	HandlerRef string
	Href       string
}

type WidgetReadinessCheck struct {
	Key         string
	Label       string
	Status      WidgetReadinessStatus
	Message     string
	ActionLabel string
}

type WidgetStateOptions struct {
	Today     time.Time
	Selected  time.Time
	Waitlist  bool
	Closed    bool
	StaffOnly bool
}

func DefaultScheduleWidgetContract(options ScheduleWidgetOptions) ScheduleWidgetContract {
	key := strings.TrimSpace(options.Key)
	if key == "" {
		key = "schedule"
	}
	label := strings.TrimSpace(options.Label)
	if label == "" {
		label = "Schedule"
	}
	summary := strings.TrimSpace(options.Summary)
	if summary == "" {
		summary = "Calendar and schedule widget for event, availability, and registration surfaces."
	}
	contract := ScheduleWidgetContract{
		Key:       key,
		Label:     label,
		Summary:   summary,
		PublicURL: strings.TrimSpace(options.PublicHref),
		AdminURL:  strings.TrimSpace(options.AdminHref),
		Recipe: WidgetRecipe{
			Key:     key + "-widget",
			Label:   label + " widget",
			Summary: "Controls for calendar density, day cells, availability emphasis, mobile behavior, and staff visibility.",
			Controls: []WidgetControl{
				{
					Key:      "density",
					Label:    "Density",
					Kind:     WidgetControlDensity,
					Default:  "balanced",
					Required: true,
					Options: []WidgetOption{
						{Value: "compact", Label: "Compact", Summary: "Fits more sessions into each view."},
						{Value: "balanced", Label: "Balanced", Summary: "Readable default spacing for public schedules."},
						{Value: "airy", Label: "Airy", Summary: "More breathing room for family-facing pages."},
					},
				},
				{
					Key:      "day-cell-shape",
					Label:    "Day cell shape",
					Kind:     WidgetControlShape,
					Default:  "soft",
					Required: true,
					Options: []WidgetOption{
						{Value: "square", Label: "Square"},
						{Value: "soft", Label: "Soft"},
						{Value: "round", Label: "Round"},
					},
				},
				{
					Key:      "availability-emphasis",
					Label:    "Availability",
					Kind:     WidgetControlChoice,
					Default:  "badges",
					Required: true,
					Options: []WidgetOption{
						{Value: "quiet", Label: "Quiet"},
						{Value: "badges", Label: "Badges"},
						{Value: "filled", Label: "Filled"},
					},
				},
				{
					Key:      "mobile-mode",
					Label:    "Mobile mode",
					Kind:     WidgetControlMode,
					Default:  "agenda",
					Required: true,
					Options: []WidgetOption{
						{Value: "grid", Label: "Grid"},
						{Value: "agenda", Label: "Agenda"},
						{Value: "compact", Label: "Compact"},
					},
				},
				{
					Key:      "staff-visibility",
					Label:    "Staff visibility",
					Kind:     WidgetControlVisibility,
					Default:  "public",
					Required: true,
					Options: []WidgetOption{
						{Value: "public", Label: "Public"},
						{Value: "staff", Label: "Staff only"},
						{Value: "all", Label: "Public and staff"},
					},
				},
			},
			Variants: []WidgetVariant{
				{Key: "public-month", Label: "Public month", Values: map[string]string{"density": "balanced", "availability-emphasis": "badges", "mobile-mode": "agenda"}},
				{Key: "staff-board", Label: "Staff board", Values: map[string]string{"density": "compact", "availability-emphasis": "filled", "staff-visibility": "all"}},
			},
		},
		States: defaultWidgetStates(),
		Data: []WidgetDataField{
			{Key: "event.id", Label: "Event ID", Kind: "string", Required: true},
			{Key: "event.title", Label: "Title", Kind: "string", Required: true},
			{Key: "event.start", Label: "Start", Kind: "datetime", Required: true},
			{Key: "event.end", Label: "End", Kind: "datetime"},
			{Key: "event.status", Label: "Status", Kind: "status", Required: true},
			{Key: "event.capacity", Label: "Capacity", Kind: "number"},
			{Key: "event.registered", Label: "Registered", Kind: "number"},
			{Key: "event.resource", Label: "Resource", Kind: "relation"},
			{Key: "registration.status", Label: "Registration status", Kind: "status"},
			{Key: "registration.quantity", Label: "Registration quantity", Kind: "number"},
		},
		Actions: []WidgetAction{
			{Key: "register", Label: "Register", Kind: "server-action", HandlerRef: strings.TrimSpace(options.RegisterAction), Href: strings.TrimSpace(options.PublicHref)},
			{Key: "manage", Label: "Manage schedule", Kind: "admin-route", Href: strings.TrimSpace(options.AdminHref)},
		},
	}
	return NormalizeScheduleWidgetContract(contract)
}

func NormalizeScheduleWidgetContract(contract ScheduleWidgetContract) ScheduleWidgetContract {
	contract.Key = token(firstNonEmpty(contract.Key, "schedule"))
	contract.Label = strings.TrimSpace(firstNonEmpty(contract.Label, "Schedule"))
	contract.Summary = strings.TrimSpace(contract.Summary)
	contract.PublicURL = strings.TrimSpace(contract.PublicURL)
	contract.AdminURL = strings.TrimSpace(contract.AdminURL)
	contract.Recipe = normalizeWidgetRecipe(contract.Recipe, contract.Key)
	contract.States = normalizeWidgetStates(contract.States)
	contract.Data = normalizeWidgetDataFields(contract.Data)
	contract.Actions = normalizeWidgetActions(contract.Actions)
	return contract
}

func ScheduleWidgetReadiness(contract ScheduleWidgetContract) []WidgetReadinessCheck {
	contract = NormalizeScheduleWidgetContract(contract)
	checks := []WidgetReadinessCheck{
		{
			Key:         "recipe",
			Label:       "Recipe controls",
			Status:      WidgetReadinessReady,
			Message:     "Calendar density, cell shape, availability, mobile, and staff controls are registered.",
			ActionLabel: "Review controls",
		},
		{
			Key:         "states",
			Label:       "Availability states",
			Status:      WidgetReadinessReady,
			Message:     "Open, full, waitlist, closed, selected, today, staff-only, and draft states are modeled.",
			ActionLabel: "Review states",
		},
		{
			Key:         "data",
			Label:       "Schedule data",
			Status:      WidgetReadinessReady,
			Message:     "Event, resource, capacity, and registration fields are described.",
			ActionLabel: "Review fields",
		},
		{
			Key:         "actions",
			Label:       "Routes and actions",
			Status:      WidgetReadinessReady,
			Message:     "Public route, registration action, and admin route are connected.",
			ActionLabel: "Open schedule",
		},
	}
	if len(contract.Recipe.Controls) == 0 {
		checks[0].Status = WidgetReadinessNext
		checks[0].Message = "Add recipe controls before using this widget in Studio."
	}
	if !hasAllWidgetStates(contract.States) {
		checks[1].Status = WidgetReadinessNext
		checks[1].Message = "Add every required availability state before publishing schedule widgets."
	}
	if !hasRequiredWidgetData(contract.Data) {
		checks[2].Status = WidgetReadinessNext
		checks[2].Message = "Event ID, title, start, and status fields are required."
	}
	if !hasExecutableScheduleAction(contract.Actions) || contract.PublicURL == "" || contract.AdminURL == "" {
		checks[3].Status = WidgetReadinessWatch
		checks[3].Message = "Connect the public schedule route, admin route, and registration handler before launch."
	}
	return checks
}

func ScheduleWidgetContractView(contract ScheduleWidgetContract) map[string]any {
	contract = NormalizeScheduleWidgetContract(contract)
	return map[string]any{
		"key":       contract.Key,
		"label":     contract.Label,
		"summary":   contract.Summary,
		"publicURL": contract.PublicURL,
		"adminURL":  contract.AdminURL,
		"recipe":    widgetRecipeView(contract.Recipe),
		"states":    widgetStateViews(contract.States),
		"data":      widgetDataFieldViews(contract.Data),
		"actions":   widgetActionViews(contract.Actions),
		"readiness": widgetReadinessViews(ScheduleWidgetReadiness(contract)),
	}
}

func WidgetStatesForEvent(event Event, options WidgetStateOptions) []WidgetState {
	event, ok := NormalizeEvent(event)
	if !ok {
		return nil
	}
	states := map[WidgetState]bool{}
	status := WidgetState(statusToken(event.Status))
	switch status {
	case WidgetStateDraft:
		states[WidgetStateDraft] = true
	case WidgetStateClosed:
		states[WidgetStateClosed] = true
	case WidgetStateFull:
		states[WidgetStateFull] = true
	case WidgetStateWaitlist:
		states[WidgetStateWaitlist] = true
	default:
		if event.Status == StatusCancelled {
			states[WidgetStateClosed] = true
		}
	}
	if options.Closed {
		states[WidgetStateClosed] = true
		delete(states, WidgetStateOpen)
	}
	if !states[WidgetStateDraft] && !states[WidgetStateClosed] && EventAvailability(event).Full {
		states[WidgetStateFull] = true
	}
	if options.Waitlist && !states[WidgetStateDraft] && !states[WidgetStateClosed] {
		states[WidgetStateWaitlist] = true
	}
	if !states[WidgetStateDraft] && !states[WidgetStateClosed] && !states[WidgetStateFull] && !states[WidgetStateWaitlist] {
		states[WidgetStateOpen] = true
	}
	if options.StaffOnly {
		states[WidgetStateStaffOnly] = true
	}
	if !options.Today.IsZero() && sameDay(event.Start, options.Today) {
		states[WidgetStateToday] = true
	}
	if !options.Selected.IsZero() && sameDay(event.Start, options.Selected) {
		states[WidgetStateSelected] = true
	}
	return orderedWidgetStates(states)
}

func normalizeWidgetRecipe(recipe WidgetRecipe, fallbackKey string) WidgetRecipe {
	recipe.Key = token(firstNonEmpty(recipe.Key, fallbackKey+"-widget"))
	recipe.Label = strings.TrimSpace(firstNonEmpty(recipe.Label, "Schedule widget"))
	recipe.Summary = strings.TrimSpace(recipe.Summary)
	recipe.Controls = normalizeWidgetControls(recipe.Controls)
	recipe.Variants = normalizeWidgetVariants(recipe.Variants)
	return recipe
}

func normalizeWidgetControls(controls []WidgetControl) []WidgetControl {
	out := make([]WidgetControl, 0, len(controls))
	seen := map[string]bool{}
	for _, control := range controls {
		control.Key = token(control.Key)
		control.Label = strings.TrimSpace(control.Label)
		control.Default = token(control.Default)
		if control.Key == "" || seen[control.Key] {
			continue
		}
		if control.Label == "" {
			control.Label = labelize(control.Key)
		}
		if control.Kind == "" {
			control.Kind = WidgetControlChoice
		}
		control.Options = normalizeWidgetOptions(control.Options)
		out = append(out, control)
		seen[control.Key] = true
	}
	return out
}

func normalizeWidgetOptions(options []WidgetOption) []WidgetOption {
	out := make([]WidgetOption, 0, len(options))
	seen := map[string]bool{}
	for _, option := range options {
		option.Value = token(option.Value)
		option.Label = strings.TrimSpace(option.Label)
		option.Summary = strings.TrimSpace(option.Summary)
		if option.Value == "" || seen[option.Value] {
			continue
		}
		if option.Label == "" {
			option.Label = labelize(option.Value)
		}
		out = append(out, option)
		seen[option.Value] = true
	}
	return out
}

func normalizeWidgetVariants(variants []WidgetVariant) []WidgetVariant {
	out := make([]WidgetVariant, 0, len(variants))
	seen := map[string]bool{}
	for _, variant := range variants {
		variant.Key = token(variant.Key)
		variant.Label = strings.TrimSpace(variant.Label)
		variant.Summary = strings.TrimSpace(variant.Summary)
		if variant.Key == "" || seen[variant.Key] {
			continue
		}
		if variant.Label == "" {
			variant.Label = labelize(variant.Key)
		}
		variant.Values = normalizeStringMap(variant.Values)
		out = append(out, variant)
		seen[variant.Key] = true
	}
	return out
}

func normalizeWidgetStates(states []WidgetStateDescriptor) []WidgetStateDescriptor {
	if len(states) == 0 {
		states = defaultWidgetStates()
	}
	out := make([]WidgetStateDescriptor, 0, len(states))
	seen := map[WidgetState]bool{}
	for _, state := range states {
		state.Key = WidgetState(token(string(state.Key)))
		state.Label = strings.TrimSpace(state.Label)
		state.Summary = strings.TrimSpace(state.Summary)
		state.ClassSuffix = token(firstNonEmpty(state.ClassSuffix, string(state.Key)))
		if state.Key == "" || seen[state.Key] {
			continue
		}
		if state.Label == "" {
			state.Label = labelize(string(state.Key))
		}
		out = append(out, state)
		seen[state.Key] = true
	}
	sort.SliceStable(out, func(i, j int) bool {
		return widgetStateRank(out[i].Key) < widgetStateRank(out[j].Key)
	})
	return out
}

func normalizeWidgetDataFields(fields []WidgetDataField) []WidgetDataField {
	out := make([]WidgetDataField, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		field.Label = strings.TrimSpace(field.Label)
		field.Kind = token(firstNonEmpty(field.Kind, "string"))
		if field.Key == "" || seen[field.Key] {
			continue
		}
		if field.Label == "" {
			field.Label = labelize(strings.ReplaceAll(field.Key, ".", "-"))
		}
		out = append(out, field)
		seen[field.Key] = true
	}
	return out
}

func normalizeWidgetActions(actions []WidgetAction) []WidgetAction {
	out := make([]WidgetAction, 0, len(actions))
	seen := map[string]bool{}
	for _, action := range actions {
		action.Key = token(action.Key)
		action.Label = strings.TrimSpace(action.Label)
		action.Kind = token(firstNonEmpty(action.Kind, "route"))
		action.HandlerRef = strings.TrimSpace(action.HandlerRef)
		action.Href = strings.TrimSpace(action.Href)
		if action.Key == "" || seen[action.Key] {
			continue
		}
		if action.Label == "" {
			action.Label = labelize(action.Key)
		}
		out = append(out, action)
		seen[action.Key] = true
	}
	return out
}

func hasAllWidgetStates(states []WidgetStateDescriptor) bool {
	seen := map[WidgetState]bool{}
	for _, state := range states {
		seen[state.Key] = true
	}
	for _, required := range requiredWidgetStates() {
		if !seen[required] {
			return false
		}
	}
	return true
}

func hasRequiredWidgetData(fields []WidgetDataField) bool {
	seen := map[string]bool{}
	for _, field := range fields {
		if field.Required {
			seen[field.Key] = true
		}
	}
	for _, required := range []string{"event.id", "event.title", "event.start", "event.status"} {
		if !seen[required] {
			return false
		}
	}
	return true
}

func hasExecutableScheduleAction(actions []WidgetAction) bool {
	for _, action := range actions {
		if action.Key == "register" && action.HandlerRef != "" && action.Href != "" {
			return true
		}
	}
	return false
}

func widgetRecipeView(recipe WidgetRecipe) map[string]any {
	return map[string]any{
		"key":      recipe.Key,
		"label":    recipe.Label,
		"summary":  recipe.Summary,
		"controls": widgetControlViews(recipe.Controls),
		"variants": widgetVariantViews(recipe.Variants),
	}
}

func widgetControlViews(controls []WidgetControl) []map[string]any {
	out := make([]map[string]any, 0, len(controls))
	for _, control := range controls {
		out = append(out, map[string]any{
			"key":      control.Key,
			"label":    control.Label,
			"kind":     string(control.Kind),
			"default":  control.Default,
			"required": control.Required,
			"options":  widgetOptionViews(control.Options),
		})
	}
	return out
}

func widgetOptionViews(options []WidgetOption) []map[string]any {
	out := make([]map[string]any, 0, len(options))
	for _, option := range options {
		out = append(out, map[string]any{
			"value":   option.Value,
			"label":   option.Label,
			"summary": option.Summary,
		})
	}
	return out
}

func widgetVariantViews(variants []WidgetVariant) []map[string]any {
	out := make([]map[string]any, 0, len(variants))
	for _, variant := range variants {
		out = append(out, map[string]any{
			"key":     variant.Key,
			"label":   variant.Label,
			"summary": variant.Summary,
			"values":  variant.Values,
		})
	}
	return out
}

func widgetStateViews(states []WidgetStateDescriptor) []map[string]any {
	out := make([]map[string]any, 0, len(states))
	for _, state := range states {
		out = append(out, map[string]any{
			"key":         string(state.Key),
			"label":       state.Label,
			"summary":     state.Summary,
			"classSuffix": state.ClassSuffix,
		})
	}
	return out
}

func widgetDataFieldViews(fields []WidgetDataField) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		out = append(out, map[string]any{
			"key":      field.Key,
			"label":    field.Label,
			"kind":     field.Kind,
			"required": field.Required,
		})
	}
	return out
}

func widgetActionViews(actions []WidgetAction) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		out = append(out, map[string]any{
			"key":        action.Key,
			"label":      action.Label,
			"kind":       action.Kind,
			"handlerRef": action.HandlerRef,
			"href":       action.Href,
		})
	}
	return out
}

func widgetReadinessViews(checks []WidgetReadinessCheck) []map[string]any {
	out := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		out = append(out, map[string]any{
			"key":         check.Key,
			"label":       check.Label,
			"status":      string(check.Status),
			"message":     check.Message,
			"actionLabel": check.ActionLabel,
		})
	}
	return out
}

func orderedWidgetStates(states map[WidgetState]bool) []WidgetState {
	out := make([]WidgetState, 0, len(states))
	for state := range states {
		out = append(out, state)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return widgetStateRank(out[i]) < widgetStateRank(out[j])
	})
	return out
}

func widgetStateRank(state WidgetState) int {
	for index, candidate := range requiredWidgetStates() {
		if state == candidate {
			return index
		}
	}
	return len(requiredWidgetStates()) + 1
}

func requiredWidgetStates() []WidgetState {
	return []WidgetState{
		WidgetStateOpen,
		WidgetStateFull,
		WidgetStateWaitlist,
		WidgetStateClosed,
		WidgetStateSelected,
		WidgetStateToday,
		WidgetStateStaffOnly,
		WidgetStateDraft,
	}
}

func defaultWidgetStates() []WidgetStateDescriptor {
	return []WidgetStateDescriptor{
		{Key: WidgetStateOpen, Label: "Open", Summary: "Registration or attendance is available."},
		{Key: WidgetStateFull, Label: "Full", Summary: "Capacity has been reached."},
		{Key: WidgetStateWaitlist, Label: "Waitlist", Summary: "Capacity is reached but waitlist registration is available."},
		{Key: WidgetStateClosed, Label: "Closed", Summary: "Registration or public visibility is closed."},
		{Key: WidgetStateSelected, Label: "Selected", Summary: "The owner or visitor has selected this day or session."},
		{Key: WidgetStateToday, Label: "Today", Summary: "The session or day is today."},
		{Key: WidgetStateStaffOnly, Label: "Staff only", Summary: "The item is visible only to internal staff views."},
		{Key: WidgetStateDraft, Label: "Draft", Summary: "The item is not published."},
	}
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		key = token(key)
		value = token(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func token(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
