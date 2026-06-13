// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"sync"
	"testing"
	"time"
)

func TestPermBridgeBeginResolve(t *testing.T) {
	b := newPermBridge()
	id, ch := b.begin()
	if id == "" {
		t.Fatal("empty id")
	}
	if b.pending() != 1 {
		t.Fatalf("pending = %d, want 1", b.pending())
	}

	// A waiter blocks until resolve delivers the decision.
	var got bool
	done := make(chan struct{})
	go func() { got = <-ch; close(done) }()

	if !b.resolve(id, true) {
		t.Fatal("resolve should succeed for a known id")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waiter never received the decision")
	}
	if !got {
		t.Fatal("decision should be approve=true")
	}
	if b.pending() != 0 {
		t.Fatalf("pending should be 0 after resolve, got %d", b.pending())
	}
}

func TestPermBridgeResolveUnknown(t *testing.T) {
	b := newPermBridge()
	if b.resolve("nope", true) {
		t.Fatal("resolving an unknown id should return false")
	}
}

func TestPermBridgeUniqueIDsConcurrent(t *testing.T) {
	b := newPermBridge()
	var wg sync.WaitGroup
	ids := make(chan string, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); id, _ := b.begin(); ids <- id }()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != 50 {
		t.Fatalf("expected 50 unique ids, got %d", len(seen))
	}
}
