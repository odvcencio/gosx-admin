package blockstudio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type FieldKind string

const (
	FieldText     FieldKind = "text"
	FieldTextarea FieldKind = "textarea"
	FieldURL      FieldKind = "url"
	FieldImage    FieldKind = "image"
	FieldBoolean  FieldKind = "boolean"
	FieldBool     FieldKind = FieldBoolean
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
	Required    bool
	Default     Value
	Options     []FieldOption
	UI          FieldUI
}

type FieldUI struct {
	Width       string
	Placeholder string
	Picker      string
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

type Document struct {
	Version int             `json:"version"`
	Kind    string          `json:"kind,omitempty"`
	Blocks  []BlockInstance `json:"blocks"`
}

type BlockInstance struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Order   int    `json:"order"`
	Values  Values `json:"values,omitempty"`
}

type Values map[string]Value

type Value struct {
	Kind     FieldKind        `json:"kind,omitempty"`
	String   string           `json:"string,omitempty"`
	Bool     bool             `json:"bool,omitempty"`
	Number   float64          `json:"number,omitempty"`
	List     []Value          `json:"list,omitempty"`
	Object   map[string]Value `json:"object,omitempty"`
	Media    *MediaValue      `json:"media,omitempty"`
	Relation *RelationValue   `json:"relation,omitempty"`
}

type MediaValue struct {
	URL     string  `json:"url"`
	Alt     string  `json:"alt,omitempty"`
	AssetID string  `json:"assetId,omitempty"`
	FocalX  float64 `json:"focalX,omitempty"`
	FocalY  float64 `json:"focalY,omitempty"`
}

type RelationValue struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type BlockInput struct {
	Key     string
	Enabled bool
	Order   int
}

type FormOptions struct {
	Prefix          string
	LegacyPrefix    string
	PreserveUnknown bool
	AllowAdd        bool
	AllowRemove     bool
}

type DocumentOptions struct {
	Version int
	Kind    string
}

type NormalizeOption func(*normalizeDocumentOptions)

type normalizeDocumentOptions struct {
	PreserveUnknown bool
}

func WithUnknownBlocks() NormalizeOption {
	return func(options *normalizeDocumentOptions) {
		options.PreserveUnknown = true
	}
}

type ValidationErrors map[string]string

type FormDocumentView struct {
	Version   int
	Kind      string
	Prefix    string
	Count     int
	CountName string
	Blocks    []FormInstance
}

type FormInstance struct {
	ID             string
	Key            string
	Label          string
	Summary        string
	Kind           string
	Preview        string
	Enabled        bool
	EnabledChecked string
	Order          int
	MaxOrder       int
	IDName         string
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
	Fields         []FormField
}

type FormField struct {
	Name        string
	Label       string
	Kind        FieldKind
	Help        string
	Placeholder string
	Required    bool
	Value       string
	Checked     string
	InputName   string
	AltName     string
	Options     []FormFieldOption
}

type FormFieldOption struct {
	Value    string
	Label    string
	Selected string
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

func NormalizeDocument(doc Document, catalog []Definition, opts ...NormalizeOption) Document {
	normalizeOptions := normalizeDocumentOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&normalizeOptions)
		}
	}
	definitions := normalizedDefinitions(catalog)
	if len(definitions) == 0 {
		if doc.Version <= 0 {
			doc.Version = 1
		}
		doc.Blocks = nil
		return doc
	}
	byKey := definitionMap(definitions)
	type sortableBlock struct {
		block BlockInstance
		order int
		index int
	}
	seen := map[string]int{}
	blocks := make([]sortableBlock, 0, len(definitions)+len(doc.Blocks))
	for index, block := range doc.Blocks {
		key := normalizeKey(block.Key)
		if key == "" {
			continue
		}
		definition, ok := byKey[key]
		if !ok {
			if normalizeOptions.PreserveUnknown {
				if strings.TrimSpace(block.ID) == "" {
					block.ID = instanceID(key, seen[key]+1)
				}
				block.Key = key
				if block.Order <= 0 {
					block.Order = index + 1
				}
				blocks = append(blocks, sortableBlock{block: block, order: block.Order, index: index})
			}
			continue
		}
		seen[key]++
		if seen[key] > 1 && !definition.Repeatable {
			continue
		}
		block.Key = key
		if strings.TrimSpace(block.ID) == "" {
			block.ID = instanceID(key, seen[key])
		}
		if block.Order <= 0 {
			block.Order = index + 1
		}
		block.Values = normalizeValues(block.Values, definition)
		blocks = append(blocks, sortableBlock{block: block, order: block.Order, index: index})
	}
	for index, definition := range definitions {
		if seen[definition.Key] > 0 {
			continue
		}
		blocks = append(blocks, sortableBlock{
			block: BlockInstance{
				ID:      instanceID(definition.Key, 1),
				Key:     definition.Key,
				Enabled: definition.DefaultOn,
				Order:   index + 1,
				Values:  normalizeValues(nil, definition),
			},
			order: index + 1,
			index: len(doc.Blocks) + index,
		})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].order == blocks[j].order {
			return blocks[i].index < blocks[j].index
		}
		return blocks[i].order < blocks[j].order
	})
	out := Document{Version: doc.Version, Kind: strings.TrimSpace(doc.Kind)}
	if out.Version <= 0 {
		out.Version = 1
	}
	out.Blocks = make([]BlockInstance, 0, len(blocks))
	for index, item := range blocks {
		item.block.Order = index + 1
		out.Blocks = append(out.Blocks, item.block)
	}
	return out
}

func FormDocument(doc Document, catalog []Definition, options FormOptions) FormDocumentView {
	doc = NormalizeDocument(doc, catalog)
	return FormDocumentView{
		Version:   doc.Version,
		Kind:      doc.Kind,
		Prefix:    strings.TrimSpace(options.Prefix),
		Count:     len(doc.Blocks),
		CountName: blockCountName(options),
		Blocks:    FormInstances(doc, catalog, options),
	}
}

func FormInstances(doc Document, catalog []Definition, options FormOptions) []FormInstance {
	doc = NormalizeDocument(doc, catalog)
	definitions := normalizedDefinitions(catalog)
	byKey := definitionMap(definitions)
	out := make([]FormInstance, 0, len(doc.Blocks))
	for index, block := range doc.Blocks {
		definition, ok := byKey[normalizeKey(block.Key)]
		if !ok {
			continue
		}
		out = append(out, formInstance(block, definition, index, len(doc.Blocks), options))
	}
	return out
}

func ParseDocumentForm(form map[string]string, catalog []Definition, options FormOptions) (Document, ValidationErrors) {
	errs := ValidationErrors{}
	definitions := normalizedDefinitions(catalog)
	byKey := definitionMap(definitions)
	count := documentFormCount(form, options)
	blocks := make([]BlockInstance, 0, count)
	for i := 0; i < count; i++ {
		key := firstFormValue(form, blockKeyName(options, i), legacyKeyName(options, i))
		key = normalizeKey(key)
		if key == "" {
			continue
		}
		definition, ok := byKey[key]
		if !ok {
			if options.PreserveUnknown {
				blocks = append(blocks, BlockInstance{ID: firstFormValue(form, blockIDName(options, i)), Key: key, Enabled: boolFormValue(firstFormValue(form, blockEnabledName(options, i), legacyEnabledName(options, i))), Order: i + 1})
			}
			continue
		}
		order := i + 1
		orderField := firstPresentName(form, blockOrderName(options, i), legacyOrderName(options, i))
		if orderField != "" && strings.TrimSpace(form[orderField]) != "" {
			parsed, err := parsePositiveInt(form[orderField])
			if err != nil {
				errs[orderField] = "Enter a positive number."
			} else {
				order = parsed
			}
		}
		values := parseFieldValues(form, definition, i, options, errs)
		blocks = append(blocks, BlockInstance{
			ID:      strings.TrimSpace(firstFormValue(form, blockIDName(options, i))),
			Key:     key,
			Enabled: boolFormValue(firstFormValue(form, blockEnabledName(options, i), legacyEnabledName(options, i))),
			Order:   order,
			Values:  values,
		})
	}
	opts := []NormalizeOption{}
	if options.PreserveUnknown {
		opts = append(opts, WithUnknownBlocks())
	}
	return NormalizeDocument(Document{Version: 1, Blocks: blocks}, definitions, opts...), errs
}

func ValidateDocument(doc Document, catalog []Definition) ValidationErrors {
	errs := ValidationErrors{}
	definitions := definitionMap(normalizedDefinitions(catalog))
	for index, block := range doc.Blocks {
		definition, ok := definitions[normalizeKey(block.Key)]
		if !ok {
			continue
		}
		validateValues(fmt.Sprintf("blocks[%d].values", index), block.Values, definition, errs)
	}
	return errs
}

func InputsFromDocument(doc Document, options FormOptions) []BlockInput {
	out := make([]BlockInput, 0, len(doc.Blocks))
	for index, block := range doc.Blocks {
		order := block.Order
		if order <= 0 {
			order = index + 1
		}
		out = append(out, BlockInput{Key: block.Key, Enabled: block.Enabled, Order: order})
	}
	return out
}

func DocumentFromBlocks(blocks []Block, options DocumentOptions) Document {
	version := options.Version
	if version <= 0 {
		version = 1
	}
	doc := Document{Version: version, Kind: strings.TrimSpace(options.Kind), Blocks: make([]BlockInstance, 0, len(blocks))}
	seen := map[string]int{}
	for index, block := range blocks {
		key := normalizeKey(block.Key)
		if key == "" {
			continue
		}
		seen[key]++
		doc.Blocks = append(doc.Blocks, BlockInstance{
			ID:      instanceID(key, seen[key]),
			Key:     key,
			Enabled: block.Enabled,
			Order:   index + 1,
		})
	}
	return doc
}

func BlocksFromDocument(doc Document) []Block {
	out := make([]Block, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		out = append(out, Block{Key: block.Key, Enabled: block.Enabled})
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

func definitionMap(definitions []Definition) map[string]Definition {
	out := map[string]Definition{}
	for _, definition := range definitions {
		out[definition.Key] = definition
	}
	return out
}

func normalizeKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func normalizeValues(input Values, definition Definition) Values {
	if len(definition.Fields) == 0 && len(input) == 0 {
		return nil
	}
	out := Values{}
	for key, value := range input {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		out[name] = value
	}
	for _, field := range definition.Fields {
		if strings.TrimSpace(field.Name) == "" {
			continue
		}
		if _, ok := out[field.Name]; ok {
			continue
		}
		if !valueEmpty(field.Default) {
			out[field.Name] = field.Default
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func valueEmpty(value Value) bool {
	return value.Kind == "" && value.String == "" && !value.Bool && value.Number == 0 && len(value.List) == 0 && len(value.Object) == 0 && value.Media == nil && value.Relation == nil
}

func instanceID(key string, count int) string {
	if count <= 1 {
		return key
	}
	return fmt.Sprintf("%s-%d", key, count)
}

func formInstance(block BlockInstance, definition Definition, index, count int, options FormOptions) FormInstance {
	fields := make([]FormField, 0, len(definition.Fields))
	for _, field := range definition.Fields {
		fields = append(fields, formField(block, field, index, options))
	}
	return FormInstance{
		ID:             block.ID,
		Key:            definition.Key,
		Label:          definition.Label,
		Summary:        definition.Summary,
		Kind:           definition.Kind,
		Preview:        definition.Preview,
		Enabled:        block.Enabled,
		EnabledChecked: checked(block.Enabled),
		Order:          index + 1,
		MaxOrder:       count,
		IDName:         blockIDName(options, index),
		KeyName:        blockKeyName(options, index),
		EnabledName:    blockEnabledName(options, index),
		OrderName:      blockOrderName(options, index),
		OrderLabel:     "Block order for " + definition.Label,
		DragLabel:      "Reorder " + definition.Label,
		MoveUpLabel:    "Move " + definition.Label + " up",
		MoveDownLabel:  "Move " + definition.Label + " down",
		Locked:         definition.Locked,
		Repeatable:     definition.Repeatable,
		Icon:           definition.Icon,
		Fields:         fields,
	}
}

func formField(block BlockInstance, field FieldDefinition, index int, options FormOptions) FormField {
	value := block.Values[field.Name]
	out := FormField{
		Name:        field.Name,
		Label:       field.Label,
		Kind:        field.Kind,
		Help:        field.Help,
		Placeholder: firstNonEmpty(field.UI.Placeholder, field.Placeholder),
		Required:    field.Required,
		InputName:   blockFieldName(options, index, field.Name),
		Options:     make([]FormFieldOption, 0, len(field.Options)),
	}
	switch field.Kind {
	case FieldBoolean:
		out.Checked = checked(value.Bool)
	case FieldImage:
		if value.Media != nil {
			out.Value = value.Media.URL
		} else {
			out.Value = value.String
		}
		out.AltName = blockFieldName(options, index, field.Name+".alt")
	default:
		out.Value = value.String
	}
	for _, option := range field.Options {
		out.Options = append(out.Options, FormFieldOption{
			Value:    option.Value,
			Label:    option.Label,
			Selected: selected(option.Value == out.Value),
		})
	}
	return out
}

func parseFieldValues(form map[string]string, definition Definition, index int, options FormOptions, errs ValidationErrors) Values {
	if len(definition.Fields) == 0 {
		return nil
	}
	values := Values{}
	for _, field := range definition.Fields {
		name := blockFieldName(options, index, field.Name)
		switch field.Kind {
		case FieldBoolean:
			values[field.Name] = Value{Kind: field.Kind, Bool: boolFormValue(form[name])}
		case FieldImage:
			url := strings.TrimSpace(form[name])
			alt := strings.TrimSpace(form[blockFieldName(options, index, field.Name+".alt")])
			values[field.Name] = Value{Kind: field.Kind, String: url, Media: &MediaValue{URL: url, Alt: alt}}
		default:
			values[field.Name] = Value{Kind: field.Kind, String: strings.TrimSpace(form[name])}
		}
	}
	validateValues(fmt.Sprintf("blocks[%d].values", index), values, definition, errs)
	if len(values) == 0 {
		return nil
	}
	return values
}

func validateValues(prefix string, values Values, definition Definition, errs ValidationErrors) {
	for _, field := range definition.Fields {
		value := values[field.Name]
		if field.Required && fieldValueString(value) == "" && !value.Bool {
			errs[prefix+"."+field.Name] = "This field is required."
			continue
		}
		if field.Kind == FieldSelect && len(field.Options) > 0 && fieldValueString(value) != "" && !optionAllowed(fieldValueString(value), field.Options) {
			errs[prefix+"."+field.Name] = "Choose one of the available options."
		}
	}
}

func fieldValueString(value Value) string {
	if value.Media != nil {
		return strings.TrimSpace(value.Media.URL)
	}
	return strings.TrimSpace(value.String)
}

func optionAllowed(value string, options []FieldOption) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func documentFormCount(form map[string]string, options FormOptions) int {
	if value := strings.TrimSpace(form[blockCountName(options)]); value != "" {
		if count, err := parsePositiveInt(value); err == nil {
			return count
		}
	}
	count := 0
	prefixes := []string{blockKeyPrefix(options), blockOrderPrefix(options), blockEnabledPrefix(options)}
	if legacyPrefix(options) != "" {
		prefixes = append(prefixes, legacyKeyPrefix(options), legacyOrderPrefix(options), legacyEnabledPrefix(options))
	}
	for key := range form {
		for _, prefix := range prefixes {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			suffix := strings.TrimPrefix(key, prefix)
			index, err := parsePositiveInt(suffix)
			if err == nil && index+1 > count {
				count = index + 1
			}
		}
	}
	return count
}

func blockCountName(options FormOptions) string {
	return strings.TrimSpace(options.Prefix) + "BlockCount"
}

func blockIDName(options FormOptions, index int) string {
	return fmt.Sprintf("%sBlockID%d", strings.TrimSpace(options.Prefix), index)
}

func blockKeyName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", blockKeyPrefix(options), index)
}

func blockEnabledName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", blockEnabledPrefix(options), index)
}

func blockOrderName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", blockOrderPrefix(options), index)
}

func blockFieldName(options FormOptions, index int, field string) string {
	return fmt.Sprintf("%sBlockField%d_%s", strings.TrimSpace(options.Prefix), index, field)
}

func blockKeyPrefix(options FormOptions) string {
	return strings.TrimSpace(options.Prefix) + "BlockKey"
}

func blockEnabledPrefix(options FormOptions) string {
	return strings.TrimSpace(options.Prefix) + "BlockEnabled"
}

func blockOrderPrefix(options FormOptions) string {
	return strings.TrimSpace(options.Prefix) + "BlockOrder"
}

func legacyPrefix(options FormOptions) string {
	if strings.TrimSpace(options.LegacyPrefix) != "" {
		return strings.TrimSpace(options.LegacyPrefix)
	}
	return strings.TrimSpace(options.Prefix)
}

func legacyKeyName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", legacyKeyPrefix(options), index)
}

func legacyEnabledName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", legacyEnabledPrefix(options), index)
}

func legacyOrderName(options FormOptions, index int) string {
	return fmt.Sprintf("%s%d", legacyOrderPrefix(options), index)
}

func legacyKeyPrefix(options FormOptions) string {
	return legacyPrefix(options) + "Key"
}

func legacyEnabledPrefix(options FormOptions) string {
	return legacyPrefix(options) + "Enabled"
}

func legacyOrderPrefix(options FormOptions) string {
	return legacyPrefix(options) + "Order"
}

func firstFormValue(form map[string]string, keys ...string) string {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value, ok := form[key]; ok {
			return value
		}
	}
	return ""
}

func firstPresentName(form map[string]string, keys ...string) string {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := form[key]; ok {
			return key
		}
	}
	return ""
}

func boolFormValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("negative value")
	}
	return parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func selected(value bool) string {
	if value {
		return "selected"
	}
	return ""
}

func checked(value bool) string {
	if value {
		return "checked"
	}
	return ""
}
