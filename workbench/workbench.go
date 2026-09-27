package workbench

type FieldKind string

const (
	FieldText     FieldKind = "text"
	FieldSlug     FieldKind = "slug"
	FieldTextarea FieldKind = "textarea"
	FieldMoney    FieldKind = "money"
	FieldBoolean  FieldKind = "boolean"
	FieldSelect   FieldKind = "select"
	FieldImage    FieldKind = "image"
	FieldDateTime FieldKind = "datetime"
	FieldRelation FieldKind = "relation"
)

type Field struct {
	Name     string
	Label    string
	Kind     FieldKind
	Required bool
	ReadOnly bool
	Options  []string
	// Help is one sentence shown under the label. Renderers link it to the
	// control with aria-describedby.
	Help string
	// MaxLength is the largest number of characters (Unicode code points)
	// the field accepts. Zero means no limit.
	MaxLength int
}

type Column struct {
	Name  string
	Label string
	Kind  FieldKind
}

type Action struct {
	Name        string
	Label       string
	Description string
	Kind        string
	// Confirm marks an action whose effects the operator must confirm before
	// it runs, for example one that changes money, capacity, a published
	// schedule, or a recipient list. workbench/render renders a confirmation
	// step for it, and render.Guard rejects a request that skips the step.
	Confirm bool
}

type Resource struct {
	Slug         string
	Label        string
	Singular     string
	Description  string
	Route        string
	Count        int
	Mutable      bool
	Generated    bool
	Capabilities []string
	Columns      []Column
	Fields       []Field
	Actions      []Action
}

type Tool struct {
	Slug        string
	Label       string
	Description string
	Route       string
	Kind        string
	Actions     []Action
}

type Workspace struct {
	Resources []Resource
	Tools     []Tool
}
