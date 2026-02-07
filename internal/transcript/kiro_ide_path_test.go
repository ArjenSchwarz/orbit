package transcript

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"pgregory.net/rapid"
)

func TestSha256Hex32(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"workspace path": {
			input: "/Users/alice/myproject",
			want:  "5695780b2e5fc2b9c7dd41fc24065cc0",
		},
		"execution saves constant": {
			input: "KIRO::EXECUTION::SAVES",
			want:  "414d1636299d2b9e4ce7e17fb11f63e9",
		},
		"execution id": {
			input: "ccfd398f-c4d8-44d7-ad56-532bb7f2ffa1",
			want:  "40defd79b14eb45d143d1eba8d157e54",
		},
		"empty string": {
			input: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb924", // sha256("")[:32]
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := sha256Hex32(tc.input)
			if got != tc.want {
				t.Errorf("sha256Hex32(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPropertySha256Hex32(t *testing.T) {
	hexPattern := regexp.MustCompile(`^[0-9a-f]{32}$`)

	rapid.Check(t, func(rt *rapid.T) {
		input := rapid.String().Draw(rt, "input")
		result := sha256Hex32(input)

		// Property 1: output is always exactly 32 hex characters
		if !hexPattern.MatchString(result) {
			rt.Fatalf("sha256Hex32(%q) = %q, want 32 hex chars", input, result)
		}

		// Property 2: deterministic — same input produces same output
		result2 := sha256Hex32(input)
		if result != result2 {
			rt.Fatalf("sha256Hex32(%q) not deterministic: %q != %q", input, result, result2)
		}
	})
}

func TestKiroIDEBasePath(t *testing.T) {
	path, err := KiroIDEBasePath()
	if err != nil {
		// On systems without a config dir this is acceptable
		t.Skipf("KiroIDEBasePath returned error (expected on some CI): %v", err)
	}

	// Verify it ends with the expected subdirectory
	wantSuffix := filepath.Join("Kiro", "User", "globalStorage", "kiro.kiroagent")
	if !hasPathSuffix(path, wantSuffix) {
		t.Errorf("KiroIDEBasePath() = %q, want suffix %q", path, wantSuffix)
	}
}

func TestKiroIDEWorkspaceDir(t *testing.T) {
	// Create a fake Kiro IDE storage structure
	baseDir := t.TempDir()
	projectPath := "/Users/alice/myproject"
	workspaceHash := sha256Hex32(projectPath)
	workspaceDir := filepath.Join(baseDir, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	tests := map[string]struct {
		projectPath string
		wantDir     string
		wantErr     error
	}{
		"existing workspace": {
			projectPath: projectPath,
			wantDir:     workspaceDir,
		},
		"non-existent workspace": {
			projectPath: "/does/not/exist",
			wantErr:     ErrKiroIDENotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := kiroIDEWorkspaceDirWithBase(baseDir, tc.projectPath)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Errorf("got error %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantDir {
				t.Errorf("got %q, want %q", got, tc.wantDir)
			}
		})
	}
}

func TestKiroIDEWorkspaceDir_PathNormalization(t *testing.T) {
	// Create a real temp directory to use as the project path
	// so filepath.Abs works correctly
	projectDir := t.TempDir()
	baseDir := t.TempDir()

	// Create workspace dir for the normalized project path
	workspaceHash := sha256Hex32(projectDir)
	workspaceDir := filepath.Join(baseDir, workspaceHash)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Path with trailing slash should resolve to the same workspace
	pathWithSlash := projectDir + "/"
	got, err := kiroIDEWorkspaceDirWithBase(baseDir, pathWithSlash)
	if err != nil {
		t.Fatalf("unexpected error for path with trailing slash: %v", err)
	}
	if got != workspaceDir {
		t.Errorf("path with trailing slash: got %q, want %q", got, workspaceDir)
	}
}

func TestKiroIDEExecutionDetailPath(t *testing.T) {
	workspaceDir := "/fake/workspace/dir"
	executionID := "ccfd398f-c4d8-44d7-ad56-532bb7f2ffa1"

	got := KiroIDEExecutionDetailPath(workspaceDir, executionID)

	// The path should be: workspaceDir / executionSavesDir / sha256Hex32(executionID)
	wantParts := []string{
		workspaceDir,
		"414d1636299d2b9e4ce7e17fb11f63e9",
		"40defd79b14eb45d143d1eba8d157e54",
	}
	want := filepath.Join(wantParts...)
	if got != want {
		t.Errorf("KiroIDEExecutionDetailPath() = %q, want %q", got, want)
	}
}

// hasPathSuffix checks if path ends with the given suffix components.
func hasPathSuffix(path, suffix string) bool {
	// Normalize both to use the same separator
	path = filepath.Clean(path)
	suffix = filepath.Clean(suffix)
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
