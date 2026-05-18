package collab

import (
	"strings"

	"github.com/odvcencio/gosx-admin/blockstudio"
)

func InvertTransaction(before blockstudio.Document, tx Transaction) []Operation {
	before = cloneDocument(before)
	ops := NormalizeOperations(tx.Operations)
	inverse := make([]Operation, 0, len(ops))
	for i := len(ops) - 1; i >= 0; i-- {
		op := normalizeOperation(ops[i])
		switch op.Kind {
		case OpInsertBlock:
			payload, err := DecodePayload[InsertBlockPayload](op)
			if err != nil {
				continue
			}
			blockID := firstNonEmpty(payload.Block.ID, op.Target.BlockID)
			inverse = append(inverse, undoOperation(op, OpDeleteBlock, Target{BlockID: blockID}, DeleteBlockPayload{BlockID: blockID, Reason: "undo insert"}))
		case OpDeleteBlock:
			payload, err := DecodePayload[DeleteBlockPayload](op)
			if err != nil {
				continue
			}
			blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
			if block, index, ok := blockAt(before, blockID); ok {
				inverse = append(inverse, undoOperation(op, OpInsertBlock, Target{BlockID: blockID}, InsertBlockPayload{Block: block, Index: index + 1}))
			}
		case OpMoveBlock:
			payload, err := DecodePayload[MoveBlockPayload](op)
			if err != nil {
				continue
			}
			blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
			if _, index, ok := blockAt(before, blockID); ok {
				inverse = append(inverse, undoOperation(op, OpMoveBlock, Target{BlockID: blockID}, MoveBlockPayload{BlockID: blockID, Index: index + 1}))
			}
		case OpSetEnabled:
			payload, err := DecodePayload[SetEnabledPayload](op)
			if err != nil {
				continue
			}
			blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
			if block, _, ok := blockAt(before, blockID); ok {
				inverse = append(inverse, undoOperation(op, OpSetEnabled, Target{BlockID: blockID}, SetEnabledPayload{BlockID: blockID, Enabled: block.Enabled}))
			}
		case OpSetField, OpSetText:
			var blockID, field string
			if op.Kind == OpSetText {
				payload, err := DecodePayload[SetTextPayload](op)
				if err != nil {
					continue
				}
				blockID = firstNonEmpty(payload.BlockID, op.Target.BlockID)
				field = firstNonEmpty(payload.Field, op.Target.Field)
			} else {
				payload, err := DecodePayload[SetFieldPayload](op)
				if err != nil {
					continue
				}
				blockID = firstNonEmpty(payload.BlockID, op.Target.BlockID)
				field = firstNonEmpty(payload.Field, op.Target.Field)
			}
			if block, _, ok := blockAt(before, blockID); ok && field != "" {
				inverse = append(inverse, undoOperation(op, OpSetField, Target{BlockID: blockID, Field: field}, SetFieldPayload{BlockID: blockID, Field: field, Value: cloneValue(block.Values[field])}))
			}
		case OpSetStyle:
			payload, err := DecodePayload[SetStylePayload](op)
			if err != nil {
				continue
			}
			blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
			key := firstNonEmpty(payload.Key, op.Target.Field)
			if block, _, ok := blockAt(before, blockID); ok && key != "" {
				inverse = append(inverse, undoOperation(op, OpSetStyle, Target{BlockID: blockID, Field: key}, SetStylePayload{BlockID: blockID, Key: key, Value: cloneValue(block.Values["style."+key])}))
			}
		}
	}
	return inverse
}

func undoOperation(source Operation, kind OperationKind, target Target, payload any) Operation {
	return Operation{
		ID:        "undo-" + strings.TrimSpace(source.ID),
		ActorID:   source.ActorID,
		ActorKind: source.ActorKind,
		Clock:     source.Clock + "~undo",
		Kind:      kind,
		Target:    target,
		Payload:   Payload(payload),
	}
}

func blockAt(doc blockstudio.Document, blockID string) (blockstudio.BlockInstance, int, bool) {
	blockID = strings.TrimSpace(blockID)
	for index, block := range doc.Blocks {
		if block.ID == blockID {
			return cloneBlock(block), index, true
		}
	}
	return blockstudio.BlockInstance{}, -1, false
}
