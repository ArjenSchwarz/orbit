package testutil

import (
	"sync"
	"testing"
	"time"
)

func TestFakeClock_Advance(t *testing.T) {
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	// Initial time should match start
	if !clock.Now().Equal(start) {
		t.Fatalf("expected initial time %v, got %v", start, clock.Now())
	}

	// Advance by 1 hour
	clock.Advance(time.Hour)
	expected := start.Add(time.Hour)
	if !clock.Now().Equal(expected) {
		t.Fatalf("expected time %v after advance, got %v", expected, clock.Now())
	}

	// Advance by another 30 minutes
	clock.Advance(30 * time.Minute)
	expected = expected.Add(30 * time.Minute)
	if !clock.Now().Equal(expected) {
		t.Fatalf("expected time %v after second advance, got %v", expected, clock.Now())
	}
}

func TestFakeClock_Sleep(t *testing.T) {
	start := time.Now()
	clock := NewFakeClock(start)

	// Sleep should record duration without blocking
	clock.Sleep(time.Second)
	clock.Sleep(2 * time.Second)
	clock.Sleep(500 * time.Millisecond)

	sleeps := clock.Sleeps()
	if len(sleeps) != 3 {
		t.Fatalf("expected 3 sleeps, got %d", len(sleeps))
	}

	expected := []time.Duration{time.Second, 2 * time.Second, 500 * time.Millisecond}
	for i, exp := range expected {
		if sleeps[i] != exp {
			t.Fatalf("sleep %d: expected %v, got %v", i, exp, sleeps[i])
		}
	}

	// Verify Sleep returns immediately (not actually blocking)
	startReal := time.Now()
	clock.Sleep(time.Hour) // Should not actually sleep for an hour
	elapsed := time.Since(startReal)
	if elapsed > time.Second {
		t.Fatalf("Sleep should return immediately, but took %v", elapsed)
	}
}

func TestFakeClock_Concurrent(t *testing.T) {
	clock := NewFakeClock(time.Now())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clock.Now()
			clock.Sleep(time.Duration(idx) * time.Millisecond)
			clock.Advance(time.Millisecond)
		}(i)
	}
	wg.Wait()

	sleeps := clock.Sleeps()
	if len(sleeps) != 100 {
		t.Fatalf("expected 100 sleeps, got %d", len(sleeps))
	}
}

func TestFakeClock_SleepsReturnsCopy(t *testing.T) {
	clock := NewFakeClock(time.Now())
	clock.Sleep(time.Second)
	clock.Sleep(2 * time.Second)

	sleeps := clock.Sleeps()

	// Modify the returned slice
	sleeps[0] = time.Hour

	// Verify original is unchanged
	original := clock.Sleeps()
	if original[0] != time.Second {
		t.Fatalf("expected original to be 1s, got %v", original[0])
	}
}

func TestFakeClock_AssertSleeps(t *testing.T) {
	clock := NewFakeClock(time.Now())
	clock.Sleep(time.Second)
	clock.Sleep(2 * time.Second)

	// Should pass with correct expectations
	clock.AssertSleeps(t, []time.Duration{time.Second, 2 * time.Second})

	// Test failure on count mismatch
	mockT := &mockTB{}
	clock.AssertSleeps(mockT, []time.Duration{time.Second})
	if !mockT.failed {
		t.Fatal("expected AssertSleeps to fail on count mismatch")
	}

	// Test failure on value mismatch
	mockT2 := &mockTB{}
	clock.AssertSleeps(mockT2, []time.Duration{time.Second, time.Second})
	if !mockT2.failed {
		t.Fatal("expected AssertSleeps to fail on value mismatch")
	}
}

func TestFakeClock_NoSleeps(t *testing.T) {
	clock := NewFakeClock(time.Now())

	sleeps := clock.Sleeps()
	if len(sleeps) != 0 {
		t.Fatalf("expected 0 sleeps, got %d", len(sleeps))
	}

	// Should pass with empty expectations
	clock.AssertSleeps(t, []time.Duration{})
}

func TestRealClock_Implements_Clock(t *testing.T) {
	// Verify RealClock implements Clock interface
	var _ Clock = RealClock{}
}

func TestFakeClock_Implements_Clock(t *testing.T) {
	// Verify FakeClock implements Clock interface
	var _ Clock = &FakeClock{}
}
