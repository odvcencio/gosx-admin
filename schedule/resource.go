package schedule

import (
	"strings"
	"time"
)

// Resource is something events use, such as a room or a program.
type Resource struct {
	ID string
	// Kind is a consumer-defined label.
	Kind    string
	Label   string
	Summary string
	// Capacity is the default capacity for new events on this resource.
	Capacity int
	Archived bool
	Revision Revision
	Created  time.Time
	Updated  time.Time
	Meta     map[string]string
}

// NormalizeResource trims text fields and validates: ID and Label are
// required, Capacity >= 0. It returns a *ValidationError keyed by field.
func NormalizeResource(r Resource) (Resource, error) {
	r.ID = strings.TrimSpace(r.ID)
	r.Kind = strings.TrimSpace(r.Kind)
	r.Label = strings.TrimSpace(r.Label)
	r.Summary = strings.TrimSpace(r.Summary)
	fields := map[string]string{}
	if r.ID == "" {
		fields["id"] = "Enter an ID."
	}
	if r.Label == "" {
		fields["label"] = "Enter a label."
	}
	if r.Capacity < 0 {
		fields["capacity"] = "Capacity cannot be negative."
	}
	if len(fields) != 0 {
		return Resource{}, &ValidationError{Fields: fields}
	}
	return r, nil
}
