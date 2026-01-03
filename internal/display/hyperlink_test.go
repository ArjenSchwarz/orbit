package display

import (
	"bytes"
	"os"
	"testing"
)

func TestFormatOSC8Link(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		text     string
		isTTY    bool
		expected string
	}{
		{
			name:     "basic link with TTY",
			uri:      "file:///path/to/file.md",
			text:     "file.md",
			isTTY:    true,
			expected: "\x1b]8;;file:///path/to/file.md\x1b\\file.md\x1b]8;;\x1b\\",
		},
		{
			name:     "plain text without TTY",
			uri:      "file:///path/to/file.md",
			text:     "file.md",
			isTTY:    false,
			expected: "file.md",
		},
		{
			name:     "link with spaces in URI",
			uri:      "file:///path/to/my%20file.md",
			text:     "my file.md",
			isTTY:    true,
			expected: "\x1b]8;;file:///path/to/my%20file.md\x1b\\my file.md\x1b]8;;\x1b\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatOSC8LinkWithTTY(tt.uri, tt.text, tt.isTTY)
			if result != tt.expected {
				t.Errorf("FormatOSC8Link() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatFileLink(t *testing.T) {
	tests := []struct {
		name     string
		absPath  string
		expected string
	}{
		{
			name:     "simple path",
			absPath:  "/Users/foo/file.md",
			expected: "file:///Users/foo/file.md",
		},
		{
			name:     "path with spaces",
			absPath:  "/Users/foo/My Project/file.md",
			expected: "file:///Users/foo/My%20Project/file.md",
		},
		{
			name:     "path with special characters",
			absPath:  "/Users/foo/project [v1]/file.md",
			expected: "file:///Users/foo/project%20%5Bv1%5D/file.md",
		},
		{
			name:     "path with unicode",
			absPath:  "/Users/foo/日本語/file.md",
			expected: "file:///Users/foo/%E6%97%A5%E6%9C%AC%E8%AA%9E/file.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatFileLink(tt.absPath)
			if result != tt.expected {
				t.Errorf("FormatFileLink(%q) = %q, want %q", tt.absPath, result, tt.expected)
			}
		})
	}
}

func TestPrintIndexLinks(t *testing.T) {
	tests := []struct {
		name       string
		sessionDir string
		isTTY      bool
		wantOutput bool
	}{
		{
			name:       "empty session dir",
			sessionDir: "",
			isTTY:      true,
			wantOutput: false,
		},
		{
			name:       "valid session dir with TTY",
			sessionDir: "/tmp/orbit-test",
			isTTY:      true,
			wantOutput: true,
		},
		{
			name:       "valid session dir without TTY",
			sessionDir: "/tmp/orbit-test",
			isTTY:      false,
			wantOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printIndexLinksTo(&buf, tt.sessionDir, tt.isTTY)

			hasOutput := buf.Len() > 0
			if hasOutput != tt.wantOutput {
				t.Errorf("PrintIndexLinks() output = %v, wantOutput %v", hasOutput, tt.wantOutput)
			}

			if tt.wantOutput && tt.sessionDir != "" {
				output := buf.String()
				// Check for expected labels
				if !bytes.Contains(buf.Bytes(), []byte("Markdown:")) {
					t.Error("PrintIndexLinks() missing 'Markdown:' label")
				}
				if !bytes.Contains(buf.Bytes(), []byte("HTML:")) {
					t.Error("PrintIndexLinks() missing 'HTML:' label")
				}
				// Check for index file names
				if !bytes.Contains(buf.Bytes(), []byte("index.md")) {
					t.Errorf("PrintIndexLinks() missing index.md reference, got: %s", output)
				}
				if !bytes.Contains(buf.Bytes(), []byte("index.html")) {
					t.Errorf("PrintIndexLinks() missing index.html reference, got: %s", output)
				}
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	// When running in tests, stderr is typically not a TTY
	// This test just verifies the function runs without error
	result := IsTTY(os.Stderr)
	// We can't assert the specific value as it depends on test environment
	t.Logf("IsTTY(os.Stderr) = %v", result)
}
