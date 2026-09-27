package mdppstudio

import "context"

// MediaRef is one image the source uses.
type MediaRef struct {
	URL string
	Alt string
	// From and To locate it in the source (UTF-16 offsets).
	From int
	To   int
}

// MediaUse says where the source is going.
type MediaUse struct {
	DraftID string
	Target  Target
	Locale  string
	Kind    string
}

// MediaDecision is the consumer's answer for one image.
type MediaDecision struct {
	Allowed  bool
	Reason   string
	Severity Severity
}

// MediaPolicy decides whether an image may be used. A missing permission
// record must return Allowed false.
type MediaPolicy interface {
	Check(ctx context.Context, ref MediaRef, use MediaUse) (MediaDecision, error)
}

// MediaPolicyFunc adapts a function to MediaPolicy.
type MediaPolicyFunc func(ctx context.Context, ref MediaRef, use MediaUse) (MediaDecision, error)

// Check calls f.
func (f MediaPolicyFunc) Check(ctx context.Context, ref MediaRef, use MediaUse) (MediaDecision, error) {
	return f(ctx, ref, use)
}

// CheckMedia runs policy on every image Link in a and returns one diagnostic
// with Code "media-permission" per image that is not Allowed. A nil policy
// returns nil. A policy error is returned as is.
func CheckMedia(ctx context.Context, policy MediaPolicy, a Analysis, use MediaUse) ([]Diagnostic, error) {
	if policy == nil {
		return nil, nil
	}
	var out []Diagnostic
	for _, link := range a.Links {
		if link.Kind != "image" {
			continue
		}
		decision, err := policy.Check(ctx, MediaRef{URL: link.Href, Alt: link.Alt, From: link.From, To: link.To}, use)
		if err != nil {
			return out, err
		}
		if decision.Allowed {
			continue
		}
		severity := decision.Severity
		if severity == "" {
			severity = SeverityError
		}
		message := decision.Reason
		if message == "" {
			message = "This image needs a recorded permission before it can be used."
		}
		out = append(out, Diagnostic{From: link.From, To: link.To, Severity: severity, Code: "media-permission", Message: message})
	}
	return out, nil
}
