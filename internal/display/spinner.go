package display

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
)

// Spinner configuration constants.
const (
	spinnerCharSet  = 14 // Braille dots
	spinnerInterval = 100 * time.Millisecond
	spinnerColor    = "fgCyan"
)

// Spinner wraps briandowns/spinner with orbit-specific behavior.
type Spinner struct {
	spinner          *spinner.Spinner
	startTime        time.Time
	phase            int
	isWaiting        bool
	isPostCompletion bool
	waitEndTime      time.Time
	done             chan struct{}
	mu               sync.Mutex
	started          bool
	stopOnce         sync.Once
}

// NewSpinner creates a spinner configured for orbit.
// Returns nil if stderr is not a TTY.
func NewSpinner() *Spinner {
	return newSpinnerWithTTY(IsTTY(os.Stderr))
}

// newSpinnerWithTTY is the internal constructor for testing.
func newSpinnerWithTTY(isTTY bool) *Spinner {
	if !isTTY {
		return nil
	}

	s := spinner.New(spinner.CharSets[spinnerCharSet], spinnerInterval,
		spinner.WithWriter(os.Stderr),
		spinner.WithColor(spinnerColor),
	)

	return &Spinner{
		spinner: s,
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation for a phase.
// Idempotent: calling Start() when already started is a no-op.
func (s *Spinner) Start(phase int) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return
	}

	s.phase = phase
	s.startTime = time.Now()
	s.started = true
	s.isWaiting = false
	s.isPostCompletion = false
	s.stopOnce = sync.Once{}
	s.done = make(chan struct{})

	s.updateSuffix()
	s.spinner.Start()

	// Start goroutine to update elapsed time
	go s.updateLoop()
}

// StartPostCompletion begins spinner for post-completion command.
func (s *Spinner) StartPostCompletion() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return
	}

	s.phase = 0
	s.startTime = time.Now()
	s.started = true
	s.isWaiting = false
	s.isPostCompletion = true
	s.stopOnce = sync.Once{}
	s.done = make(chan struct{})

	s.updateSuffix()
	s.spinner.Start()

	go s.updateLoop()
}

// UpdateWait switches to wait mode with countdown.
func (s *Spinner) UpdateWait(remaining time.Duration) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.isWaiting = true
	s.waitEndTime = time.Now().Add(remaining)
	s.updateSuffix()
}

// ResumePhase switches back from wait mode to normal phase mode.
func (s *Spinner) ResumePhase() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.isWaiting = false
	s.updateSuffix()
}

// Pause temporarily stops the spinner to allow log output.
func (s *Spinner) Pause() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return
	}

	s.spinner.Stop()
}

// Resume restarts the spinner after Pause().
func (s *Spinner) Resume() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return
	}

	s.updateSuffix()
	s.spinner.Start()
}

// Stop halts the spinner and clears the line.
// Idempotent: calling Stop() multiple times is safe.
func (s *Spinner) Stop() {
	if s == nil {
		return
	}

	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.started {
			close(s.done)
			s.started = false
		}
		s.mu.Unlock()

		s.spinner.Stop()
	})
}

// updateLoop periodically updates the spinner suffix with elapsed time.
func (s *Spinner) updateLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.updateSuffix()
			s.mu.Unlock()
		}
	}
}

// updateSuffix updates the spinner's suffix text.
// Must be called with mutex held.
func (s *Spinner) updateSuffix() {
	var prefix string
	if s.isPostCompletion {
		prefix = " Post-completion"
	} else {
		prefix = fmt.Sprintf(" Phase %d", s.phase)
	}

	var suffix string
	if s.isWaiting {
		remaining := time.Until(s.waitEndTime)
		if remaining < 0 {
			remaining = 0
		}
		suffix = fmt.Sprintf(" [waiting %ds]", int(remaining.Seconds()))
	} else {
		elapsed := time.Since(s.startTime)
		minutes := int(elapsed.Minutes())
		seconds := int(elapsed.Seconds()) % 60
		suffix = fmt.Sprintf(" [%dm %02ds]", minutes, seconds)
	}

	s.spinner.Suffix = prefix + suffix
}
