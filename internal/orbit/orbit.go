// Package orbit provides the main orchestration loop for running Claude Code sessions.
package orbit

import (
	"context"
	"fmt"
	"log"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"
	"github.com/arjenschwarz/orbit/internal/claude"
	orberrors "github.com/arjenschwarz/orbit/internal/errors"
	"github.com/arjenschwarz/orbit/internal/logs"
	"github.com/arjenschwarz/orbit/internal/rune"
)

const (
	maxRetries = 5
)

// Config holds the orchestrator configuration.
type Config struct {
	TasksFile       string
	LogDir          string
	BranchName      string
	SkipPermissions bool
	Verbose         bool
	DryRun          bool
	WorkingDir      string
	Command         string // Custom phase command
	PostCommand     string // Post-completion command (empty = disabled)
}

// Orbit orchestrates Claude Code sessions to implement spec phases.
type Orbit struct {
	config         Config
	runeClient     *rune.Client
	claudeClient   *claude.Client
	logManager     *logs.Manager
	phaseSummaries []rune.PhaseSummary
}

// New creates a new Orbit instance.
func New(config Config) (*Orbit, error) {
	runeClient := rune.NewClient(config.TasksFile)

	claudeClient := claude.NewClient(claude.Config{
		SkipPermissions: config.SkipPermissions,
		WorkingDir:      config.WorkingDir,
		Prompt:          config.Command,
	})

	var logManager *logs.Manager
	if !config.DryRun {
		var err error
		logManager, err = logs.NewManager(config.LogDir, config.BranchName, config.WorkingDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create log manager: %w", err)
		}
		if config.Verbose {
			log.Printf("Logs will be saved to: %s", logManager.SessionDir())
		}
	}

	return &Orbit{
		config:       config,
		runeClient:   runeClient,
		claudeClient: claudeClient,
		logManager:   logManager,
	}, nil
}

// Run executes the orchestration loop until all tasks are complete.
func (o *Orbit) Run() error {
	log.Println("Starting Orbit orchestration...")
	log.Printf("Tasks file: %s", o.config.TasksFile)

	// Display phase overview at startup
	if err := o.displayPhaseOverview(); err != nil {
		log.Printf("Warning: could not display phase overview: %v", err)
	}

	for {
		// Check for remaining tasks
		pending, err := o.runeClient.ListPending()
		if err != nil {
			return o.fail(fmt.Errorf("failed to check pending tasks: %w", err))
		}

		if len(pending) == 0 {
			log.Println("All tasks complete!")
			return o.complete()
		}

		// Get the next phase info
		nextPhase, err := o.runeClient.GetNextPhase()
		if err != nil {
			return o.fail(fmt.Errorf("failed to get next phase: %w", err))
		}

		if nextPhase.AllComplete {
			log.Println("All phases complete!")
			return o.complete()
		}

		// Get the actual phase number from the tasks file
		phaseNum := o.getPhaseNumber(nextPhase.PhaseName)

		if o.config.DryRun {
			log.Printf("[DRY RUN] Would execute phase %d with %d pending tasks", phaseNum, len(pending))
			log.Printf("[DRY RUN] Next phase: %s with %d tasks", nextPhase.PhaseName, len(nextPhase.Tasks))
			if o.config.Command != "" {
				log.Printf("[DRY RUN] Phase command: %s", o.config.Command)
			} else {
				log.Printf("[DRY RUN] Phase command: (default)")
			}
			if o.config.PostCommand != "" {
				log.Printf("[DRY RUN] Post-command: %s", o.config.PostCommand)
			} else {
				log.Printf("[DRY RUN] Post-command: (disabled)")
			}
			return nil
		}

		// Log phase start
		log.Printf("Starting phase %d: %s (%d tasks)", phaseNum, nextPhase.PhaseName, len(nextPhase.Tasks))

		// Run the phase
		if err := o.runPhaseWithRetry(phaseNum); err != nil {
			return o.fail(err)
		}

		log.Printf("Completed phase %d: %s", phaseNum, nextPhase.PhaseName)
	}
}

// complete handles successful orchestration completion, including post-command execution.
func (o *Orbit) complete() error {
	// Run post-command if configured
	if o.config.PostCommand != "" {
		log.Println("Running post-completion command...")
		if err := o.runPostCommandWithRetry(); err != nil {
			log.Printf("Orchestration succeeded but post-command failed: %v", err)
			return o.fail(err)
		}
		log.Println("Post-completion command finished")
	}

	if o.logManager != nil {
		return o.logManager.Complete()
	}
	return nil
}

// getPhaseNumber returns the phase number for a given phase name.
// Returns 0 if the phase is not found.
func (o *Orbit) getPhaseNumber(phaseName string) int {
	for _, s := range o.phaseSummaries {
		if s.Name == phaseName {
			return s.Order
		}
	}
	return 0
}

// displayPhaseOverview shows a table of all phases with their status and task counts.
func (o *Orbit) displayPhaseOverview() error {
	summaries, err := o.runeClient.GetPhaseSummaries()
	if err != nil {
		return err
	}

	// Cache summaries for phase number lookup
	o.phaseSummaries = summaries

	if len(summaries) == 0 {
		return nil
	}

	// Build table data
	rows := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		status := string(s.Status)
		if status == "" {
			status = "-"
		}
		rows = append(rows, map[string]any{
			"#":         s.Order,
			"Phase":     s.Name,
			"Tasks":     s.Total,
			"Completed": s.Completed,
			"Pending":   s.Pending,
			"Status":    status,
		})
	}

	doc := output.New().
		Table("Phase Overview", rows, output.WithKeys("#", "Phase", "Tasks", "Completed", "Pending", "Status")).
		Build()

	out := output.NewOutput(
		output.WithFormat(output.Table()),
		output.WithWriter(output.NewStdoutWriter()),
	)

	fmt.Println() // Add blank line before table
	if err := out.Render(context.Background(), doc); err != nil {
		return fmt.Errorf("failed to render phase table: %w", err)
	}
	fmt.Println() // Add blank line after table

	return nil
}

// runPhaseWithRetry executes a phase with retry logic for transient errors.
func (o *Orbit) runPhaseWithRetry(phase int) error {
	var lastErr error

	for attempt := range maxRetries {
		err := o.runPhase(phase)
		if err == nil {
			return nil
		}

		// Classify the error
		classified, ok := err.(*orberrors.ClassifiedError)
		if !ok {
			// Unknown error type, don't retry
			return err
		}

		lastErr = err

		if !classified.Type.IsRetryable() {
			// Non-retryable error
			return err
		}

		// Determine wait time
		var waitTime time.Duration
		switch classified.Type {
		case orberrors.ErrRateLimit:
			waitTime = classified.RetryAfter
			if waitTime == 0 {
				waitTime = 60 * time.Second
			}
			log.Printf("Rate limited. Waiting %s before retry...", waitTime)

		case orberrors.ErrOverloaded:
			waitTime = 30 * time.Second
			log.Printf("API overloaded. Waiting %s before retry...", waitTime)

		case orberrors.ErrConnection:
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Connection error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)

		default:
			waitTime = orberrors.BackoffDuration(attempt)
		}

		time.Sleep(waitTime)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// runPhase executes a single phase.
func (o *Orbit) runPhase(phase int) error {
	startTime := time.Now()

	result, err := o.claudeClient.RunPhase()
	if err != nil {
		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			_ = o.logManager.SaveSession(phase, result, startTime)
		}

		// Classify and return the error
		classified := orberrors.Classify(1, result.Stderr, result.Output)
		return classified
	}

	// Check if Claude reported an error in its output
	if result.IsError {
		if o.logManager != nil {
			_ = o.logManager.SaveSession(phase, result, startTime)
		}
		classified := orberrors.Classify(1, result.Stderr, result.Output)
		return classified
	}

	// Save successful session
	if o.logManager != nil {
		if err := o.logManager.SaveSession(phase, result, startTime); err != nil {
			log.Printf("Warning: failed to save session log: %v", err)
		}
	}

	if o.config.Verbose {
		log.Printf("Phase %d: cost=$%.4f, duration=%s, turns=%d",
			phase, result.Cost, result.Duration, result.NumTurns)
	}

	return nil
}

// runPostCommand executes the post-completion command.
func (o *Orbit) runPostCommand() error {
	startTime := time.Now()

	result, err := o.claudeClient.RunCustomPrompt(o.config.PostCommand)
	if err != nil {
		// Save the failed session for debugging
		if o.logManager != nil && result != nil {
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := orberrors.Classify(1, result.Stderr, result.Output)
		return classified
	}

	// Check if Claude reported an error in its output
	if result.IsError {
		if o.logManager != nil {
			_ = o.logManager.SavePostCompletionSession(result, startTime)
		}
		classified := orberrors.Classify(1, result.Stderr, result.Output)
		return classified
	}

	// Save successful session
	if o.logManager != nil {
		if err := o.logManager.SavePostCompletionSession(result, startTime); err != nil {
			log.Printf("Warning: failed to save post-completion log: %v", err)
		}
	}

	if o.config.Verbose {
		log.Printf("Post-completion: cost=$%.4f, duration=%s, turns=%d",
			result.Cost, result.Duration, result.NumTurns)
	}

	return nil
}

// runPostCommandWithRetry executes the post-command with retry logic for transient errors.
func (o *Orbit) runPostCommandWithRetry() error {
	var lastErr error

	for attempt := range maxRetries {
		err := o.runPostCommand()
		if err == nil {
			return nil
		}

		// Classify the error
		classified, ok := err.(*orberrors.ClassifiedError)
		if !ok {
			return err
		}

		lastErr = err

		if !classified.Type.IsRetryable() {
			return err
		}

		// Determine wait time
		var waitTime time.Duration
		switch classified.Type {
		case orberrors.ErrRateLimit:
			waitTime = classified.RetryAfter
			if waitTime == 0 {
				waitTime = 60 * time.Second
			}
			log.Printf("Rate limited. Waiting %s before retry...", waitTime)

		case orberrors.ErrOverloaded:
			waitTime = 30 * time.Second
			log.Printf("API overloaded. Waiting %s before retry...", waitTime)

		case orberrors.ErrConnection:
			waitTime = orberrors.BackoffDuration(attempt)
			log.Printf("Connection error (attempt %d/%d). Waiting %s before retry...", attempt+1, maxRetries, waitTime)

		default:
			waitTime = orberrors.BackoffDuration(attempt)
		}

		time.Sleep(waitTime)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// fail marks the orchestration as failed and returns the error.
func (o *Orbit) fail(err error) error {
	if o.logManager != nil {
		_ = o.logManager.Fail(err)
	}
	return err
}
