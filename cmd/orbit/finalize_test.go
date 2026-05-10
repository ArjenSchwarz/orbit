package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arjenschwarz/orbit/internal/consolidation"
	"github.com/arjenschwarz/orbit/internal/variants"
)

// setupFinalizeRepo creates a git repo with a single completed variant whose
// agent fields are configurable. It returns the repo root.
func setupFinalizeRepo(t *testing.T, specName string, agent, agentType, model string) string {
	t.Helper()
	tmpDir := t.TempDir()

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", args[0], out)
	}

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	require.NoError(t, err)
	baseCommit := strings.TrimSpace(string(out))

	specOrbitDir := filepath.Join(tmpDir, "specs", specName, ".orbit")
	require.NoError(t, os.MkdirAll(specOrbitDir, 0755))
	worktreePath := filepath.Join(specOrbitDir, "worktrees", "orbit-impl-1-"+specName)
	require.NoError(t, os.MkdirAll(worktreePath, 0755))

	metadata := variants.VariantsMetadata{
		RunID:          "test-run",
		BaseCommit:     baseCommit,
		OriginalBranch: "main",
		StartedAt:      time.Now(),
		Variants: []*variants.Variant{
			{
				ID:           1,
				Branch:       "orbit-impl-1-" + specName,
				WorktreePath: worktreePath,
				Status:       variants.StatusCompleted,
				Agent:        agent,
				AgentType:    agentType,
				Model:        model,
			},
		},
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(specOrbitDir, "variants.json"), data, 0644))

	return tmpDir
}

// runFinalizeCapture runs finalizeCommand with the given args, redirecting
// stdin to "n\n" so the confirmation prompt cancels (avoiding any rebase),
// and returns captured stdout plus any returned error.
func runFinalizeCapture(t *testing.T, args []string) (string, error) {
	t.Helper()

	originalStdin := os.Stdin
	stdinR, stdinW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = stdinR
	_, _ = stdinW.WriteString("n\n")
	_ = stdinW.Close()
	t.Cleanup(func() { os.Stdin = originalStdin })

	originalStdout := os.Stdout
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = stdoutW

	cmdErr := finalizeCommand(args)

	_ = stdoutW.Close()
	os.Stdout = originalStdout

	captured, readErr := io.ReadAll(stdoutR)
	require.NoError(t, readErr)

	return string(captured), cmdErr
}

func TestFormatVariantAgentInfo(t *testing.T) {
	tests := map[string]struct {
		agent     string
		agentType string
		model     string
		want      string
	}{
		"all three populated": {
			agent:     "primary",
			agentType: "claude-code",
			model:     "claude-opus-4-7",
			want:      "Agent: primary (claude-code, model: claude-opus-4-7)",
		},
		"only agent populated": {
			agent: "primary",
			want:  "Agent: primary",
		},
		"agent and type, no model": {
			agent:     "primary",
			agentType: "claude-code",
			want:      "Agent: primary (claude-code)",
		},
		"only model populated": {
			model: "claude-opus-4-7",
			want:  "Agent: (model: claude-opus-4-7)",
		},
		"all empty": {
			want: "Agent: unknown",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			v := &variants.Variant{
				Agent:     tc.agent,
				AgentType: tc.agentType,
				Model:     tc.model,
			}
			got := formatVariantAgentInfo(v)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFinalizeCommand_AgentInfoPreamble(t *testing.T) {
	tests := map[string]struct {
		agent     string
		agentType string
		model     string
		wantLine  string
	}{
		"all three populated": {
			agent:     "primary",
			agentType: "claude-code",
			model:     "claude-opus-4-7",
			wantLine:  "Agent: primary (claude-code, model: claude-opus-4-7)",
		},
		"only agent populated": {
			agent:    "primary",
			wantLine: "Agent: primary",
		},
		"agent and type, no model": {
			agent:     "primary",
			agentType: "claude-code",
			wantLine:  "Agent: primary (claude-code)",
		},
		"only model populated": {
			model:    "claude-opus-4-7",
			wantLine: "Agent: (model: claude-opus-4-7)",
		},
		"all empty": {
			wantLine: "Agent: unknown",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			specName := "agent-info-" + strings.ReplaceAll(name, " ", "-")
			repoRoot := setupFinalizeRepo(t, specName, tc.agent, tc.agentType, tc.model)

			originalWd, err := os.Getwd()
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Chdir(originalWd) })
			require.NoError(t, os.Chdir(repoRoot))

			out, _ := runFinalizeCapture(t, []string{"--variant", "1", specName})
			assert.Contains(t, out, tc.wantLine)
		})
	}
}

// writeConsolidationLog writes a fixture consolidation-log.json containing the
// given entries to the spec's .orbit directory.
func writeConsolidationLog(t *testing.T, repoRoot, specName string, entries []consolidation.LogEntry) {
	t.Helper()
	orbitDir := filepath.Join(repoRoot, "specs", specName, ".orbit")
	log := consolidation.ConsolidationLog{
		SchemaVersion: consolidation.SchemaVersion,
		Entries:       entries,
	}
	data, err := json.MarshalIndent(log, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(orbitDir, "consolidation-log.json"), data, 0644))
}

// writeRawConsolidationLog writes raw bytes to consolidation-log.json (for
// corrupt-JSON test fixtures).
func writeRawConsolidationLog(t *testing.T, repoRoot, specName string, data []byte) {
	t.Helper()
	orbitDir := filepath.Join(repoRoot, "specs", specName, ".orbit")
	require.NoError(t, os.WriteFile(filepath.Join(orbitDir, "consolidation-log.json"), data, 0644))
}

func TestFinalizeCommand_ConsolidationMismatchWarning(t *testing.T) {
	priorTimestamp := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	priorRFC3339 := priorTimestamp.Format(time.RFC3339)

	mismatchEntry := consolidation.LogEntry{
		Timestamp:       priorTimestamp,
		ChosenVariantID: 2,
		Agent:           "claude-code",
	}
	matchingEntry := consolidation.LogEntry{
		Timestamp:       priorTimestamp,
		ChosenVariantID: 1,
		Agent:           "claude-code",
	}

	tests := map[string]struct {
		setupLog    func(t *testing.T, repoRoot, specName string)
		args        []string
		wantWarning bool
	}{
		"mismatch fires warning": {
			setupLog: func(t *testing.T, repoRoot, specName string) {
				writeConsolidationLog(t, repoRoot, specName, []consolidation.LogEntry{mismatchEntry})
			},
			args:        []string{"--variant", "1"},
			wantWarning: true,
		},
		"matching entry produces no warning": {
			setupLog: func(t *testing.T, repoRoot, specName string) {
				writeConsolidationLog(t, repoRoot, specName, []consolidation.LogEntry{matchingEntry})
			},
			args:        []string{"--variant", "1"},
			wantWarning: false,
		},
		"missing log file produces no warning": {
			setupLog:    func(t *testing.T, repoRoot, specName string) {},
			args:        []string{"--variant", "1"},
			wantWarning: false,
		},
		"corrupt json log produces no warning": {
			setupLog: func(t *testing.T, repoRoot, specName string) {
				writeRawConsolidationLog(t, repoRoot, specName, []byte("{not valid json"))
			},
			args:        []string{"--variant", "1"},
			wantWarning: false,
		},
		"empty entries slice produces no warning": {
			setupLog: func(t *testing.T, repoRoot, specName string) {
				writeConsolidationLog(t, repoRoot, specName, []consolidation.LogEntry{})
			},
			args:        []string{"--variant", "1"},
			wantWarning: false,
		},
		"warning prints under force": {
			setupLog: func(t *testing.T, repoRoot, specName string) {
				writeConsolidationLog(t, repoRoot, specName, []consolidation.LogEntry{mismatchEntry})
			},
			args:        []string{"--variant", "1", "--force"},
			wantWarning: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			specName := "mismatch-" + strings.ReplaceAll(name, " ", "-")
			repoRoot := setupFinalizeRepo(t, specName, "primary", "claude-code", "claude-opus-4-7")
			tc.setupLog(t, repoRoot, specName)

			originalWd, err := os.Getwd()
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Chdir(originalWd) })
			require.NoError(t, os.Chdir(repoRoot))

			args := append([]string{}, tc.args...)
			args = append(args, specName)
			out, _ := runFinalizeCapture(t, args)

			if tc.wantWarning {
				assert.Contains(t, out, "Warning:", "expected warning in output")
				assert.Contains(t, out, "variant 1", "expected requested variant ID in warning")
				assert.Contains(t, out, "variant 2", "expected prior chosen variant ID in warning")
				assert.Contains(t, out, priorRFC3339, "expected RFC3339 timestamp in warning")
			} else {
				assert.NotContains(t, out, "Warning:", "expected no warning in output")
			}
		})
	}
}
