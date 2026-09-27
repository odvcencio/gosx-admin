//go:build budget

package mdppstudio

import (
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLintWorkerBudget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the consumer worker build contract uses GOOS=js from a Unix shell")
	}
	out := filepath.Join(t.TempDir(), "mdpp-lint.wasm")
	cmd := exec.Command("go", "build", "-tags", "grammar_subset grammar_subset_markdown grammar_subset_markdown_inline", "-trimpath", "-ldflags=-s -w", "-o", out, "m31labs.dev/gosx-admin/mdppstudio/cmd/mdpp-lint-worker")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOOS=js", "GOARCH=wasm")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build lint worker: %v\n%s", err, output)
	}
	wasm, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var zipped bytes.Buffer
	writer, err := gzip.NewWriterLevel(&zipped, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(wasm); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if zipped.Len() > WorkerBudgetBytes {
		t.Fatalf("worker gzip size = %d bytes, budget is %d", zipped.Len(), WorkerBudgetBytes)
	}
}
