package collab

import (
	"fmt"

	"m31labs.dev/gosx-admin/blockstudio"
)

func cloneDocument(doc blockstudio.Document) blockstudio.Document {
	out := blockstudio.Document{Version: doc.Version, Kind: doc.Kind, Blocks: make([]blockstudio.BlockInstance, 0, len(doc.Blocks))}
	for _, block := range doc.Blocks {
		out.Blocks = append(out.Blocks, cloneBlock(block))
	}
	return out
}

func cloneBlock(block blockstudio.BlockInstance) blockstudio.BlockInstance {
	block.Values = cloneValues(block.Values)
	return block
}

func cloneValues(values blockstudio.Values) blockstudio.Values {
	if len(values) == 0 {
		return nil
	}
	out := blockstudio.Values{}
	for key, value := range values {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value blockstudio.Value) blockstudio.Value {
	if len(value.List) > 0 {
		value.List = cloneValueList(value.List)
	}
	if len(value.Object) > 0 {
		next := map[string]blockstudio.Value{}
		for key, nested := range value.Object {
			next[key] = cloneValue(nested)
		}
		value.Object = next
	}
	if value.Media != nil {
		media := *value.Media
		value.Media = &media
	}
	if value.Relation != nil {
		relation := *value.Relation
		value.Relation = &relation
	}
	return value
}

func cloneValueList(values []blockstudio.Value) []blockstudio.Value {
	out := make([]blockstudio.Value, len(values))
	for index, value := range values {
		out[index] = cloneValue(value)
	}
	return out
}

func generatedBlockID(key string, count int) string {
	if count <= 1 {
		return key
	}
	return fmt.Sprintf("%s-%d", key, count)
}
