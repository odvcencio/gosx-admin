package blockstudio

import (
	"fmt"
	"sort"
	"strings"
)

type FieldKind string

const (
	FieldText     FieldKind = "text"
	FieldTextarea FieldKind = "textarea"
	FieldURL      FieldKind = "url"
	FieldImage    FieldKind = "image"
	FieldBoolean  FieldKind = "boolean"
	FieldSelect   FieldKind = "select"
)

type FieldOption struct {
	Value string
	Label string
}

type FieldDefinition struct {
	Name        string
	Label       string
	Kind        FieldKind
	Help        string
	Placeholder string
	Options     []FieldOption
}

type Definition struct {
	Key        string
	Label      string
	Summary    string
	Kind       string
	Preview    string
	DefaultOn  bool
	Fields     []FieldDefinition
	Locked     bool
	Repeatable bool
	Icon       string
}

type Block struct {
	Key     string
	Enabled bool
}

type BlockInput struct {
	Key     string
	Enabled bool
	Order   int
}

type FormOptions struct {
	Prefix string
}

type FormBlock struct {
	Key            string
	Label          string
	Summary        string
	Kind           string
	Preview        string
	Enabled        bool
	EnabledChecked string
	Order          int
	MaxOrder       int
	KeyName        string
	EnabledName    string
	OrderName      string
	OrderLabel     string
	DragLabel      string
	MoveUpLabel    string
	MoveDownLabel  string
	Locked         bool
	Repeatable     bool
	Icon           string
}

func Normalize(input []BlockInput, catalog []Definition) []Block {
	definitions := normalizedDefinitions(catalog)
	if len(definitions) == 0 {
		return nil
	}
	inputByKey := map[string]BlockInput{}
	for _, item := range input {
		key := normalizeKey(item.Key)
		if key == "" {
			continue
		}
		item.Key = key
		inputByKey[key] = item
	}
	type sortableBlock struct {
		block Block
		order int
		index int
	}
	blocks := make([]sortableBlock, 0, len(definitions))
	for index, definition := range definitions {
		order := index + 1
		enabled := definition.DefaultOn
		if item, ok := inputByKey[definition.Key]; ok {
			enabled = item.Enabled
			if item.Order > 0 {
				order = item.Order
			}
		}
		blocks = append(blocks, sortableBlock{
			block: Block{Key: definition.Key, Enabled: enabled},
			order: order,
			index: index,
		})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].order == blocks[j].order {
			return blocks[i].index < blocks[j].index
		}
		return blocks[i].order < blocks[j].order
	})
	out := make([]Block, 0, len(blocks))
	for _, item := range blocks {
		out = append(out, item.block)
	}
	return out
}

func InputsFromBlocks(blocks []Block) []BlockInput {
	out := make([]BlockInput, 0, len(blocks))
	for index, block := range blocks {
		out = append(out, BlockInput{
			Key:     block.Key,
			Enabled: block.Enabled,
			Order:   index + 1,
		})
	}
	return out
}

func FormBlocks(blocks []Block, catalog []Definition, options FormOptions) []FormBlock {
	definitions := normalizedDefinitions(catalog)
	if len(definitions) == 0 {
		return nil
	}
	if len(blocks) == 0 {
		inputs := make([]BlockInput, 0, len(definitions))
		for index, definition := range definitions {
			inputs = append(inputs, BlockInput{Key: definition.Key, Enabled: definition.DefaultOn, Order: index + 1})
		}
		blocks = Normalize(inputs, definitions)
	}
	byKey := map[string]Definition{}
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	prefix := strings.TrimSpace(options.Prefix)
	out := make([]FormBlock, 0, len(blocks))
	for index, block := range blocks {
		definition, ok := byKey[normalizeKey(block.Key)]
		if !ok {
			continue
		}
		out = append(out, FormBlock{
			Key:            definition.Key,
			Label:          definition.Label,
			Summary:        definition.Summary,
			Kind:           definition.Kind,
			Preview:        definition.Preview,
			Enabled:        block.Enabled,
			EnabledChecked: checked(block.Enabled),
			Order:          index + 1,
			MaxOrder:       len(definitions),
			KeyName:        fmt.Sprintf("%sKey%d", prefix, index),
			EnabledName:    fmt.Sprintf("%sEnabled%d", prefix, index),
			OrderName:      fmt.Sprintf("%sOrder%d", prefix, index),
			OrderLabel:     "Block order for " + definition.Label,
			DragLabel:      "Reorder " + definition.Label,
			MoveUpLabel:    "Move " + definition.Label + " up",
			MoveDownLabel:  "Move " + definition.Label + " down",
			Locked:         definition.Locked,
			Repeatable:     definition.Repeatable,
			Icon:           definition.Icon,
		})
	}
	return out
}

func FormMaps(blocks []Block, catalog []Definition, options FormOptions) []map[string]any {
	forms := FormBlocks(blocks, catalog, options)
	out := make([]map[string]any, 0, len(forms))
	for _, form := range forms {
		out = append(out, map[string]any{
			"key":            form.Key,
			"label":          form.Label,
			"summary":        form.Summary,
			"kind":           form.Kind,
			"preview":        form.Preview,
			"enabled":        form.Enabled,
			"enabledChecked": form.EnabledChecked,
			"order":          form.Order,
			"maxOrder":       form.MaxOrder,
			"keyName":        form.KeyName,
			"enabledName":    form.EnabledName,
			"orderName":      form.OrderName,
			"orderLabel":     form.OrderLabel,
			"dragLabel":      form.DragLabel,
			"moveUpLabel":    form.MoveUpLabel,
			"moveDownLabel":  form.MoveDownLabel,
			"locked":         form.Locked,
			"repeatable":     form.Repeatable,
			"icon":           form.Icon,
			"hasSummary":     strings.TrimSpace(form.Summary) != "",
			"hasPreview":     strings.TrimSpace(form.Preview) != "",
		})
	}
	return out
}

func KeyAllowed(key string, catalog []Definition) bool {
	key = normalizeKey(key)
	for _, definition := range normalizedDefinitions(catalog) {
		if definition.Key == key {
			return true
		}
	}
	return false
}

func normalizedDefinitions(catalog []Definition) []Definition {
	out := make([]Definition, 0, len(catalog))
	seen := map[string]bool{}
	for _, definition := range catalog {
		key := normalizeKey(definition.Key)
		if key == "" || seen[key] {
			continue
		}
		definition.Key = key
		if strings.TrimSpace(definition.Label) == "" {
			definition.Label = key
		}
		definition.Label = strings.TrimSpace(definition.Label)
		definition.Summary = strings.TrimSpace(definition.Summary)
		definition.Kind = strings.TrimSpace(definition.Kind)
		definition.Preview = strings.TrimSpace(definition.Preview)
		definition.Icon = strings.TrimSpace(definition.Icon)
		seen[key] = true
		out = append(out, definition)
	}
	return out
}

func normalizeKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func checked(value bool) string {
	if value {
		return "checked"
	}
	return ""
}
