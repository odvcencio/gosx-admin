//go:build js && wasm

// Command mdpp-lint-worker is the WASM half of the editor's lint worker. Build
// it in the consumer's module so it uses the consumer's pinned mdpp:
//
//	GOOS=js GOARCH=wasm go build \
//	  -tags 'grammar_subset grammar_subset_markdown grammar_subset_markdown_inline' \
//	  -trimpath -ldflags='-s -w' -o mdpp-lint.wasm \
//	  m31labs.dev/gosx-admin/mdppstudio/cmd/mdpp-lint-worker
//
// It sets globalThis.mdppAnalyze(source, optionsJSON) → JSON of
// mdppstudio.Analysis, then blocks. lint-worker.js speaks the wire protocol.
package main

import (
	"encoding/json"
	"syscall/js"

	"m31labs.dev/gosx-admin/mdppstudio/mdppanalyze"
)

func main() {
	fn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return "{}"
		}
		source := args[0].String()
		var opts mdppanalyze.Options
		if len(args) > 1 && args[1].Type() == js.TypeString {
			_ = json.Unmarshal([]byte(args[1].String()), &opts)
		}
		analysis := mdppanalyze.Analyze(source, opts)
		data, err := json.Marshal(analysis)
		if err != nil {
			return "{}"
		}
		return string(data)
	})
	js.Global().Set("mdppAnalyze", fn)
	if post := js.Global().Get("postMessage"); post.Type() == js.TypeFunction {
		post.Invoke(map[string]any{"type": "ready", "protocol": 1, "engine": "0.4.8"})
	}
	select {}
}
