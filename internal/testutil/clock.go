package testutil

import (
	"sync"
	"testing"
	"time"
)

// RealClock uses actual time functions.
// This matches the RealClock in internal/orbit/orbit.go.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// Sleep pauses execution for the specified duration.
func (RealClock) Sleep(d time.Duration) { time.Sleep(d) }

// FakeClock provides controllable time for tests.
// It records all sleep requests and returns immediately instead of blocking.
//
// Limitations:
// This FakeClock only supports Now() and Sleep(). Timer-based code
// (time.After, time.NewTimer, time.NewTicker) is not supported and will
// use real time. This is documented as a scope limitation for MVP.
type FakeClock struct {
	mu      sync.Mutex
	current time.Time
	sleeps  []time.Duration
}

// NewFakeClock creates a fake clock starting at the given time.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{current: start}
}

// Now returns the controlled current time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// Advance moves time forward by the specified duration.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(d)
}

// Sleep records the sleep request and returns immediately.
// Unlike real sleep, this does not block.
func (c *FakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sleeps = append(c.sleeps, d)
}

// Sleeps returns a copy of all recorded sleep durations.
func (c *FakeClock) Sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]time.Duration, len(c.sleeps))
	copy(result, c.sleeps)
	return result
}

// AssertSleeps verifies that the recorded sleeps match the expected durations.
func (c *FakeClock) AssertSleeps(t testing.TB, expected []time.Duration) {
	t.Helper()
	actual := c.Sleeps()

	if len(actual) != len(expected) {
		t.Fatalf("sleep count mismatch: got %d, want %d\n  actual: %v\n  expected: %v",
			len(actual), len(expected), actual, expected)
	}

	for i, exp := range expected {
		if actual[i] != exp {
			t.Fatalf("sleep %d mismatch: got %v, want %v\n  actual: %v\n  expected: %v",
				i, actual[i], exp, actual, expected)
		}
	}
}
