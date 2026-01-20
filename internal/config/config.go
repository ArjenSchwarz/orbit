// Package config handles configuration loading for Orbit using Viper.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/spf13/viper"
)

const (
	// DefaultCommand is the default prompt used for Claude during phase execution.
	DefaultCommand = "Run /next-task --phase and when complete run /commit"
	// DefaultPostCommand is the default prompt executed after all tasks complete.
	DefaultPostCommand = "Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."
	// DefaultServePort is the default port for the web server.
	DefaultServePort = 8080
	// DefaultServeBind is the default bind address for the web server.
	DefaultServeBind = "localhost"
	// DefaultMaxParallel is the default maximum number of parallel variants.
	DefaultMaxParallel = 3
	// DefaultBranchPrefix is the default prefix for variant branch names.
	DefaultBranchPrefix = "orbit-impl"
)

// AgentConfig holds per-agent settings from .orbit.yaml.
type AgentConfig struct {
	CLIPath     string   `yaml:"cli-path"`     // Override CLI command path
	AutoApprove bool     `yaml:"auto-approve"` // Tool approval behavior
	ExtraArgs   []string `yaml:"extra-args"`   // Additional CLI arguments
	Timeout     string   `yaml:"timeout"`      // Execution timeout as duration string (e.g., "30m", "1h")
	Model       string   `yaml:"model"`        // Agent-specific model option
}

// Config holds the resolved configuration values.
type Config struct {
	Command         string
	PostCommand     string
	DateSubdirs     bool
	ContinueSession bool
	ServePort       int
	ServeBind       string
	Debug           bool // Enable debug logging for troubleshooting

	// Agent selection and configuration
	Agent  string                 // Default agent for project (e.g., "claude-code", "codex")
	Agents map[string]AgentConfig // Per-agent configuration

	// Variant configuration for multi-spec comparison
	VariantCount   int    // Number of variants (0 = single-run mode)
	Parallel       bool   // Run variants in parallel
	MaxParallel    int    // Maximum parallel variants
	BranchPrefix   string // Branch naming prefix
	GuidanceFile   string // Path to YAML file with per-variant guidance
	CompareCommand string // Custom comparison command (empty = use Claude)
	GlobalGuidance string // Global guidance applied to all variants

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
	v.SetDefault("date-subdirs", false)
	v.SetDefault("continue-session", true)
	v.SetDefault("serve-port", DefaultServePort)
	v.SetDefault("serve-bind", DefaultServeBind)
	v.SetDefault("debug", false)
	// Variant defaults
	v.SetDefault("variant-count", 0)
	v.SetDefault("parallel", false)
	v.SetDefault("max-parallel", DefaultMaxParallel)
	v.SetDefault("branch-prefix", DefaultBranchPrefix)
	v.SetDefault("guidance-file", "")
	v.SetDefault("compare-command", "")
	v.SetDefault("global-guidance", "")

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
	dateSubdirs := v.GetBool("date-subdirs")
	continueSession := v.GetBool("continue-session")
	servePort := v.GetInt("serve-port")
	serveBind := v.GetString("serve-bind")
	debug := v.GetBool("debug")
	// Agent configuration
	agent := v.GetString("agent")
	agentsMap := parseAgentsConfig(v)
	// Variant configuration
	variantCount := v.GetInt("variant-count")
	parallel := v.GetBool("parallel")
	maxParallel := v.GetInt("max-parallel")
	branchPrefix := v.GetString("branch-prefix")
	guidanceFile := v.GetString("guidance-file")
	compareCommand := v.GetString("compare-command")
	globalGuidance := v.GetString("global-guidance")

	// Apply environment variable overrides (highest priority)
	// Using os.LookupEnv to detect both set values and explicitly empty values
	if envCmd, exists := os.LookupEnv("ORBIT_COMMAND"); exists {
		command = envCmd
	}
	if envPostCmd, exists := os.LookupEnv("ORBIT_POST_COMMAND"); exists {
		postCommand = envPostCmd
		postCommandExplicit = true
	}
	if envDateSubdirs, exists := os.LookupEnv("ORBIT_DATE_SUBDIRS"); exists {
		dateSubdirs = envDateSubdirs == "true" || envDateSubdirs == "1"
	}
	if envContinueSession, exists := os.LookupEnv("ORBIT_CONTINUE_SESSION"); exists {
		continueSession = envContinueSession == "true" || envContinueSession == "1"
	}
	if envServePort, exists := os.LookupEnv("ORBIT_SERVE_PORT"); exists {
		if port, err := parsePort(envServePort); err == nil {
			servePort = port
		}
		// Invalid port values are silently ignored (use config/default)
	}
	if envServeBind, exists := os.LookupEnv("ORBIT_SERVE_BIND"); exists {
		serveBind = envServeBind
	}
	if envDebug, exists := os.LookupEnv("ORBIT_DEBUG"); exists {
		debug = envDebug == "true" || envDebug == "1"
	}
	// Variant environment variable overrides
	if envVariantCount, exists := os.LookupEnv("ORBIT_VARIANT_COUNT"); exists {
		if count, err := parsePositiveInt(envVariantCount); err == nil {
			variantCount = count
		}
	}
	if envParallel, exists := os.LookupEnv("ORBIT_PARALLEL"); exists {
		parallel = envParallel == "true" || envParallel == "1"
	}
	if envMaxParallel, exists := os.LookupEnv("ORBIT_MAX_PARALLEL"); exists {
		if max, err := parsePositiveInt(envMaxParallel); err == nil {
			maxParallel = max
		}
	}
	if envBranchPrefix, exists := os.LookupEnv("ORBIT_BRANCH_PREFIX"); exists {
		branchPrefix = envBranchPrefix
	}
	if envGuidanceFile, exists := os.LookupEnv("ORBIT_GUIDANCE_FILE"); exists {
		guidanceFile = envGuidanceFile
	}
	if envCompareCommand, exists := os.LookupEnv("ORBIT_COMPARE_COMMAND"); exists {
		compareCommand = envCompareCommand
	}
	if envGlobalGuidance, exists := os.LookupEnv("ORBIT_GLOBAL_GUIDANCE"); exists {
		globalGuidance = envGlobalGuidance
	}
	// Agent environment variable override
	if envAgent, exists := os.LookupEnv("ORBIT_AGENT"); exists {
		agent = envAgent
	}

	return &Config{
		Command:             command,
		PostCommand:         postCommand,
		DateSubdirs:         dateSubdirs,
		ContinueSession:     continueSession,
		ServePort:           servePort,
		ServeBind:           serveBind,
		Debug:               debug,
		Agent:               agent,
		Agents:              agentsMap,
		VariantCount:        variantCount,
		Parallel:            parallel,
		MaxParallel:         maxParallel,
		BranchPrefix:        branchPrefix,
		GuidanceFile:        guidanceFile,
		CompareCommand:      compareCommand,
		GlobalGuidance:      globalGuidance,
		postCommandExplicit: postCommandExplicit,
	}
}

// parsePort attempts to parse a string as a valid port number.
func parsePort(s string) (int, error) {
	var port int
	_, err := fmt.Sscanf(s, "%d", &port)
	if err != nil {
		return 0, err
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}
	return port, nil
}

// parsePositiveInt attempts to parse a string as a positive integer.
func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("value must be non-negative: %d", n)
	}
	return n, nil
}

// IsPostCommandDisabled returns true if post-command was explicitly set to empty.
// This allows distinguishing "use default" from "disable".
func (c *Config) IsPostCommandDisabled() bool {
	return c.postCommandExplicit && c.PostCommand == ""
}

// parseAgentsConfig extracts the agents map from viper configuration.
// It handles the nested YAML structure of agent configurations.
func parseAgentsConfig(v *viper.Viper) map[string]AgentConfig {
	agentsMap := make(map[string]AgentConfig)

	// Get the agents section as a map
	agentsRaw := v.GetStringMap("agents")
	if agentsRaw == nil {
		return agentsMap
	}

	for name, cfg := range agentsRaw {
		// Each agent config is a map[string]interface{}
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			continue
		}

		// Default AutoApprove to true for non-interactive operation.
		// Can be explicitly set to false in config to disable.
		agentCfg := AgentConfig{
			AutoApprove: true,
		}

		if v, ok := cfgMap["cli-path"].(string); ok {
			agentCfg.CLIPath = v
		}
		if v, ok := cfgMap["auto-approve"].(bool); ok {
			agentCfg.AutoApprove = v
		}
		if v, ok := cfgMap["timeout"].(string); ok {
			agentCfg.Timeout = v
		}
		if v, ok := cfgMap["model"].(string); ok {
			agentCfg.Model = v
		}
		// Handle extra-args as a slice
		if v, ok := cfgMap["extra-args"].([]interface{}); ok {
			for _, arg := range v {
				if s, ok := arg.(string); ok {
					agentCfg.ExtraArgs = append(agentCfg.ExtraArgs, s)
				}
			}
		}

		agentsMap[name] = agentCfg
	}

	return agentsMap
}

// GetAgentConfig returns the agents.AgentConfig for a specific agent.
// If the agent is not configured, returns default AgentConfig with AutoApprove enabled.
// This method parses the timeout string into a time.Duration.
func (c *Config) GetAgentConfig(name string) agents.AgentConfig {
	// Default AutoApprove to true for non-interactive operation.
	// Orbit runs agents in automated mode, so tools should be approved automatically.
	cfg := agents.AgentConfig{
		AutoApprove: true,
	}

	ac, ok := c.Agents[name]
	if !ok {
		return cfg
	}

	cfg.CLIPath = ac.CLIPath
	cfg.AutoApprove = ac.AutoApprove
	cfg.ExtraArgs = ac.ExtraArgs

	// Parse timeout duration
	if ac.Timeout != "" {
		if d, err := time.ParseDuration(ac.Timeout); err == nil {
			cfg.Timeout = d
		}
	}

	// Store model in Options map
	if ac.Model != "" {
		cfg.Options = map[string]string{"model": ac.Model}
	}

	return cfg
}

// GetAllAgentConfigs returns a map of all agent configurations.
// Each config is converted to agents.AgentConfig format.
func (c *Config) GetAllAgentConfigs() map[string]agents.AgentConfig {
	result := make(map[string]agents.AgentConfig)
	for name := range c.Agents {
		result[name] = c.GetAgentConfig(name)
	}
	return result
}
