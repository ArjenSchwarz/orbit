package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/arjenschwarz/orbit/internal/config"
)

// initCommand creates a default .orbit.yaml configuration file.
func initCommand(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "Overwrite existing configuration file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configPath := filepath.Join(workDir, ".orbit.yaml")

	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		if !*force {
			return fmt.Errorf("configuration file .orbit.yaml already exists\n\nUse --force to overwrite the existing file")
		}
	}

	// Write the default configuration
	if err := os.WriteFile(configPath, config.GenerateDefaultConfig(), 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	log.Printf("Created %s", configPath)
	return nil
}
