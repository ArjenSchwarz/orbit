package testutil

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// Recorder tracks all calls made to a TestAgent.
type Recorder struct {
	mu    sync.Mutex
	calls []AgentCall
}

// NewRecorder creates a new Recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// record captures a call. Called by TestAgent.
func (r *Recorder) record(call AgentCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

// CallCount returns the total number of recorded calls.
func (r *Recorder) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// Calls returns a copy of all recorded AgentCall structs.
// Returns a copy for thread safety (copy-on-read).
func (r *Recorder) Calls() []AgentCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]AgentCall, len(r.calls))
	copy(result, r.calls)
	return result
}

// CallsWithPrompt returns calls whose Prompt matches the given regex pattern.
func (r *Recorder) CallsWithPrompt(pattern string) []AgentCall {
	re := regexp.MustCompile(pattern)
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []AgentCall
	for _, call := range r.calls {
		if re.MatchString(call.Options.Prompt) {
			result = append(result, call)
		}
	}
	return result
}

// PhasePromptCalls returns calls matching the /next-task --phase pattern.
func (r *Recorder) PhasePromptCalls() []AgentCall {
	return r.CallsWithPrompt(`/next-task\s+--phase`)
}

// AssertCallCount verifies that exactly the expected number of calls were made.
// Fails the test with a descriptive message if the count doesn't match.
func (r *Recorder) AssertCallCount(t testing.TB, expected int) {
	t.Helper()
	actual := r.CallCount()
	if actual != expected {
		t.Fatalf("call count mismatch: got %d, want %d\nCalls:\n%s",
			actual, expected, r.formatCalls())
	}
}

// AssertCallOrder verifies that prompts match the given patterns in order.
// Each pattern is a regex matched against the call's Prompt field.
// Fails if the number of patterns doesn't match the number of calls,
// or if any pattern doesn't match its corresponding call.
func (r *Recorder) AssertCallOrder(t testing.TB, patterns ...string) {
	t.Helper()
	calls := r.Calls()

	if len(calls) != len(patterns) {
		t.Fatalf("call count mismatch: got %d calls, want %d patterns\nCalls:\n%s",
			len(calls), len(patterns), r.formatCalls())
	}

	for i, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, calls[i].Options.Prompt)
		if err != nil {
			t.Fatalf("invalid pattern %q at index %d: %v", pattern, i, err)
		}
		if !matched {
			t.Fatalf("call %d prompt mismatch:\n  got:  %q\n  want: pattern %q\nAll calls:\n%s",
				i, calls[i].Options.Prompt, pattern, r.formatCalls())
		}
	}
}

// formatCalls returns a formatted string of all recorded calls for error messages.
func (r *Recorder) formatCalls() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.calls) == 0 {
		return "  (no calls recorded)"
	}

	var sb strings.Builder
	for _, call := range r.calls {
		fmt.Fprintf(&sb, "  [%d] %s", call.Index, call.Method)
		if call.SessionID != "" {
			fmt.Fprintf(&sb, " (session: %s)", call.SessionID)
		}
		if call.Options.Prompt != "" {
			prompt := call.Options.Prompt
			if len(prompt) > 80 {
				prompt = prompt[:77] + "..."
			}
			fmt.Fprintf(&sb, ": %q", prompt)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
