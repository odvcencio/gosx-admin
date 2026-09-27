package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Errors returned by stores and operations. Stores return the first five;
// operations return all of them. Test with errors.Is.
var (
	ErrNotFound = errors.New("schedule: not found")
	ErrExists   = errors.New("schedule: already exists")
	// ErrConflict: a revision precondition failed; nothing was written.
	ErrConflict = errors.New("schedule: revision changed")
	// ErrDuplicateKey: a store already holds a receipt for the key; nothing
	// was written.
	ErrDuplicateKey = errors.New("schedule: idempotency key already stored")
	ErrInvalid      = errors.New("schedule: invalid input")
	// ErrKeyReused: the key's receipt belongs to a different operation or
	// record.
	ErrKeyReused   = errors.New("schedule: idempotency key used for another request")
	ErrFull        = errors.New("schedule: no seats left")
	ErrClosed      = errors.New("schedule: booking is closed")
	ErrHoldExpired = errors.New("schedule: hold expired")
	// ErrTransition: the registration's state does not allow the operation.
	ErrTransition = errors.New("schedule: state change not allowed")
)

// ValidationError lists invalid fields. errors.Is(err, ErrInvalid) is true.
type ValidationError struct {
	// Fields maps a field name to a plain message, for example
	// "zone": "Choose a time zone such as America/Los_Angeles."
	Fields map[string]string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if e == nil {
		return ErrInvalid.Error()
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	if len(parts) == 0 {
		return ErrInvalid.Error()
	}
	return ErrInvalid.Error() + ": " + strings.Join(parts, "; ")
}

// Is reports whether target is ErrInvalid.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalid }
