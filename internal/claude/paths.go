package claude

import "strings"

// BuildProjectPath converts a project path to Claude's projects directory format.
// Example: /Users/foo/project -> -Users-foo-project
// The leading dash is preserved to match Claude's directory structure.
func BuildProjectPath(projectPath string) string {
	// Replace path separators with dashes (leading separator becomes leading dash)
	p := strings.ReplaceAll(projectPath, "/", "-")
	p = strings.ReplaceAll(p, "\\", "-")
	return p
}
