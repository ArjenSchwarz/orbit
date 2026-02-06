package testutil

import (
	"sync"
	"testing"

	"github.com/arjenschwarz/orbit/internal/agents"
)

func TestRecorder_ConcurrentAccess(t *testing.T) {
	r := NewRecorder()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.record(AgentCall{
				Index:  idx,
				Method: "Run",
				Options: agents.RunOptions{
					Prompt: "test prompt",
				},
			})
			_ = r.Calls()
			_ = r.CallCount()
		}(i)
	}
	wg.Wait()

	if r.CallCount() != 100 {
		t.Fatalf("expected 100 calls, got %d", r.CallCount())
	}
}

func TestRecorder_CallsReturnsCopy(t *testing.T) {
	r := NewRecorder()
	r.record(AgentCall{Index: 0, Method: "Run"})
	r.record(AgentCall{Index: 1, Method: "Resume"})

	calls := r.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	// Modify the returned slice
	calls[0].Method = "Modified"

	// Verify original is unchanged
	original := r.Calls()
	if original[0].Method != "Run" {
		t.Fatalf("expected original to be 'Run', got %q", original[0].Method)
	}
}

func TestRecorder_CallsWithPrompt(t *testing.T) {
	r := NewRecorder()
	r.record(AgentCall{Index: 0, Method: "Run", Options: agents.RunOptions{Prompt: "/next-task --phase"}})
	r.record(AgentCall{Index: 1, Method: "Run", Options: agents.RunOptions{Prompt: "Review the code"}})
	r.record(AgentCall{Index: 2, Method: "Run", Options: agents.RunOptions{Prompt: "/next-task --phase 2"}})

	// Test pattern matching
	matches := r.CallsWithPrompt(`/next-task`)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Test PhasePromptCalls convenience method
	phaseMatches := r.PhasePromptCalls()
	if len(phaseMatches) != 2 {
		t.Fatalf("expected 2 phase matches, got %d", len(phaseMatches))
	}
}

func TestRecorder_AssertCallCount(t *testing.T) {
	r := NewRecorder()
	r.record(AgentCall{Index: 0, Method: "Run"})
	r.record(AgentCall{Index: 1, Method: "Resume"})

	// Should pass - correct count
	r.AssertCallCount(t, 2)

	// Test failure message by using a mock testing.TB
	mockT := &mockTB{}
	r.AssertCallCount(mockT, 5)
	if !mockT.failed {
		t.Fatal("expected AssertCallCount to fail when count doesn't match")
	}
	if mockT.message == "" {
		t.Fatal("expected failure message to be non-empty")
	}
}

func TestRecorder_AssertCallOrder(t *testing.T) {
	r := NewRecorder()
	r.record(AgentCall{Index: 0, Method: "Run", Options: agents.RunOptions{Prompt: "first prompt"}})
	r.record(AgentCall{Index: 1, Method: "Resume", Options: agents.RunOptions{Prompt: "second prompt"}})
	r.record(AgentCall{Index: 2, Method: "Run", Options: agents.RunOptions{Prompt: "third prompt"}})

	// Should pass with correct patterns
	r.AssertCallOrder(t, "first", "second", "third")

	// Test with regex patterns
	r.AssertCallOrder(t, "^first.*", ".*second.*", "third.*")

	// Test failure on wrong pattern count
	mockT := &mockTB{}
	r.AssertCallOrder(mockT, "first", "second")
	if !mockT.failed {
		t.Fatal("expected AssertCallOrder to fail when pattern count doesn't match")
	}

	// Test failure on pattern mismatch
	mockT2 := &mockTB{}
	r.AssertCallOrder(mockT2, "first", "WRONG", "third")
	if !mockT2.failed {
		t.Fatal("expected AssertCallOrder to fail when pattern doesn't match")
	}
}

func TestRecorder_FormatCallsEmpty(t *testing.T) {
	r := NewRecorder()
	formatted := r.formatCalls()
	if formatted != "  (no calls recorded)" {
		t.Fatalf("expected empty message, got %q", formatted)
	}
}

func TestRecorder_FormatCallsTruncatesLongPrompts(t *testing.T) {
	r := NewRecorder()
	longPrompt := "This is a very long prompt that should be truncated because it exceeds eighty characters in length"
	r.record(AgentCall{Index: 0, Method: "Run", Options: agents.RunOptions{Prompt: longPrompt}})

	formatted := r.formatCalls()
	if len(formatted) > 200 { // Give some room for formatting
		t.Fatalf("formatted output should be reasonably short, got length %d", len(formatted))
	}
	if formatted[len(formatted)-5:] != "...\"\n" {
		t.Fatalf("expected truncated prompt to end with ..., got %q", formatted)
	}
}

// mockTB is a minimal testing.TB implementation for testing assertion failures.
type mockTB struct {
	testing.TB
	failed  bool
	message string
}

func (m *mockTB) Helper() {}

func (m *mockTB) Fatalf(format string, args ...any) {
	m.failed = true
	m.message = format
}
