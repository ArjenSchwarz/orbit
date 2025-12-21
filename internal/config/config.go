// Package config handles configuration loading for Orbit using Viper.
package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	// DefaultCommand is the default prompt used for Claude during phase execution.
	DefaultCommand = "Run /next-task --phase and when complete run /commit"
	// DefaultPostCommand is the default prompt executed after all tasks complete.
	DefaultPostCommand = "Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."
)

// Config holds the resolved configuration values.
type Config struct {
	Command     string
	PostCommand string

	// postCommandExplicit tracks whether post-command was explicitly set in config.
	// This allows distinguishing "not set" (use default) from "set to empty" (disabled).
	postCommandExplicit bool
}

// Load reads configuration from home and project directories using Viper.
//
// Priority (highest to lowest):
//  1. Environment variables (ORBIT_COMMAND, ORBIT_POST_COMMAND)
//  2. Project config (.orbit.yaml in working directory)
//  3. Home config (~/.orbit.yaml)
//  4. Built-in defaults
//
// Environment variables are checked using os.LookupEnv rather than Viper's
// AutomaticEnv() because Viper cannot distinguish between "not set" and
// "set to empty string". This distinction matters for post-command: setting
// ORBIT_POST_COMMAND="" explicitly disables the post-command, while not
// setting it uses the default.
func Load(workingDir string) *Config {
	v := viper.New()

	// Set defaults
	v.SetDefault("command", DefaultCommand)
	v.SetDefault("post-command", DefaultPostCommand)

	// Config file name (without extension)
	v.SetConfigName(".orbit")
	v.SetConfigType("yaml")

	// Track if post-command was explicitly set in either config file
	postCommandExplicit := false

	// Load home config first (lowest priority for files)
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(homeDir, ".orbit.yaml")
		if _, statErr := os.Stat(homeConfigPath); statErr == nil {
			homeViper := viper.New()
			homeViper.SetConfigFile(homeConfigPath)
			if err := homeViper.ReadInConfig(); err != nil {
				log.Printf("Warning: could not read %s: %v", homeConfigPath, err)
			} else {
				// Track if home config explicitly set post-command
				if homeViper.IsSet("post-command") {
					postCommandExplicit = true
				}
				// Merge home config into main viper
				if err := v.MergeConfigMap(homeViper.AllSettings()); err != nil {
					log.Printf("Warning: could not merge %s: %v", homeConfigPath, err)
				}
			}
		}
	}

	// Load project config (higher priority, merges with home)
	projectConfigPath := filepath.Join(workingDir, ".orbit.yaml")
	if _, statErr := os.Stat(projectConfigPath); statErr == nil {
		projectViper := viper.New()
		projectViper.SetConfigFile(projectConfigPath)

		if err := projectViper.ReadInConfig(); err != nil {
			log.Printf("Warning: could not read %s: %v", projectConfigPath, err)
		} else {
			// Check if post-command was explicitly set in project config
			if projectViper.IsSet("post-command") {
				postCommandExplicit = true
			}

			// Merge project config into main viper
			if err := v.MergeConfigMap(projectViper.AllSettings()); err != nil {
				log.Printf("Warning: could not merge %s: %v", projectConfigPath, err)
			}
		}
	}

	// Get values from config files (with defaults as fallback)
	command := v.GetString("command")
	postCommand := v.GetString("post-command")

	// Apply environment variable overrides (highest priority)
	// Using os.LookupEnv to detect both set values and explicitly empty values
	if envCmd, exists := os.LookupEnv("ORBIT_COMMAND"); exists {
		command = envCmd
	}
	if envPostCmd, exists := os.LookupEnv("ORBIT_POST_COMMAND"); exists {
		postCommand = envPostCmd
		postCommandExplicit = true
	}

	return &Config{
		Command:             command,
		PostCommand:         postCommand,
		postCommandExplicit: postCommandExplicit,
	}
}

// IsPostCommandDisabled returns true if post-command was explicitly set to empty.
// This allows distinguishing "use default" from "disable".
func (c *Config) IsPostCommandDisabled() bool {
	return c.postCommandExplicit && c.PostCommand == ""
}
