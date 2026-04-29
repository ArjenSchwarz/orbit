package main

import (
	"flag"
	"fmt"
	"testing"
)

// newCompareFlagSet mirrors the compare subcommand's flag definitions.
func newCompareFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	_ = fs.String("compare-command", "", "")
	_ = fs.String("from-file", "", "")
	return fs
}

// newStatusFlagSet mirrors the status subcommand's flag definitions.
func newStatusFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	_ = fs.String("format", "text", "")
	return fs
}

// newCleanupFlagSet mirrors the cleanup subcommand's flag definitions.
func newCleanupFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	_ = fs.Int("keep", 0, "")
	_ = fs.Bool("force", false, "")
	_ = fs.Bool("dry-run", false, "")
	return fs
}

// TestReorderArgs_CompareSubcommand verifies documented invocation forms for
// orbit compare parse correctly after the T-971 fix.
func TestReorderArgs_CompareSubcommand(t *testing.T) {
	tests := map[string]struct {
		args         []string
		wantSpec     string
		wantFromFile string
	}{
		"spec then --from-file": {
			args:         []string{"my-feature", "--from-file", "specs/my-feature/.orbit/comparison.json"},
			wantSpec:     "my-feature",
			wantFromFile: "specs/my-feature/.orbit/comparison.json",
		},
		"--from-file then spec": {
			args:         []string{"--from-file", "specs/my-feature/.orbit/comparison.json", "my-feature"},
			wantSpec:     "my-feature",
			wantFromFile: "specs/my-feature/.orbit/comparison.json",
		},
		"spec only": {
			args:     []string{"my-feature"},
			wantSpec: "my-feature",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := newCompareFlagSet()
			fromFile := fs.Lookup("from-file")

			if err := fs.Parse(reorderArgs(fs, tc.args)); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if got := fs.Arg(0); got != tc.wantSpec {
				t.Errorf("spec: got %q, want %q", got, tc.wantSpec)
			}
			if got := fromFile.Value.String(); got != tc.wantFromFile {
				t.Errorf("from-file: got %q, want %q", got, tc.wantFromFile)
			}
		})
	}
}

// TestReorderArgs_StatusSubcommand verifies documented invocation forms for
// orbit status parse correctly after the T-971 fix.
func TestReorderArgs_StatusSubcommand(t *testing.T) {
	tests := map[string]struct {
		args       []string
		wantSpec   string
		wantFormat string
	}{
		"spec then --format": {
			args:       []string{"my-feature", "--format", "json"},
			wantSpec:   "my-feature",
			wantFormat: "json",
		},
		"--format then spec": {
			args:       []string{"--format", "json", "my-feature"},
			wantSpec:   "my-feature",
			wantFormat: "json",
		},
		"spec only": {
			args:       []string{"my-feature"},
			wantSpec:   "my-feature",
			wantFormat: "text",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := newStatusFlagSet()
			format := fs.Lookup("format")

			if err := fs.Parse(reorderArgs(fs, tc.args)); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if got := fs.Arg(0); got != tc.wantSpec {
				t.Errorf("spec: got %q, want %q", got, tc.wantSpec)
			}
			if got := format.Value.String(); got != tc.wantFormat {
				t.Errorf("format: got %q, want %q", got, tc.wantFormat)
			}
		})
	}
}

// TestReorderArgs_CleanupSubcommand verifies documented invocation forms for
// orbit cleanup parse correctly after the T-971 fix.
func TestReorderArgs_CleanupSubcommand(t *testing.T) {
	tests := map[string]struct {
		args       []string
		wantSpec   string
		wantKeep   int
		wantDryRun bool
		wantForce  bool
	}{
		"spec then --dry-run": {
			args:       []string{"my-feature", "--dry-run"},
			wantSpec:   "my-feature",
			wantDryRun: true,
		},
		"spec then --keep": {
			args:     []string{"my-feature", "--keep", "1"},
			wantSpec: "my-feature",
			wantKeep: 1,
		},
		"spec then --force": {
			args:      []string{"my-feature", "--force"},
			wantSpec:  "my-feature",
			wantForce: true,
		},
		"spec then multiple flags": {
			args:      []string{"my-feature", "--keep", "2", "--force"},
			wantSpec:  "my-feature",
			wantKeep:  2,
			wantForce: true,
		},
		"flags before spec": {
			args:       []string{"--dry-run", "--keep", "1", "my-feature"},
			wantSpec:   "my-feature",
			wantKeep:   1,
			wantDryRun: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := newCleanupFlagSet()
			keep := fs.Lookup("keep")
			dryRun := fs.Lookup("dry-run")
			force := fs.Lookup("force")

			if err := fs.Parse(reorderArgs(fs, tc.args)); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if got := fs.Arg(0); got != tc.wantSpec {
				t.Errorf("spec: got %q, want %q", got, tc.wantSpec)
			}
			if got := keep.Value.String(); got != "0" && got != "" {
				if got != fmt.Sprintf("%d", tc.wantKeep) {
					t.Errorf("keep: got %s, want %d", got, tc.wantKeep)
				}
			} else if tc.wantKeep != 0 {
				t.Errorf("keep: got %s, want %d", got, tc.wantKeep)
			}
			if got := dryRun.Value.String(); (got == "true") != tc.wantDryRun {
				t.Errorf("dry-run: got %s, want %v", got, tc.wantDryRun)
			}
			if got := force.Value.String(); (got == "true") != tc.wantForce {
				t.Errorf("force: got %s, want %v", got, tc.wantForce)
			}
		})
	}
}
