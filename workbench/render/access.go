package render

import "context"

// Principal is the signed-in operator. Roles are consumer-defined strings.
type Principal struct {
	ID    string
	Roles []string
}

// FieldAccess says how one field appears to a principal. The zero value hides
// the field, so a missing decision fails closed.
type FieldAccess int

const (
	// FieldHidden omits the field: no label, no value, no input.
	FieldHidden FieldAccess = iota
	// FieldRedacted shows the label and the text "Hidden" in place of the
	// value. A form renders no control for it, so it is never posted.
	FieldRedacted
	// FieldVisible shows the field normally.
	FieldVisible
)

// Authorizer decides, on the server, what a principal may see and do.
// Resource is a workbench.Resource or workbench.Tool slug. Implementations
// must be safe for concurrent use. Record-level scoping belongs in the store.
type Authorizer interface {
	// CanRead reports whether p may see the resource or tool, including its
	// navigation entry.
	CanRead(ctx context.Context, p Principal, resource string) bool
	// CanAct reports whether p may run the named action on the resource.
	CanAct(ctx context.Context, p Principal, resource, action string) bool
	// Field reports how the named field of the resource appears to p.
	Field(ctx context.Context, p Principal, resource, field string) FieldAccess
}

// Access pairs a principal with the Authorizer that judges it. A nil
// Authorizer denies every read and action and hides every field.
type Access struct {
	// Principal is the signed-in operator being checked.
	Principal Principal
	// Authorizer judges reads, actions, and field visibility.
	Authorizer Authorizer
}

// CanRead reports whether a.Principal may read resource. It returns false
// when a.Authorizer is nil.
func (a Access) CanRead(ctx context.Context, resource string) bool {
	return a.Authorizer != nil && a.Authorizer.CanRead(ctx, a.Principal, resource)
}

// CanAct reports whether a.Principal may run action on resource. It returns
// false when a.Authorizer is nil.
func (a Access) CanAct(ctx context.Context, resource, action string) bool {
	return a.Authorizer != nil && a.Authorizer.CanAct(ctx, a.Principal, resource, action)
}

// Field reports how field of resource appears to a.Principal. It returns
// FieldHidden when a.Authorizer is nil.
func (a Access) Field(ctx context.Context, resource, field string) FieldAccess {
	if a.Authorizer == nil {
		return FieldHidden
	}
	return a.Authorizer.Field(ctx, a.Principal, resource, field)
}

func validFieldAccess(access FieldAccess) bool {
	return access == FieldHidden || access == FieldRedacted || access == FieldVisible
}

// Rule grants one role access to one resource. Policy combines rules.
type Rule struct {
	// Role is the principal role this rule applies to.
	Role string
	// Resource is a slug, or "*" for every resource and tool.
	Resource string
	// Read allows reading the resource.
	Read bool
	// Actions lists action names the role may run, or "*" for all.
	Actions []string
	// Redact lists fields shown as "Hidden".
	Redact []string
	// Hide lists fields omitted entirely.
	Hide []string
}

// Policy is a table-driven Authorizer. A rule matches when the principal has
// rule.Role and rule.Resource equals the resource or "*". CanRead is true
// when any matching rule has Read. CanAct is true when any matching rule has
// Read and lists the action or "*". Field uses the most permissive result
// across matching rules with Read: visible beats redacted, which beats hidden.
// One rule hides names in Hide, redacts names in Redact, and shows other
// fields. No matching readable rule yields FieldHidden.
type Policy struct{ Rules []Rule }

var _ Authorizer = Policy{}

func (p Policy) matching(pr Principal, resource string) []Rule {
	roles := make(map[string]struct{}, len(pr.Roles))
	for _, role := range pr.Roles {
		roles[role] = struct{}{}
	}
	var matches []Rule
	for _, rule := range p.Rules {
		if _, ok := roles[rule.Role]; ok && (rule.Resource == resource || rule.Resource == "*") {
			matches = append(matches, rule)
		}
	}
	return matches
}

// CanRead implements Authorizer.
func (p Policy) CanRead(_ context.Context, pr Principal, resource string) bool {
	for _, rule := range p.matching(pr, resource) {
		if rule.Read {
			return true
		}
	}
	return false
}

// CanAct implements Authorizer.
func (p Policy) CanAct(_ context.Context, pr Principal, resource, action string) bool {
	for _, rule := range p.matching(pr, resource) {
		if !rule.Read {
			continue
		}
		for _, name := range rule.Actions {
			if name == action || name == "*" {
				return true
			}
		}
	}
	return false
}

// Field implements Authorizer.
func (p Policy) Field(_ context.Context, pr Principal, resource, field string) FieldAccess {
	result := FieldHidden
	for _, rule := range p.matching(pr, resource) {
		if !rule.Read {
			continue
		}
		access := FieldVisible
		if contains(rule.Hide, field) {
			access = FieldHidden
		} else if contains(rule.Redact, field) {
			access = FieldRedacted
		}
		if access > result {
			result = access
		}
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
