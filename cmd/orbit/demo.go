package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	output "github.com/ArjenSchwarz/go-output/v2"

	"github.com/arjenschwarz/orbit/internal/display"
	"github.com/arjenschwarz/orbit/internal/status"
)

// demoCommand routes to the appropriate demo based on arguments.
func demoCommand(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "status":
			return RunStatusDemo()
		case "spinner", "phase":
			return RunSpinnerDemo()
		default:
			fmt.Fprintf(os.Stderr, "Unknown demo: %s\n", args[0])
			fmt.Fprintf(os.Stderr, "Available demos: status, spinner\n")
			return fmt.Errorf("unknown demo: %s", args[0])
		}
	}
	// Default: show available demos
	fmt.Fprintf(os.Stderr, "Usage: orbit demo <type>\n\n")
	fmt.Fprintf(os.Stderr, "Available demos:\n")
	fmt.Fprintf(os.Stderr, "  status   Show the status command output format\n")
	fmt.Fprintf(os.Stderr, "  spinner  Show the spinner/phase overview animation\n")
	return nil
}

// RunSpinnerDemo executes the spinner/phase animation demo.
// It shows a simulated phase overview, runs spinner animations,
// and displays completion links when interrupted.
func RunSpinnerDemo() error {
	// Create spinner - requires TTY
	spin := display.NewSpinner()
	if spin == nil {
		return fmt.Errorf("demo requires a TTY terminal")
	}

	// Set up signal handler for graceful exit
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Orbit Demo - Press Ctrl+C to exit")
	fmt.Fprintln(os.Stderr)

	// Display mock phase overview
	displayMockPhaseOverview()

	fmt.Fprintln(os.Stderr)

	// Run spinner simulation
	phase := 1
	for {
		spin.Start(phase)

		// Simulate work for 10 seconds per phase
		select {
		case <-ctx.Done():
			spin.Stop()
			displayDemoLinks()
			return nil
		case <-time.After(10 * time.Second):
			// Simulate retry wait on even phases
			if phase%2 == 0 {
				spin.UpdateWait(5 * time.Second)
				// Wait with countdown, checking for interrupt
				interrupted := func() bool {
					waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
					defer waitCancel()
					<-waitCtx.Done()
					return ctx.Err() != nil
				}()
				if interrupted {
					spin.Stop()
					displayDemoLinks()
					return nil
				}
			}
			spin.Stop()
			phase++
			if phase > 3 {
				phase = 1 // Loop back
			}
		}
	}
}

// displayMockPhaseOverview renders a sample phase overview table.
func displayMockPhaseOverview() {
	// Build sample table data matching real orbit phase overview
	rows := []map[string]any{
		{"#": 1, "Phase": "Setup", "Tasks": 3, "Completed": 3, "Pending": 0, "Status": "complete"},
		{"#": 2, "Phase": "Implementation", "Tasks": 5, "Completed": 2, "Pending": 3, "Status": "running"},
		{"#": 3, "Phase": "Testing", "Tasks": 4, "Completed": 0, "Pending": 4, "Status": "pending"},
	}

	doc := output.New().
		Table("Phase Overview (Demo)", rows, output.WithKeys("#", "Phase", "Tasks", "Completed", "Pending", "Status")).
		Build()

	out := output.NewOutput(
		output.WithFormat(output.Table()),
		output.WithWriter(output.NewStdoutWriter()),
	)

	ctx := context.Background()
	if err := out.Render(ctx, doc); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to render table: %v\n", err)
	}
}

// displayDemoLinks shows sample completion links.
func displayDemoLinks() {
	display.PrintIndexLinks("/tmp/orbit-demo")
}

// RunStatusDemo executes a demonstration of the orbit status command output.
// It renders mock variant data to show the status display format.
func RunStatusDemo() error {
	mockData := buildMockStatusData()
	return renderStatus(context.Background(), mockData, "text")
}

// buildMockStatusData creates sample status data for demo purposes.
func buildMockStatusData() *status.StatusOutput {
	return &status.StatusOutput{
		SpecName:       "demo-feature",
		BaseCommit:     "abc123def456",
		OriginalBranch: "main",
		StartedAt:      time.Now().Add(-45 * time.Minute).Format("2006-01-02 15:04:05"),
		ActiveVariants: []status.VariantOutput{
			{
				ID:       1,
				Branch:   "orbit/demo-feature/variant-1",
				Worktree: "specs/demo-feature/.orbit/worktrees/variant-1",
				Status:   "running",
				GitState: "dirty",
				Commits: []status.CommitOutput{
					{Hash: "f8e7d6c5b4a3", Subject: "Add user authentication endpoint"},
					{Hash: "a1b2c3d4e5f6", Subject: "Implement session management"},
				},
				LastAction: "Writing auth middleware in internal/auth/middleware.go",
				Tasks: []status.TaskOutput{
					{Phase: "Setup", Completed: 3, Total: 3, IsActive: false},
					{Phase: "Implementation", Completed: 2, Total: 5, IsActive: true},
					{Phase: "Testing", Completed: 0, Total: 4, IsActive: false},
				},
			},
			{
				ID:       2,
				Branch:   "orbit/demo-feature/variant-2",
				Worktree: "specs/demo-feature/.orbit/worktrees/variant-2",
				Status:   "running",
				GitState: "clean",
				Commits: []status.CommitOutput{
					{Hash: "9a8b7c6d5e4f", Subject: "Add OAuth2 provider support"},
				},
				LastAction: "Reading specs/demo-feature/design.md",
				Tasks: []status.TaskOutput{
					{Phase: "Setup", Completed: 3, Total: 3, IsActive: false},
					{Phase: "Implementation", Completed: 1, Total: 5, IsActive: true},
					{Phase: "Testing", Completed: 0, Total: 4, IsActive: false},
				},
			},
		},
		OtherVariants: []status.VariantOutput{
			{
				ID:     3,
				Branch: "orbit/demo-feature/variant-3",
				Status: "pending",
			},
		},
	}
}
