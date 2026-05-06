package blockstudio

import (
	"bytes"
	"embed"
	"net/http"
	"time"
)

const RuntimePath = "/block-studio.js"

//go:embed block-studio.js
var runtimeFS embed.FS

func RuntimeScript() []byte {
	data, err := runtimeFS.ReadFile("block-studio.js")
	if err != nil {
		return nil
	}
	return data
}

func RuntimeHandler() http.Handler {
	data := RuntimeScript()
	modtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, "block-studio.js", modtime, bytes.NewReader(data))
	})
}
