package claude

import "testing"

func TestBuildProjectPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unix absolute path",
			input:    "/Users/foo/project",
			expected: "-Users-foo-project",
		},
		{
			name:     "unix relative path",
			input:    "foo/project",
			expected: "foo-project",
		},
		{
			name:     "windows absolute path",
			input:    `C:\Users\foo\project`,
			expected: `C:-Users-foo-project`,
		},
		{
			name:     "windows relative path",
			input:    `foo\project`,
			expected: `foo-project`,
		},
		{
			name:     "mixed separators",
			input:    `/Users/foo\project`,
			expected: `-Users-foo-project`,
		},
		{
			name:     "single directory",
			input:    "/project",
			expected: "-project",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildProjectPath(tt.input)
			if result != tt.expected {
				t.Errorf("BuildProjectPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
