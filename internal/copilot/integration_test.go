package copilot

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// sidecarPath locates sidecar/index.mjs relative to this test file.
func sidecarPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	// internal/copilot/integration_test.go -> repo root.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "sidecar", "index.mjs")
}

// TestSidecarIntegration drives the real Node sidecar's echo engine over the
// production Spawn path, exercising the full Go<->Node protocol. It is skipped
// when Node is unavailable so unit CI stays hermetic.
func TestSidecarIntegration(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping sidecar integration test")
	}
	client, err := Spawn(SpawnConfig{Command: node, Args: []string{sidecarPath(t)}})
	if err != nil {
		t.Fatalf("spawn sidecar: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	id, err := client.CreateSession(ctx, SessionSpec{Model: "gpt-5", Streaming: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if id == "" {
		t.Fatal("empty session id")
	}
	if err := client.Send(ctx, id, "ping"); err != nil {
		t.Fatalf("send: %v", err)
	}

	var text string
	var sawUsage, sawIdle bool
	deadline := time.After(10 * time.Second)
	for !sawIdle {
		select {
		case ev := <-client.Events():
			switch ev.Type {
			case EventMessageDelta:
				var d DeltaData
				_ = json.Unmarshal(ev.Data, &d)
				text += d.DeltaContent
			case EventUsage:
				var u UsageData
				_ = json.Unmarshal(ev.Data, &u)
				if u.OutputTokens <= 0 {
					t.Fatalf("expected positive output tokens, got %+v", u)
				}
				sawUsage = true
			case EventIdle:
				sawIdle = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for idle; text=%q usage=%v", text, sawUsage)
		}
	}
	if text == "" {
		t.Fatal("no streamed text received from sidecar")
	}
	if !sawUsage {
		t.Fatal("no usage event received from sidecar")
	}
}
