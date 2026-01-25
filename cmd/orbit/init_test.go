package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommand_NoExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Run init command
	err := initCommand([]string{})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".orbit.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	// Verify content contains expected values
	if !strings.Contains(string(content), "type: claude-code") {
		t.Errorf("config should contain 'type: claude-code', got: %s", content)
	}
	if !strings.Contains(string(content), "auto-approve: true") {
		t.Errorf("config should contain 'auto-approve: true', got: %s", content)
	}
}

func TestInitCommand_ExistingConfigFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing config file
	configPath := filepath.Join(tmpDir, ".orbit.yaml")
	if err := os.WriteFile(configPath, []byte("existing: config"), 0644); err != nil {
		t.Fatalf("failed to create existing config: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Run init command
	err := initCommand([]string{})
	if err == nil {
		t.Fatal("expected error when config already exists")
	}

	// Verify error message
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention file already exists, got: %v", err)
	}

	// Verify original content is unchanged
	content, _ := os.ReadFile(configPath)
	if string(content) != "existing: config" {
		t.Errorf("config should not be modified, got: %s", content)
	}
}

func TestInitCommand_ForceOverwrites(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing config file
	configPath := filepath.Join(tmpDir, ".orbit.yaml")
	if err := os.WriteFile(configPath, []byte("existing: config"), 0644); err != nil {
		t.Fatalf("failed to create existing config: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Run init command with --force
	err := initCommand([]string{"--force"})
	if err != nil {
		t.Fatalf("init --force should succeed: %v", err)
	}

	// Verify file was overwritten
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	// Verify content was replaced
	if strings.Contains(string(content), "existing: config") {
		t.Errorf("config should be overwritten, still contains old content")
	}
	if !strings.Contains(string(content), "type: claude-code") {
		t.Errorf("config should contain new content with 'type: claude-code'")
	}
}

func TestInitCommand_WritePermissionError(t *testing.T) {
	// Skip on Windows as permission handling is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()

	// Make directory read-only
	if err := os.Chmod(tmpDir, 0555); err != nil {
		t.Fatalf("failed to change directory permissions: %v", err)
	}
	// Restore permissions for cleanup
	defer func() { _ = os.Chmod(tmpDir, 0755) }()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Run init command
	err := initCommand([]string{})
	if err == nil {
		t.Fatal("expected error when directory is read-only")
	}

	// Verify error mentions write failure
	if !strings.Contains(err.Error(), "failed to write") {
		t.Errorf("error should mention write failure, got: %v", err)
	}
}

func TestInitCommand_Help(t *testing.T) {
	// Test that --help doesn't error (though it does exit via flag parsing)
	err := initCommand([]string{"-h"})
	// flag.ErrHelp is returned when help is displayed
	if err != nil && err.Error() != "flag: help requested" {
		t.Errorf("help flag returned unexpected error: %v", err)
	}
}
