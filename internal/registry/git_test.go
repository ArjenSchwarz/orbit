package registry

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Property-based tests using rapid

func TestPropertyGitURLRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate valid GitHub-style owner/repo names
		// Owners can be alphanumeric with hyphens but not start with hyphen
		owner := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9-]{0,38}`).Draw(t, "owner")
		repo := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9._-]{0,99}`).Draw(t, "repo")

		expected := owner + "/" + repo

		// Test HTTPS format
		httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
		if got := ParseGitRemote(httpsURL); got != expected {
			t.Fatalf("HTTPS: got %q, want %q (url: %s)", got, expected, httpsURL)
		}

		// Test HTTPS format without .git suffix
		httpsURLNoGit := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
		if got := ParseGitRemote(httpsURLNoGit); got != expected {
			t.Fatalf("HTTPS (no .git): got %q, want %q (url: %s)", got, expected, httpsURLNoGit)
		}

		// Test SSH format
		sshURL := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
		if got := ParseGitRemote(sshURL); got != expected {
			t.Fatalf("SSH: got %q, want %q (url: %s)", got, expected, sshURL)
		}

		// Test SSH format without .git suffix
		sshURLNoGit := fmt.Sprintf("git@github.com:%s/%s", owner, repo)
		if got := ParseGitRemote(sshURLNoGit); got != expected {
			t.Fatalf("SSH (no .git): got %q, want %q (url: %s)", got, expected, sshURLNoGit)
		}
	})
}

func TestPropertyGitLabURLRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owner := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9-]{0,38}`).Draw(t, "owner")
		repo := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9._-]{0,99}`).Draw(t, "repo")

		expected := owner + "/" + repo

		// Test HTTPS format for GitLab
		httpsURL := fmt.Sprintf("https://gitlab.com/%s/%s.git", owner, repo)
		if got := ParseGitRemote(httpsURL); got != expected {
			t.Fatalf("GitLab HTTPS: got %q, want %q (url: %s)", got, expected, httpsURL)
		}

		// Test SSH format for GitLab
		sshURL := fmt.Sprintf("git@gitlab.com:%s/%s.git", owner, repo)
		if got := ParseGitRemote(sshURL); got != expected {
			t.Fatalf("GitLab SSH: got %q, want %q (url: %s)", got, expected, sshURL)
		}
	})
}

func TestPropertyConsistentOutput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		owner := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9-]{0,38}`).Draw(t, "owner")
		repo := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9._-]{0,99}`).Draw(t, "repo")

		// All formats should produce the same output
		httpsURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
		sshURL := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
		sshAltURL := fmt.Sprintf("ssh://git@github.com/%s/%s.git", owner, repo)

		httpsResult := ParseGitRemote(httpsURL)
		sshResult := ParseGitRemote(sshURL)
		sshAltResult := ParseGitRemote(sshAltURL)

		if httpsResult != sshResult {
			t.Fatalf("HTTPS and SSH results differ: %q vs %q", httpsResult, sshResult)
		}
		if httpsResult != sshAltResult {
			t.Fatalf("HTTPS and SSH-alt results differ: %q vs %q", httpsResult, sshAltResult)
		}
	})
}

// Unit tests for git URL parsing

func TestParseGitRemote_HTTPS(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "GitHub HTTPS with .git",
			url:      "https://github.com/ArjenSchwarz/orbit.git",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "GitHub HTTPS without .git",
			url:      "https://github.com/ArjenSchwarz/orbit",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "GitLab HTTPS with .git",
			url:      "https://gitlab.com/owner/project.git",
			expected: "owner/project",
		},
		{
			name:     "Bitbucket HTTPS",
			url:      "https://bitbucket.org/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "Custom domain HTTPS",
			url:      "https://git.example.com/team/project.git",
			expected: "team/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGitRemote(tt.url)
			if got != tt.expected {
				t.Errorf("ParseGitRemote(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestParseGitRemote_SSH(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "GitHub SSH with .git",
			url:      "git@github.com:ArjenSchwarz/orbit.git",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "GitHub SSH without .git",
			url:      "git@github.com:ArjenSchwarz/orbit",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "GitLab SSH",
			url:      "git@gitlab.com:owner/project.git",
			expected: "owner/project",
		},
		{
			name:     "Custom domain SSH",
			url:      "git@git.example.com:team/project.git",
			expected: "team/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGitRemote(tt.url)
			if got != tt.expected {
				t.Errorf("ParseGitRemote(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestParseGitRemote_SSHAlt(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "GitHub SSH-alt with .git",
			url:      "ssh://git@github.com/ArjenSchwarz/orbit.git",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "GitHub SSH-alt without .git",
			url:      "ssh://git@github.com/ArjenSchwarz/orbit",
			expected: "ArjenSchwarz/orbit",
		},
		{
			name:     "SSH-alt with port",
			url:      "ssh://git@github.com:22/owner/repo.git",
			expected: "owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGitRemote(tt.url)
			if got != tt.expected {
				t.Errorf("ParseGitRemote(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestParseGitRemote_Invalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "empty string", url: ""},
		{name: "not a URL", url: "not-a-url"},
		{name: "file path", url: "/path/to/repo"},
		{name: "http no path", url: "https://github.com"},
		{name: "http single segment", url: "https://github.com/onlyowner"},
		{name: "malformed SSH", url: "git@github.com"},
		{name: "malformed SSH no path", url: "git@github.com:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGitRemote(tt.url)
			if got != "" {
				t.Errorf("ParseGitRemote(%q) = %q, want empty string", tt.url, got)
			}
		})
	}
}

func TestGetRepository(t *testing.T) {
	// Note: This test is limited because it depends on git being available
	// and the working directory having a git remote configured.
	// We mainly test the fallback behavior here.

	t.Run("fallback to directory name", func(t *testing.T) {
		// Use a directory that definitely doesn't have git
		result := GetRepository("/tmp")
		if result != "tmp" {
			t.Errorf("GetRepository(/tmp) = %q, want %q", result, "tmp")
		}
	})
}
