package collab

import (
	"fmt"
	"strings"

	"m31labs.dev/gosx-admin/blockstudio"
)

type ReviewSeverity string

const (
	ReviewInfo    ReviewSeverity = "info"
	ReviewWarning ReviewSeverity = "warning"
)

type Review struct {
	TransactionID  string          `json:"transactionId,omitempty"`
	Actor          Actor           `json:"actor"`
	OperationCount int             `json:"operationCount"`
	Summary        string          `json:"summary"`
	RequiresReview bool            `json:"requiresReview"`
	Items          []ReviewItem    `json:"items,omitempty"`
	Findings       []ReviewFinding `json:"findings,omitempty"`
}

type ReviewItem struct {
	OperationID string        `json:"operationId,omitempty"`
	Kind        OperationKind `json:"kind"`
	Target      Target        `json:"target,omitempty"`
	Summary     string        `json:"summary"`
}

type ReviewFinding struct {
	OperationID string         `json:"operationId,omitempty"`
	Severity    ReviewSeverity `json:"severity"`
	Message     string         `json:"message"`
}

func ReviewTransaction(before blockstudio.Document, tx Transaction) Review {
	actor := NormalizeActor(tx.Actor)
	ops := NormalizeOperations(tx.Operations)
	review := Review{
		TransactionID:  strings.TrimSpace(tx.ID),
		Actor:          actor,
		OperationCount: len(ops),
		RequiresReview: actor.Kind == ActorAgent || actor.Kind == ActorAutomation,
		Items:          make([]ReviewItem, 0, len(ops)),
	}
	for _, op := range ops {
		if op.ActorID == "" {
			op.ActorID = actor.ID
		}
		if op.ActorKind == "" {
			op.ActorKind = actor.Kind
		}
		item, findings := reviewOperation(before, op)
		review.Items = append(review.Items, item)
		review.Findings = append(review.Findings, findings...)
		if op.Kind == OpSuggest {
			review.RequiresReview = true
		}
	}
	review.Summary = reviewSummary(review)
	return review
}

func ReviewOperations(before blockstudio.Document, actor Actor, operations []Operation) Review {
	return ReviewTransaction(before, Transaction{Actor: actor, Operations: operations})
}

func reviewOperation(before blockstudio.Document, op Operation) (ReviewItem, []ReviewFinding) {
	op = normalizeOperation(op)
	item := ReviewItem{OperationID: op.ID, Kind: op.Kind, Target: op.Target}
	var findings []ReviewFinding
	switch op.Kind {
	case OpInsertBlock:
		payload, err := DecodePayload[InsertBlockPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Add a block"
			return item, findings
		}
		block := payload.Block
		if block.ID == "" {
			block.ID = firstNonEmpty(op.Target.BlockID, op.ID)
		}
		item.Target.BlockID = block.ID
		item.Summary = "Add " + blockName(block) + placementSummary(payload.Before, payload.After, payload.Index)
	case OpMoveBlock:
		payload, err := DecodePayload[MoveBlockPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Move a block"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		item.Target.BlockID = blockID
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Move missing block " + blockID
			return item, findings
		}
		item.Summary = "Move " + blockName(block) + placementSummary(payload.Before, payload.After, payload.Index)
	case OpDeleteBlock:
		payload, err := DecodePayload[DeleteBlockPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Remove a block"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		item.Target.BlockID = blockID
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Remove missing block " + blockID
			return item, findings
		}
		item.Summary = "Remove " + blockName(block)
	case OpSetEnabled:
		payload, err := DecodePayload[SetEnabledPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Change block visibility"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		item.Target.BlockID = blockID
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Change visibility for missing block " + blockID
			return item, findings
		}
		verb := "Show "
		if !payload.Enabled {
			verb = "Hide "
		}
		item.Summary = verb + blockName(block)
	case OpSetField:
		payload, err := DecodePayload[SetFieldPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Change a field"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		field := firstNonEmpty(payload.Field, op.Target.Field)
		item.Target.BlockID = blockID
		item.Target.Field = field
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Change " + fieldLabel(field) + " on missing block " + blockID
			return item, findings
		}
		item.Summary = "Change " + fieldLabel(field) + " on " + blockName(block)
	case OpSetText:
		payload, err := DecodePayload[SetTextPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Edit text"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		field := firstNonEmpty(payload.Field, op.Target.Field)
		item.Target.BlockID = blockID
		item.Target.Field = field
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Edit " + fieldLabel(field) + " on missing block " + blockID
			return item, findings
		}
		item.Summary = "Edit " + fieldLabel(field) + " on " + blockName(block)
	case OpSetStyle:
		payload, err := DecodePayload[SetStylePayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Change style"
			return item, findings
		}
		blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
		key := firstNonEmpty(payload.Key, op.Target.Field)
		item.Target.BlockID = blockID
		item.Target.Field = key
		block, _, ok := blockAt(before, blockID)
		if !ok {
			findings = append(findings, missingBlockFinding(op, blockID))
			item.Summary = "Change " + fieldLabel(key) + " style on missing block " + blockID
			return item, findings
		}
		item.Summary = "Change " + fieldLabel(key) + " style on " + blockName(block)
	case OpSuggest:
		payload, err := DecodePayload[SuggestPayload](op)
		if err != nil {
			findings = append(findings, reviewDecodeFinding(op, err))
			item.Summary = "Review a suggestion"
			return item, findings
		}
		title := strings.TrimSpace(payload.Title)
		if title == "" {
			title = fmt.Sprintf("%d proposed operations", len(payload.Operations))
		}
		item.Summary = "Review suggestion: " + title
	case OpComment:
		item.Summary = "Add a comment"
	default:
		item.Summary = "Apply " + strings.ReplaceAll(string(op.Kind), "_", " ")
		findings = append(findings, ReviewFinding{OperationID: op.ID, Severity: ReviewInfo, Message: "This operation kind does not have a specialized review summary yet."})
	}
	return item, findings
}

func reviewSummary(review Review) string {
	actor := strings.TrimSpace(review.Actor.DisplayName)
	if actor == "" {
		actor = review.Actor.ID
	}
	if actor == "" {
		actor = "Unknown actor"
	}
	count := review.OperationCount
	noun := "operations"
	if count == 1 {
		noun = "operation"
	}
	if review.RequiresReview {
		return fmt.Sprintf("%s proposed %d %s for review.", actor, count, noun)
	}
	return fmt.Sprintf("%s prepared %d %s.", actor, count, noun)
}

func reviewDecodeFinding(op Operation, err error) ReviewFinding {
	return ReviewFinding{OperationID: op.ID, Severity: ReviewWarning, Message: err.Error()}
}

func missingBlockFinding(op Operation, blockID string) ReviewFinding {
	if strings.TrimSpace(blockID) == "" {
		return ReviewFinding{OperationID: op.ID, Severity: ReviewWarning, Message: "Target block is missing."}
	}
	return ReviewFinding{OperationID: op.ID, Severity: ReviewWarning, Message: "Target block " + blockID + " was not found."}
}

func blockName(block blockstudio.BlockInstance) string {
	label := firstNonEmpty(block.Key, block.ID, "block")
	return fieldLabel(label) + " block"
}

func fieldLabel(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, ".", " ")
	if value == "" {
		return "field"
	}
	return value
}

func placementSummary(before, after string, index int) string {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	if before != "" {
		return " before " + before
	}
	if after != "" {
		return " after " + after
	}
	if index > 0 {
		return fmt.Sprintf(" at position %d", index)
	}
	return ""
}
