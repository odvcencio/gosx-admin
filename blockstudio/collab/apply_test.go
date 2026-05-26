package collab

import (
	"testing"

	"m31labs.dev/gosx-admin/blockstudio"
)

func TestConcurrentMovesAreDeterministic(t *testing.T) {
	doc := testDocument()
	ops := []Operation{
		{
			ID:      "move-products",
			ActorID: "agent",
			Clock:   "02",
			Kind:    OpMoveBlock,
			Target:  Target{BlockID: "products"},
			Payload: Payload(MoveBlockPayload{BlockID: "products", Before: "hero"}),
		},
		{
			ID:      "move-contact",
			ActorID: "human",
			Clock:   "01",
			Kind:    OpMoveBlock,
			Target:  Target{BlockID: "contact"},
			Payload: Payload(MoveBlockPayload{BlockID: "contact", After: "hero"}),
		},
	}
	forward, err := Apply(doc, ops)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := Apply(doc, []Operation{ops[1], ops[0]})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(forward.Document); got != "products,hero,contact" {
		t.Fatalf("unexpected order %s", got)
	}
	if keys(forward.Document) != keys(reverse.Document) {
		t.Fatalf("expected deterministic ordering, got %s and %s", keys(forward.Document), keys(reverse.Document))
	}
}

func TestSetFieldAndTextUseOperationOrder(t *testing.T) {
	doc := testDocument()
	result, err := Apply(doc, []Operation{
		{
			ID:      "agent-title",
			ActorID: "agent",
			Clock:   "01",
			Kind:    OpSetField,
			Target:  Target{BlockID: "hero", Field: "headline"},
			Payload: Payload(SetFieldPayload{BlockID: "hero", Field: "headline", Value: blockstudio.Value{Kind: blockstudio.FieldText, String: "Agent draft"}}),
		},
		{
			ID:      "human-title",
			ActorID: "human",
			Clock:   "02",
			Kind:    OpSetText,
			Target:  Target{BlockID: "hero", Field: "headline"},
			Payload: Payload(SetTextPayload{BlockID: "hero", Field: "headline", Text: "Human edit"}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	value := result.Document.Blocks[0].Values["headline"]
	if value.String != "Human edit" {
		t.Fatalf("headline = %#v, want human edit", value)
	}
}

func TestDeleteTombstoneIgnoresLateEdit(t *testing.T) {
	doc := testDocument()
	result, err := Apply(doc, []Operation{
		{
			ID:      "delete-contact",
			ActorID: "human",
			Clock:   "01",
			Kind:    OpDeleteBlock,
			Target:  Target{BlockID: "contact"},
			Payload: Payload(DeleteBlockPayload{BlockID: "contact"}),
		},
		{
			ID:      "late-agent-edit",
			ActorID: "agent",
			Clock:   "02",
			Kind:    OpSetText,
			Target:  Target{BlockID: "contact", Field: "body"},
			Payload: Payload(SetTextPayload{BlockID: "contact", Field: "body", Text: "late"}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(result.Document); got != "hero,products" {
		t.Fatalf("unexpected document order %s", got)
	}
	if len(result.Tombstones) != 1 || result.Tombstones[0].BlockID != "contact" {
		t.Fatalf("unexpected tombstones: %#v", result.Tombstones)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("expected only delete to apply, got %#v", result.Applied)
	}
}

func TestSetStyleSuggestionAndComment(t *testing.T) {
	doc := testDocument()
	suggested := Operation{
		ID:      "suggest-airy",
		ActorID: "agent",
		Clock:   "02.1",
		Kind:    OpSetStyle,
		Target:  Target{BlockID: "hero", Field: "spacing"},
		Payload: Payload(SetStylePayload{BlockID: "hero", Key: "spacing", Value: blockstudio.Value{Kind: blockstudio.FieldSelect, String: "airy"}}),
	}
	result, err := Apply(doc, []Operation{
		suggested,
		{
			ID:        "proposal",
			ActorID:   "agent",
			ActorKind: ActorAgent,
			Clock:     "01",
			Kind:      OpSuggest,
			Target:    Target{BlockID: "hero"},
			Payload:   Payload(SuggestPayload{Title: "Roomier hero", Operations: []Operation{suggested}}),
		},
		{
			ID:      "comment",
			ActorID: "human",
			Clock:   "03",
			Kind:    OpComment,
			Target:  Target{BlockID: "hero"},
			Payload: Payload(CommentPayload{Body: "Looks better."}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	value := result.Document.Blocks[0].Values["style.spacing"]
	if value.String != "airy" {
		t.Fatalf("style.spacing = %#v, want airy", value)
	}
	if len(result.Suggestions) != 1 || result.Suggestions[0].ActorKind != ActorAgent || len(result.Suggestions[0].Operations) != 1 {
		t.Fatalf("unexpected suggestions: %#v", result.Suggestions)
	}
	if len(result.Comments) != 1 || result.Comments[0].Body != "Looks better." {
		t.Fatalf("unexpected comments: %#v", result.Comments)
	}
}

func TestInvertTransactionRestoresLocalEdit(t *testing.T) {
	doc := testDocument()
	tx := Transaction{
		ID:    "tx-title",
		Actor: Actor{ID: "human", Kind: ActorHuman},
		Operations: []Operation{{
			ID:      "title",
			ActorID: "human",
			Clock:   "01",
			Kind:    OpSetText,
			Target:  Target{BlockID: "hero", Field: "headline"},
			Payload: Payload(SetTextPayload{BlockID: "hero", Field: "headline", Text: "New headline"}),
		}},
	}
	changed, err := Apply(doc, tx.Operations)
	if err != nil {
		t.Fatal(err)
	}
	inverse := InvertTransaction(doc, tx)
	restored, err := Apply(changed.Document, inverse)
	if err != nil {
		t.Fatal(err)
	}
	value := restored.Document.Blocks[0].Values["headline"]
	if value.String != "Original headline" {
		t.Fatalf("headline after undo = %#v", value)
	}
}

func testDocument() blockstudio.Document {
	return blockstudio.Document{
		Version: 1,
		Kind:    "home",
		Blocks: []blockstudio.BlockInstance{
			{ID: "hero", Key: "hero", Enabled: true, Order: 1, Values: blockstudio.Values{"headline": {Kind: blockstudio.FieldText, String: "Original headline"}}},
			{ID: "products", Key: "products", Enabled: true, Order: 2},
			{ID: "contact", Key: "contact", Enabled: true, Order: 3},
		},
	}
}

func keys(doc blockstudio.Document) string {
	out := ""
	for index, block := range doc.Blocks {
		if index > 0 {
			out += ","
		}
		out += block.Key
	}
	return out
}
