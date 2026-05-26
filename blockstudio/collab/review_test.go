package collab

import (
	"strings"
	"testing"

	"m31labs.dev/gosx-admin/blockstudio"
)

func TestReviewTransactionSummarizesHumanEdits(t *testing.T) {
	review := ReviewTransaction(testDocument(), Transaction{
		ID:    "tx-human",
		Actor: Actor{ID: "owner", Kind: ActorHuman, DisplayName: "Owner"},
		Operations: []Operation{
			{
				ID:      "title",
				Clock:   "01",
				Kind:    OpSetText,
				Target:  Target{BlockID: "hero", Field: "headline"},
				Payload: Payload(SetTextPayload{BlockID: "hero", Field: "headline", Text: "A better hero"}),
			},
			{
				ID:      "move",
				Clock:   "02",
				Kind:    OpMoveBlock,
				Target:  Target{BlockID: "contact"},
				Payload: Payload(MoveBlockPayload{BlockID: "contact", After: "hero"}),
			},
		},
	})
	if review.RequiresReview {
		t.Fatalf("human edit should not require review by default: %#v", review)
	}
	if review.Summary != "Owner prepared 2 operations." {
		t.Fatalf("unexpected summary: %q", review.Summary)
	}
	if len(review.Items) != 2 || review.Items[0].Summary != "Edit headline on hero block" || review.Items[1].Summary != "Move contact block after hero" {
		t.Fatalf("unexpected review items: %#v", review.Items)
	}
}

func TestReviewTransactionMarksAgentSuggestionsForReview(t *testing.T) {
	suggested := Operation{
		ID:      "agent-style",
		Clock:   "02.1",
		Kind:    OpSetStyle,
		Target:  Target{BlockID: "hero", Field: "spacing"},
		Payload: Payload(SetStylePayload{BlockID: "hero", Key: "spacing", Value: blockstudio.Value{Kind: blockstudio.FieldSelect, String: "airy"}}),
	}
	review := ReviewTransaction(testDocument(), Transaction{
		ID:    "tx-agent",
		Actor: Actor{ID: "cedar", Kind: ActorAgent, DisplayName: "Cedar"},
		Operations: []Operation{{
			ID:        "proposal",
			Clock:     "02",
			Kind:      OpSuggest,
			ActorKind: ActorAgent,
			Target:    Target{BlockID: "hero"},
			Payload: Payload(SuggestPayload{
				Title:      "Make the hero roomier",
				Summary:    "Increase spacing for a calmer first impression.",
				Operations: []Operation{suggested},
			}),
		}},
	})
	if !review.RequiresReview {
		t.Fatalf("agent proposal should require review: %#v", review)
	}
	if review.Summary != "Cedar proposed 1 operation for review." {
		t.Fatalf("unexpected summary: %q", review.Summary)
	}
	if len(review.Items) != 1 || review.Items[0].Summary != "Review suggestion: Make the hero roomier" {
		t.Fatalf("unexpected items: %#v", review.Items)
	}
}

func TestReviewTransactionReportsMissingTargets(t *testing.T) {
	review := ReviewOperations(testDocument(), Actor{ID: "owner", Kind: ActorHuman}, []Operation{{
		ID:      "missing-edit",
		Clock:   "01",
		Kind:    OpSetText,
		Target:  Target{BlockID: "missing", Field: "body"},
		Payload: Payload(SetTextPayload{BlockID: "missing", Field: "body", Text: "No target"}),
	}})
	if len(review.Findings) != 1 || review.Findings[0].Severity != ReviewWarning {
		t.Fatalf("expected warning finding, got %#v", review.Findings)
	}
	if !strings.Contains(review.Findings[0].Message, "missing") {
		t.Fatalf("unexpected finding message: %#v", review.Findings[0])
	}
	if review.Items[0].Summary != "Edit body on missing block missing" {
		t.Fatalf("unexpected missing-target item: %#v", review.Items[0])
	}
}
