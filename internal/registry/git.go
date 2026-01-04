package registry

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Regex patterns for parsing git remote URLs
var (
	// HTTPS: https://github.com/owner/repo.git or https://github.com/owner/repo
	httpsPattern = regexp.MustCompile(`^https?://[^/]+/([^/]+)/([^/]+?)(?:\.git)?$`)

	// SSH: git@github.com:owner/repo.git or git@github.com:owner/repo
	sshPattern = regexp.MustCompile(`^git@[^:]+:([^/]+)/([^/]+?)(?:\.git)?$`)

	// SSH-alt: ssh://git@github.com/owner/repo.git or ssh://git@github.com:22/owner/repo.git
	sshAltPattern = regexp.MustCompile(`^ssh://[^/]+(?::\d+)?/([^/]+)/([^/]+?)(?:\.git)?$`)
)

// ParseGitRemote extracts owner/repo from a git remote URL.
// Supports HTTPS, SSH, and SSH (alt) formats.
// Returns empty string on parse failure.
func ParseGitRemote(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}

	// Try HTTPS format
	if matches := httpsPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// Try SSH format (git@host:owner/repo)
	if matches := sshPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// Try SSH-alt format (ssh://git@host/owner/repo)
	if matches := sshAltPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	return ""
}

// GetRepository returns the repository identifier for the given working directory.
// It tries to parse the git remote origin URL first.
// Falls back to the directory name if git remote parsing fails.
func GetRepository(workingDir string) string {
	// Try to get git remote origin URL
	cmd := exec.Command("git", "-C", workingDir, "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err == nil {
		url := strings.TrimSpace(string(output))
		if repo := ParseGitRemote(url); repo != "" {
			return repo
		}
	}

	// Fallback to directory name
	return filepath.Base(workingDir)
}
