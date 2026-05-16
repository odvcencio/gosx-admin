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
