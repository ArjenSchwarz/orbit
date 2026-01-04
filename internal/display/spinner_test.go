package display

import (
	"testing"
	"time"
)

func TestNewSpinner(t *testing.T) {
	// When not a TTY, NewSpinner should return nil
	// In tests, stderr is typically not a TTY
	s := NewSpinner()
	if s != nil {
		// If we got a spinner (running in TTY), we should be able to stop it
		s.Stop()
		t.Log("NewSpinner() returned a spinner (TTY detected)")
	} else {
		t.Log("NewSpinner() returned nil (not a TTY, as expected in tests)")
	}
}

func TestSpinnerStartIdempotent(t *testing.T) {
	// Create a spinner with forced TTY for testing
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	// First Start should work
	s.Start(1)
	if !s.started {
		t.Error("Start() should set started to true")
	}

	// Second Start should be a no-op
	s.Start(2)
	if s.phase != 1 {
		t.Error("Second Start() should not change phase")
	}
}

func TestSpinnerStopIdempotent(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}

	s.Start(1)

	// First Stop should work
	s.Stop()

	// Second Stop should be a no-op (no panic)
	s.Stop()
	// Third Stop should also be safe
	s.Stop()
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}

	// Stop without Start should be a no-op
	s.Stop()
}

func TestSpinnerStartPostCompletion(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	s.StartPostCompletion()
	if !s.started {
		t.Error("StartPostCompletion() should set started to true")
	}
	if s.phase != 0 {
		t.Error("StartPostCompletion() should use phase 0")
	}
	if !s.isPostCompletion {
		t.Error("StartPostCompletion() should set isPostCompletion to true")
	}
}

func TestSpinnerUpdateWait(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	s.Start(1)
	s.UpdateWait(30 * time.Second)

	if !s.isWaiting {
		t.Error("UpdateWait() should set isWaiting to true")
	}
}

func TestSpinnerResumePhase(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	s.Start(1)
	s.UpdateWait(30 * time.Second)
	s.ResumePhase()

	if s.isWaiting {
		t.Error("ResumePhase() should set isWaiting to false")
	}
}

func TestSpinnerPauseResume(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	s.Start(1)

	// Pause should stop the spinner
	s.Pause()

	// Resume should restart it
	s.Resume()
}

func TestSpinnerPauseWithoutStart(t *testing.T) {
	s := newSpinnerWithTTY(true)
	if s == nil {
		t.Skip("Could not create test spinner")
	}
	defer s.Stop()

	// Pause without Start should be a no-op
	s.Pause()
	s.Resume()
}

func TestNilSpinnerMethods(t *testing.T) {
	// All methods should be safe to call on nil spinner
	var s *Spinner

	// These should not panic
	s.Start(1)
	s.StartPostCompletion()
	s.UpdateWait(10 * time.Second)
	s.ResumePhase()
	s.Pause()
	s.Resume()
	s.Stop()
}
