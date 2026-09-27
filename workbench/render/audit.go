package render

import (
	"context"
	"time"
)

// AuditOutcome classifies an audited action attempt.
type AuditOutcome string

const (
	// AuditOK marks a successful action.
	AuditOK AuditOutcome = "ok"
	// AuditDenied marks an action blocked by session or permission checks.
	AuditDenied AuditOutcome = "denied"
	// AuditInvalid marks malformed or invalid input.
	AuditInvalid AuditOutcome = "invalid"
	// AuditConflict marks a stale revision.
	AuditConflict AuditOutcome = "conflict"
	// AuditFailed marks a dependency or store failure.
	AuditFailed AuditOutcome = "failed"
)

// AuditEvent describes one action attempt. It holds identifiers only, never
// field values, so an audit log cannot leak private data.
type AuditEvent struct {
	// At is the attempt time.
	At time.Time
	// RequestID is server.RequestID(r).
	RequestID string
	// ActorID is the signed-in principal identifier.
	ActorID  string
	Resource string
	Action   string
	RecordID string
	Key      string
	// Revision is the revision the request expected.
	Revision int64
	// NewRevision is set by the handler on success.
	NewRevision int64
	Outcome     AuditOutcome
	// Reason is a short code such as "csrf", "forbidden", "key", "revision",
	// "confirm", "fields", "conflict", "not_found", or "error".
	Reason string
}

// Auditor records audit events. Guard calls it for denied, invalid, conflict,
// and failed attempts. The action handler records AuditOK in its own
// transaction using ActionInput.Audit.
type Auditor interface {
	Record(ctx context.Context, e AuditEvent) error
}

// AuditFunc adapts a function to Auditor.
type AuditFunc func(ctx context.Context, e AuditEvent) error

// Record calls f.
func (f AuditFunc) Record(ctx context.Context, e AuditEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, e)
}
