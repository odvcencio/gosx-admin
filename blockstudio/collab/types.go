package collab

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"m31labs.dev/gosx-admin/blockstudio"
)

type ActorKind string
type Capability string
type OperationKind string

const (
	ActorHuman      ActorKind = "human"
	ActorAgent      ActorKind = "agent"
	ActorAutomation ActorKind = "automation"
	ActorSystem     ActorKind = "system"

	CapabilityInspect Capability = "inspect"
	CapabilityEdit    Capability = "edit"
	CapabilitySuggest Capability = "suggest"
	CapabilityComment Capability = "comment"
	CapabilityPublish Capability = "publish"
	CapabilityRestore Capability = "restore"

	OpInsertBlock OperationKind = "insert_block"
	OpMoveBlock   OperationKind = "move_block"
	OpDeleteBlock OperationKind = "delete_block"
	OpSetEnabled  OperationKind = "set_enabled"
	OpSetField    OperationKind = "set_field"
	OpSetText     OperationKind = "set_text"
	OpSetStyle    OperationKind = "set_style"
	OpSuggest     OperationKind = "suggest"
	OpComment     OperationKind = "comment"
)

type Actor struct {
	ID           string            `json:"id"`
	Kind         ActorKind         `json:"kind"`
	DisplayName  string            `json:"displayName,omitempty"`
	Color        string            `json:"color,omitempty"`
	Capabilities []Capability      `json:"capabilities,omitempty"`
	Provenance   map[string]string `json:"provenance,omitempty"`
}

type Target struct {
	BlockID  string `json:"blockId,omitempty"`
	Field    string `json:"field,omitempty"`
	Scope    string `json:"scope,omitempty"`
	ScopeID  string `json:"scopeId,omitempty"`
	Selector string `json:"selector,omitempty"`
}

type Operation struct {
	ID        string          `json:"id"`
	ActorID   string          `json:"actorId"`
	ActorKind ActorKind       `json:"actorKind,omitempty"`
	Clock     string          `json:"clock,omitempty"`
	Target    Target          `json:"target,omitempty"`
	Kind      OperationKind   `json:"kind"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Created   time.Time       `json:"created,omitempty"`
}

type Transaction struct {
	ID         string      `json:"id"`
	Actor      Actor       `json:"actor"`
	Operations []Operation `json:"operations"`
	Created    time.Time   `json:"created,omitempty"`
}

type InsertBlockPayload struct {
	Block   blockstudio.BlockInstance `json:"block"`
	Before  string                    `json:"before,omitempty"`
	After   string                    `json:"after,omitempty"`
	Index   int                       `json:"index,omitempty"`
	Enabled *bool                     `json:"enabled,omitempty"`
}

type MoveBlockPayload struct {
	BlockID string `json:"blockId,omitempty"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	Index   int    `json:"index,omitempty"`
}

type DeleteBlockPayload struct {
	BlockID string `json:"blockId,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type SetEnabledPayload struct {
	BlockID string `json:"blockId,omitempty"`
	Enabled bool   `json:"enabled"`
}

type SetFieldPayload struct {
	BlockID string            `json:"blockId,omitempty"`
	Field   string            `json:"field,omitempty"`
	Value   blockstudio.Value `json:"value"`
}

type SetTextPayload struct {
	BlockID string `json:"blockId,omitempty"`
	Field   string `json:"field,omitempty"`
	Text    string `json:"text"`
}

type SetStylePayload struct {
	BlockID string            `json:"blockId,omitempty"`
	Key     string            `json:"key,omitempty"`
	Value   blockstudio.Value `json:"value"`
}

type SuggestPayload struct {
	Title      string      `json:"title,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
}

type CommentPayload struct {
	Body string `json:"body"`
}

type Tombstone struct {
	BlockID     string    `json:"blockId"`
	OperationID string    `json:"operationId"`
	ActorID     string    `json:"actorId,omitempty"`
	Created     time.Time `json:"created,omitempty"`
}

type Suggestion struct {
	ID         string      `json:"id"`
	ActorID    string      `json:"actorId,omitempty"`
	ActorKind  ActorKind   `json:"actorKind,omitempty"`
	Target     Target      `json:"target,omitempty"`
	Title      string      `json:"title,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
	Created    time.Time   `json:"created,omitempty"`
}

type Comment struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actorId,omitempty"`
	ActorKind ActorKind `json:"actorKind,omitempty"`
	Target    Target    `json:"target,omitempty"`
	Body      string    `json:"body"`
	Created   time.Time `json:"created,omitempty"`
}

type Result struct {
	Document    blockstudio.Document `json:"document"`
	Applied     []Operation          `json:"applied,omitempty"`
	Tombstones  []Tombstone          `json:"tombstones,omitempty"`
	Suggestions []Suggestion         `json:"suggestions,omitempty"`
	Comments    []Comment            `json:"comments,omitempty"`
}

func Payload(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func DecodePayload[T any](op Operation) (T, error) {
	var out T
	if len(op.Payload) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(op.Payload, &out); err != nil {
		return out, fmt.Errorf("decode %s payload: %w", op.Kind, err)
	}
	return out, nil
}

func NormalizeActor(actor Actor) Actor {
	actor.ID = cleanID(actor.ID)
	if actor.Kind == "" {
		actor.Kind = ActorHuman
	}
	actor.DisplayName = strings.TrimSpace(actor.DisplayName)
	actor.Color = strings.TrimSpace(actor.Color)
	actor.Capabilities = normalizeCapabilities(actor.Capabilities)
	if len(actor.Provenance) > 0 {
		next := map[string]string{}
		for key, value := range actor.Provenance {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key != "" && value != "" {
				next[key] = value
			}
		}
		actor.Provenance = next
	}
	return actor
}

func normalizeCapabilities(values []Capability) []Capability {
	if len(values) == 0 {
		return nil
	}
	seen := map[Capability]bool{}
	out := make([]Capability, 0, len(values))
	for _, value := range values {
		value = Capability(cleanID(string(value)))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cleanID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	return strings.Trim(value, "-")
}
