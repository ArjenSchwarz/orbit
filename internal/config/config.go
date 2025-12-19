// Package config handles configuration loading for Orbit using Viper.
package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

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
// Priority: environment variables > project config > home config > defaults.
// Also reads ORBIT_COMMAND and ORBIT_POST_COMMAND environment variables.
func Load(workingDir string) *Config {
	v := viper.New()

	// Set defaults
	v.SetDefault("command", DefaultCommand)
	v.SetDefault("post-command", DefaultPostCommand)

	// Environment variables: ORBIT_COMMAND, ORBIT_POST_COMMAND
	v.SetEnvPrefix("ORBIT")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	// Config file name (without extension)
	v.SetConfigName(".orbit")
	v.SetConfigType("yaml")

	// Load home config first (lowest priority for files)
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(homeDir, ".orbit.yaml")
		if _, statErr := os.Stat(homeConfigPath); statErr == nil {
			v.SetConfigFile(homeConfigPath)
			if err := v.ReadInConfig(); err != nil {
				log.Printf("Warning: could not read %s: %v", homeConfigPath, err)
			}
		}
	}

	// Track if post-command was explicitly set in project config
	postCommandExplicit := false

	// Load project config (higher priority, merges with home)
	projectConfigPath := filepath.Join(workingDir, ".orbit.yaml")
	if _, statErr := os.Stat(projectConfigPath); statErr == nil {
		projectViper := viper.New()
		projectViper.SetConfigFile(projectConfigPath)

		if err := projectViper.ReadInConfig(); err != nil {
			log.Printf("Warning: could not read %s: %v", projectConfigPath, err)
		} else {
			// Check if post-command was explicitly set in project config
			postCommandExplicit = projectViper.IsSet("post-command")

			// Merge project config into main viper
			if err := v.MergeConfigMap(projectViper.AllSettings()); err != nil {
				log.Printf("Warning: could not merge project config: %v", err)
			}
		}
	}

	// Resolve final values, handling env var overrides
	command := v.GetString("command")
	postCommand := v.GetString("post-command")

	// Check if environment variables are set (Viper doesn't handle empty string env vars well)
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
