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
)

// RunDemo executes a demonstration of orbit's UX features.
// It shows a simulated phase overview, runs spinner animations,
// and displays completion links when interrupted.
func RunDemo() error {
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
