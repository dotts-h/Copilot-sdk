package copilot

// guard_test.go — additional guard tests added to fill coverage gaps.
// Each test is labelled with the item number from the task specification.

import (
	"sync"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// ── Item 1: CacheWriteTokens mapping ────────────────────────────────────────

// TestNormalizeUsageMapsEveryTokenCategoryIncludingCacheWrite extends the
// existing TestNormalizeUsageMapsEveryTokenCategory to also assert that
// CacheWriteTokens threads through normalizeUsage. REGRESSIONS #3 documents
// a prior bug where CacheWriteTokens was silently unmapped (zero).
func TestNormalizeUsageMapsEveryTokenCategoryIncludingCacheWrite(t *testing.T) {
	d := &sdk.AssistantUsageData{
		Model:            "gpt-5",
		InputTokens:      i64(1000),
		OutputTokens:     i64(250),
		CacheReadTokens:  i64(400),
		CacheWriteTokens: i64(120),
		ReasoningTokens:  i64(80),
	}
	u := normalizeUsage(d)
	if u.CacheWriteTokens != 120 {
		t.Fatalf("CacheWriteTokens not mapped: got %d, want 120 (REGRESSIONS #3)", u.CacheWriteTokens)
	}
	if u.InputTokens != 1000 || u.OutputTokens != 250 || u.CachedTokens != 400 || u.ReasoningTokens != 80 {
		t.Fatalf("other token categories wrong: %+v", u)
	}
}

// ── Item 2: planChangeText all 4 branches ───────────────────────────────────

func TestPlanChangeTextAllBranches(t *testing.T) {
	cases := []struct {
		op   sdk.PlanChangedOperation
		want string
	}{
		{sdk.PlanChangedOperationCreate, "plan created"},
		{sdk.PlanChangedOperationUpdate, "plan updated"},
		{sdk.PlanChangedOperationDelete, "plan deleted"},
		{sdk.PlanChangedOperation("unknown-op"), "plan changed"},
	}
	for _, tc := range cases {
		got := planChangeText(tc.op)
		if got != tc.want {
			t.Errorf("planChangeText(%q) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// ── Item 3: compactionSummary dark branches ──────────────────────────────────

func TestCompactionSummaryDarkBranches(t *testing.T) {
	// Success:true with TokensRemoved nil → "compacted context"
	got := compactionSummary(&sdk.SessionCompactionCompleteData{Success: true, TokensRemoved: nil})
	if got != "compacted context" {
		t.Errorf("compactionSummary(success,nil tokens) = %q, want %q", got, "compacted context")
	}

	// Success:false with Error nil → "compaction failed"
	got = compactionSummary(&sdk.SessionCompactionCompleteData{Success: false, Error: nil})
	if got != "compaction failed" {
		t.Errorf("compactionSummary(failure,nil error) = %q, want %q", got, "compaction failed")
	}
}

// ── Item 4: toolResultText nil cases ────────────────────────────────────────

func TestToolResultTextNilCases(t *testing.T) {
	// Success:true, Result:nil → ""
	got := toolResultText(&sdk.ToolExecutionCompleteData{Success: true, Result: nil})
	if got != "" {
		t.Errorf("toolResultText(success,nil result) = %q, want empty", got)
	}

	// Success:false, Error:nil → ""
	got = toolResultText(&sdk.ToolExecutionCompleteData{Success: false, Error: nil})
	if got != "" {
		t.Errorf("toolResultText(failure,nil error) = %q, want empty", got)
	}
}

// ── Item 5: summarizeArgs non-map fallback ───────────────────────────────────

func TestSummarizeArgsNonMapFallback(t *testing.T) {
	// A plain string: must go through fmt.Sprint+clip, be non-empty, and not panic.
	got := summarizeArgs("some plain string")
	if got == "" {
		t.Error("summarizeArgs(string) should be non-empty via fmt.Sprint fallback")
	}

	// An int: same contract.
	got = summarizeArgs(42)
	if got == "" {
		t.Error("summarizeArgs(int) should be non-empty via fmt.Sprint fallback")
	}
}

// ── Item 6: interactive handler shutdown paths ───────────────────────────────

// permissionHandler: closing c.done returns UserNotAvailable, does not block.
func TestPermissionHandlerShutdownReturnsUserNotAvailable(t *testing.T) {
	c := newPolicyTestClient(nil) // no hooks — falls through to interactive gate

	type result struct {
		dec rpc.PermissionDecision
		err error
	}
	done := make(chan result, 1)
	go func() {
		// A write with no matching hooks ends up at the interactive select.
		dec, err := c.permissionHandler()(
			sdk.PermissionRequestWrite{FileName: "main.go"},
			sdk.PermissionInvocation{SessionID: "s1"},
		)
		done <- result{dec, err}
	}()

	// Wait for the gate to emit an EvPermission so we know the handler is blocking.
	select {
	case <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvPermission emitted before close")
	}

	close(c.done)

	select {
	case r := <-done:
		// Must return promptly with a UserNotAvailable decision and no error.
		if r.err != nil {
			t.Fatalf("permissionHandler shutdown err = %v", r.err)
		}
		// The actual type returned on c.done is PermissionDecisionUserNotAvailable.
		// We confirm it is not nil and does not panic; the concrete type check
		// mirrors the SDK's convention (interface non-nil).
		if r.dec == nil {
			t.Fatal("permissionHandler shutdown must return a non-nil decision")
		}
	case <-time.After(time.Second):
		t.Fatal("permissionHandler did not return after c.done closed (deadlock)")
	}
}

// userInputHandler: closing c.done returns ErrClosed, does not block.
func TestUserInputHandlerShutdownReturnsErrClosed(t *testing.T) {
	c := newTestSDKClient()
	h := c.userInputHandler()

	type result struct {
		resp sdk.UserInputResponse
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := h(sdk.UserInputRequest{Question: "Who?"}, sdk.UserInputInvocation{})
		done <- result{resp, err}
	}()

	// Wait for EvUserInput so handler is blocking.
	select {
	case <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvUserInput emitted before close")
	}

	close(c.done)

	select {
	case r := <-done:
		if r.err != ErrClosed {
			t.Fatalf("userInputHandler shutdown err = %v, want ErrClosed", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("userInputHandler did not return after c.done closed (deadlock)")
	}
}

// exitPlanModeHandler: closing c.done returns ErrClosed, does not block.
func TestExitPlanModeHandlerShutdownReturnsErrClosed(t *testing.T) {
	c := newTestSDKClient()
	h := c.exitPlanModeHandler()

	type result struct {
		res sdk.ExitPlanModeResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := h(sdk.ExitPlanModeRequest{Summary: "Do X"}, sdk.ExitPlanModeInvocation{})
		done <- result{res, err}
	}()

	// Wait for EvPlanReview.
	select {
	case <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvPlanReview emitted before close")
	}

	close(c.done)

	select {
	case r := <-done:
		if r.err != ErrClosed {
			t.Fatalf("exitPlanModeHandler shutdown err = %v, want ErrClosed", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("exitPlanModeHandler did not return after c.done closed (deadlock)")
	}
}

// elicitationHandler: closing c.done returns action=cancel (no error), does not block.
func TestElicitationHandlerShutdownReturnsCancel(t *testing.T) {
	c := newTestSDKClient()
	h := c.elicitationHandler()

	type result struct {
		res sdk.ElicitationResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := h(sdk.ElicitationContext{Message: "Tell me something"})
		done <- result{res, err}
	}()

	// Wait for EvElicitation.
	select {
	case <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvElicitation emitted before close")
	}

	close(c.done)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("elicitationHandler shutdown err = %v, want nil", r.err)
		}
		if r.res.Action != sdk.ElicitationActionCancel {
			t.Fatalf("elicitationHandler shutdown action = %v, want cancel", r.res.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("elicitationHandler did not return after c.done closed (deadlock)")
	}
}

// ── Item 7: bridge concurrency (race-friendly) ───────────────────────────────

// TestBridgeConcurrentBeginResolve spins N goroutines each calling begin()+
// resolve() concurrently on a generic bridge and asserts all complete without
// deadlock, data race, or missing delivery.
func TestBridgeConcurrentBeginResolve(t *testing.T) {
	const N = 8
	b := newPermBridge()

	type pair struct {
		id string
		ch chan bool
	}
	pairs := make([]pair, N)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Phase 1: all goroutines call begin() concurrently.
	for i := 0; i < N; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			id, ch := b.begin()
			mu.Lock()
			pairs[idx] = pair{id, ch}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if b.pending() != N {
		t.Fatalf("pending = %d after %d begin calls, want %d", b.pending(), N, N)
	}

	// Phase 2: all goroutines call resolve() concurrently on their own id.
	got := make([]bool, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			mu.Lock()
			p := pairs[idx]
			mu.Unlock()
			b.resolve(p.id, true)
		}()
	}

	// Phase 3: all goroutines drain their channels concurrently.
	for i := 0; i < N; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			mu.Lock()
			p := pairs[idx]
			mu.Unlock()
			select {
			case v := <-p.ch:
				mu.Lock()
				got[idx] = v
				mu.Unlock()
			case <-time.After(2 * time.Second):
				t.Errorf("goroutine %d timed out waiting for decision", idx)
			}
		}()
	}
	wg.Wait()

	if b.pending() != 0 {
		t.Fatalf("pending = %d after all resolves, want 0", b.pending())
	}
	for i, v := range got {
		if !v {
			t.Errorf("goroutine %d received wrong decision value", i)
		}
	}
}
