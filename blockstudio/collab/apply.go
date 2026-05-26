package collab

import (
	"sort"
	"strings"

	"m31labs.dev/gosx-admin/blockstudio"
)

func Apply(doc blockstudio.Document, operations []Operation) (Result, error) {
	state := newState(doc)
	ops := NormalizeOperations(operations)
	result := Result{Applied: make([]Operation, 0, len(ops))}
	for _, op := range ops {
		op = normalizeOperation(op)
		switch op.Kind {
		case OpInsertBlock:
			if err := applyInsert(state, op); err != nil {
				return Result{}, err
			}
			result.Applied = append(result.Applied, op)
		case OpMoveBlock:
			if err := applyMove(state, op); err != nil {
				return Result{}, err
			}
			result.Applied = append(result.Applied, op)
		case OpDeleteBlock:
			tombstone, err := applyDelete(state, op)
			if err != nil {
				return Result{}, err
			}
			if tombstone.BlockID != "" {
				result.Tombstones = append(result.Tombstones, tombstone)
			}
			result.Applied = append(result.Applied, op)
		case OpSetEnabled:
			if applySetEnabled(state, op) {
				result.Applied = append(result.Applied, op)
			}
		case OpSetField:
			if applySetField(state, op) {
				result.Applied = append(result.Applied, op)
			}
		case OpSetText:
			if applySetText(state, op) {
				result.Applied = append(result.Applied, op)
			}
		case OpSetStyle:
			if applySetStyle(state, op) {
				result.Applied = append(result.Applied, op)
			}
		case OpSuggest:
			suggestion, err := suggestionFromOperation(op)
			if err != nil {
				return Result{}, err
			}
			result.Suggestions = append(result.Suggestions, suggestion)
			result.Applied = append(result.Applied, op)
		case OpComment:
			comment, err := commentFromOperation(op)
			if err != nil {
				return Result{}, err
			}
			result.Comments = append(result.Comments, comment)
			result.Applied = append(result.Applied, op)
		}
	}
	result.Document = state.document()
	return result, nil
}

func NormalizeOperations(operations []Operation) []Operation {
	out := make([]Operation, 0, len(operations))
	for _, op := range operations {
		if op.Kind == "" {
			continue
		}
		out = append(out, normalizeOperation(op))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return operationSortKey(out[i]) < operationSortKey(out[j])
	})
	return out
}

type documentState struct {
	version int
	kind    string
	blocks  []blockstudio.BlockInstance
	deleted map[string]Tombstone
}

func newState(doc blockstudio.Document) *documentState {
	blocks := make([]blockstudio.BlockInstance, 0, len(doc.Blocks))
	seen := map[string]int{}
	for index, block := range doc.Blocks {
		block = cloneBlock(block)
		if strings.TrimSpace(block.ID) == "" {
			seen[block.Key]++
			block.ID = generatedBlockID(block.Key, seen[block.Key])
		}
		if block.Order <= 0 {
			block.Order = index + 1
		}
		blocks = append(blocks, block)
	}
	return &documentState{version: doc.Version, kind: doc.Kind, blocks: blocks, deleted: map[string]Tombstone{}}
}

func (s *documentState) document() blockstudio.Document {
	doc := blockstudio.Document{Version: s.version, Kind: s.kind}
	if doc.Version <= 0 {
		doc.Version = 1
	}
	doc.Blocks = make([]blockstudio.BlockInstance, 0, len(s.blocks))
	for index, block := range s.blocks {
		block = cloneBlock(block)
		block.Order = index + 1
		doc.Blocks = append(doc.Blocks, block)
	}
	return doc
}

func applyInsert(state *documentState, op Operation) error {
	payload, err := DecodePayload[InsertBlockPayload](op)
	if err != nil {
		return err
	}
	block := cloneBlock(payload.Block)
	if strings.TrimSpace(block.ID) == "" {
		block.ID = firstNonEmpty(op.Target.BlockID, op.ID)
	}
	if block.ID == "" || block.Key == "" {
		return nil
	}
	if payload.Enabled != nil {
		block.Enabled = *payload.Enabled
	}
	state.remove(block.ID)
	delete(state.deleted, block.ID)
	state.insert(block, placement{before: payload.Before, after: payload.After, index: payload.Index})
	return nil
}

func applyMove(state *documentState, op Operation) error {
	payload, err := DecodePayload[MoveBlockPayload](op)
	if err != nil {
		return err
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	block, ok := state.remove(blockID)
	if !ok || state.isDeleted(blockID) {
		return nil
	}
	state.insert(block, placement{before: payload.Before, after: payload.After, index: payload.Index})
	return nil
}

func applyDelete(state *documentState, op Operation) (Tombstone, error) {
	payload, err := DecodePayload[DeleteBlockPayload](op)
	if err != nil {
		return Tombstone{}, err
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	if blockID == "" {
		return Tombstone{}, nil
	}
	state.remove(blockID)
	tombstone := Tombstone{BlockID: blockID, OperationID: op.ID, ActorID: op.ActorID, Created: op.Created}
	state.deleted[blockID] = tombstone
	return tombstone, nil
}

func applySetEnabled(state *documentState, op Operation) bool {
	payload, err := DecodePayload[SetEnabledPayload](op)
	if err != nil {
		return false
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	block := state.block(blockID)
	if block == nil || state.isDeleted(blockID) {
		return false
	}
	block.Enabled = payload.Enabled
	return true
}

func applySetField(state *documentState, op Operation) bool {
	payload, err := DecodePayload[SetFieldPayload](op)
	if err != nil {
		return false
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	field := firstNonEmpty(payload.Field, op.Target.Field)
	block := state.block(blockID)
	if block == nil || state.isDeleted(blockID) || field == "" {
		return false
	}
	if block.Values == nil {
		block.Values = blockstudio.Values{}
	}
	block.Values[field] = cloneValue(payload.Value)
	return true
}

func applySetText(state *documentState, op Operation) bool {
	payload, err := DecodePayload[SetTextPayload](op)
	if err != nil {
		return false
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	field := firstNonEmpty(payload.Field, op.Target.Field)
	block := state.block(blockID)
	if block == nil || state.isDeleted(blockID) || field == "" {
		return false
	}
	if block.Values == nil {
		block.Values = blockstudio.Values{}
	}
	block.Values[field] = blockstudio.Value{Kind: blockstudio.FieldTextarea, String: strings.TrimSpace(payload.Text)}
	return true
}

func applySetStyle(state *documentState, op Operation) bool {
	payload, err := DecodePayload[SetStylePayload](op)
	if err != nil {
		return false
	}
	blockID := firstNonEmpty(payload.BlockID, op.Target.BlockID)
	key := firstNonEmpty(payload.Key, op.Target.Field)
	block := state.block(blockID)
	if block == nil || state.isDeleted(blockID) || key == "" {
		return false
	}
	if block.Values == nil {
		block.Values = blockstudio.Values{}
	}
	block.Values["style."+key] = cloneValue(payload.Value)
	return true
}

func suggestionFromOperation(op Operation) (Suggestion, error) {
	payload, err := DecodePayload[SuggestPayload](op)
	if err != nil {
		return Suggestion{}, err
	}
	return Suggestion{
		ID:         op.ID,
		ActorID:    op.ActorID,
		ActorKind:  op.ActorKind,
		Target:     op.Target,
		Title:      strings.TrimSpace(payload.Title),
		Summary:    strings.TrimSpace(payload.Summary),
		Operations: NormalizeOperations(payload.Operations),
		Created:    op.Created,
	}, nil
}

func commentFromOperation(op Operation) (Comment, error) {
	payload, err := DecodePayload[CommentPayload](op)
	if err != nil {
		return Comment{}, err
	}
	return Comment{
		ID:        op.ID,
		ActorID:   op.ActorID,
		ActorKind: op.ActorKind,
		Target:    op.Target,
		Body:      strings.TrimSpace(payload.Body),
		Created:   op.Created,
	}, nil
}

type placement struct {
	before string
	after  string
	index  int
}

func (s *documentState) block(id string) *blockstudio.BlockInstance {
	id = strings.TrimSpace(id)
	for index := range s.blocks {
		if s.blocks[index].ID == id {
			return &s.blocks[index]
		}
	}
	return nil
}

func (s *documentState) remove(id string) (blockstudio.BlockInstance, bool) {
	id = strings.TrimSpace(id)
	for index, block := range s.blocks {
		if block.ID != id {
			continue
		}
		s.blocks = append(s.blocks[:index], s.blocks[index+1:]...)
		return block, true
	}
	return blockstudio.BlockInstance{}, false
}

func (s *documentState) insert(block blockstudio.BlockInstance, place placement) {
	index := len(s.blocks)
	if place.before != "" {
		if found := s.indexOf(place.before); found >= 0 {
			index = found
		}
	} else if place.after != "" {
		if found := s.indexOf(place.after); found >= 0 {
			index = found + 1
		}
	} else if place.index > 0 {
		index = place.index - 1
	}
	if index < 0 {
		index = 0
	}
	if index > len(s.blocks) {
		index = len(s.blocks)
	}
	s.blocks = append(s.blocks, blockstudio.BlockInstance{})
	copy(s.blocks[index+1:], s.blocks[index:])
	s.blocks[index] = block
}

func (s *documentState) indexOf(id string) int {
	id = strings.TrimSpace(id)
	for index, block := range s.blocks {
		if block.ID == id {
			return index
		}
	}
	return -1
}

func (s *documentState) isDeleted(id string) bool {
	_, ok := s.deleted[strings.TrimSpace(id)]
	return ok
}

func normalizeOperation(op Operation) Operation {
	op.ID = firstNonEmpty(op.ID, op.Clock, string(op.Kind))
	op.ActorID = cleanID(op.ActorID)
	op.Target.BlockID = strings.TrimSpace(op.Target.BlockID)
	op.Target.Field = strings.TrimSpace(op.Target.Field)
	op.Target.Scope = cleanID(op.Target.Scope)
	op.Target.ScopeID = cleanID(op.Target.ScopeID)
	op.Target.Selector = strings.TrimSpace(op.Target.Selector)
	if op.ActorKind == "" {
		op.ActorKind = ActorHuman
	}
	return op
}

func operationSortKey(op Operation) string {
	return firstNonEmpty(op.Clock, op.Created.UTC().Format("20060102T150405.000000000Z")) + "\x00" + op.ActorID + "\x00" + op.ID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
