// Package config handles configuration loading for Orbit using Viper.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/arjenschwarz/orbit/internal/agents"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// aliasNamePattern matches valid alias names: lowercase alphanumeric with hyphens,
// not starting or ending with a hyphen.
// Pattern: [a-z0-9]+(-[a-z0-9]+)*
var aliasNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	// DefaultCommand is the default prompt used for Claude during phase execution.
	DefaultCommand = "Run /next-task --phase and when complete run /commit"
	// DefaultPostPrompt is the default prompt executed after all tasks complete.
	// Note: This was previously named DefaultPostCommand but renamed to clarify it's an AI prompt.
	DefaultPostPrompt = "Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."
	// DefaultServePort is the default port for the web server.
	DefaultServePort = 8080
	// DefaultServeBind is the default bind address for the web server.
	DefaultServeBind = "localhost"
	// DefaultMaxParallel is the default maximum number of parallel variants.
	DefaultMaxParallel = 3
	// DefaultBranchPrefix is the default prefix for variant branch names.
	DefaultBranchPrefix = "orbit-impl"
	// DefaultCommandTimeout is the default timeout for shell command execution.
	DefaultCommandTimeout = 5 * time.Minute
)

// AgentConfig holds per-agent settings from .orbit.yaml.
type AgentConfig struct {
	CLIPath     string   `yaml:"cli-path"`     // Override CLI command path
	AutoApprove bool     `yaml:"auto-approve"` // Tool approval behavior
	ExtraArgs   []string `yaml:"extra-args"`   // Additional CLI arguments
	Timeout     string   `yaml:"timeout"`      // Execution timeout as duration string (e.g., "30m", "1h")
	Model       string   `yaml:"model"`        // Agent-specific model option
}

// AgentAliasConfig holds per-alias settings from .orbit.yaml.
// This enables defining named agent configurations that combine an agent type with model and other settings.
type AgentAliasConfig struct {
	Type        string   `yaml:"type"`         // Required: underlying agent type (e.g., "claude-code", "codex")
	Model       string   `yaml:"model"`        // Optional: model to use
	CLIPath     string   `yaml:"cli-path"`     // Override CLI command path
	AutoApprove bool     `yaml:"auto-approve"` // Tool approval behavior
	ExtraArgs   []string `yaml:"extra-args"`   // Additional CLI arguments
	Timeout     string   `yaml:"timeout"`      // Execution timeout as duration string (e.g., "30m", "1h")
	PreCommand  string   `yaml:"pre-command"`  // Shell command to run before first phase
	PostCommand string   `yaml:"post-command"` // Shell command to run after last phase
}

// ResolvedAgent contains a validated and resolved agent alias.
type ResolvedAgent struct {
	Alias  string           // Original alias name (normalized to lowercase)
	Type   string           // Underlying agent type (e.g., "claude-code")
	Config AgentAliasConfig // The full configuration for this alias
}

// ValidateAliasName checks if a name matches the required pattern.
// Pattern: [a-z0-9]+(-[a-z0-9]+)* (lowercase alphanumeric with hyphens)
// Returns an error if the name is invalid.
func ValidateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}

	normalized := NormalizeAliasName(name)
	if !aliasNamePattern.MatchString(normalized) {
		return fmt.Errorf("invalid agent alias name %q: must use only lowercase letters, numbers, and hyphens, and cannot start or end with a hyphen (pattern: [a-z0-9]+(-[a-z0-9]+)*)", name)
	}
	return nil
}

// NormalizeAliasName converts a name to lowercase for case-insensitive comparison.
func NormalizeAliasName(name string) string {
	return strings.ToLower(name)
}

// Config holds the resolved configuration values.
type Config struct {
	Command         string
	PrePrompt       string        // AI prompt before phases start (empty = disabled)
	PostPrompt      string        // AI prompt after phases complete (renamed from PostCommand)
	CommandTimeout  time.Duration // Timeout for shell commands (default 5m)
	DateSubdirs     bool
	ContinueSession bool
	ServePort       int
	ServeBind       string
	Debug           bool // Enable debug logging for troubleshooting
	CentralizedLog  bool // Enable centralized file logging (default: true)

	// Agent selection and configuration
	Agent        string                      // Default agent alias for project
	Agents       map[string]AgentConfig      // Per-agent configuration (legacy)
	AgentAliases map[string]AgentAliasConfig // Agent alias configurations from YAML
	// ResolvedAgents is populated by ResolveAliases() after validation
	ResolvedAgents map[string]ResolvedAgent

	// Config file state
	ConfigFileFound   bool     // Whether .orbit.yaml was found (home or project)
	ConfigParseError  []error  // Errors from parsing config (e.g., invalid model types)
	ConfigSources     []string // List of config sources loaded (e.g., "home", "project", "env")

	// Variant configuration for multi-spec comparison
	VariantCount   int    // Number of variants (0 = single-run mode)
	Parallel       bool   // Run variants in parallel
	MaxParallel    int    // Maximum parallel variants
	BranchPrefix   string // Branch naming prefix
	GuidanceFile   string // Path to YAML file with per-variant guidance
	CompareCommand string // Custom comparison command (empty = use Claude)
	GlobalGuidance string // Global guidance applied to all variants

	// prePromptExplicit tracks whether pre-prompt was explicitly set in config.
	// This allows distinguishing "not set" (no pre-prompt) from "set to empty" (disabled).
	prePromptExplicit bool
	// postPromptExplicit tracks whether post-prompt was explicitly set in config.
	// This allows distinguishing "not set" (use default) from "set to empty" (disabled).
	postPromptExplicit bool
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
	v.SetDefault("pre-prompt", "")
	v.SetDefault("post-prompt", DefaultPostPrompt)
	v.SetDefault("command-timeout", DefaultCommandTimeout.String())
	v.SetDefault("date-subdirs", false)
	v.SetDefault("continue-session", true)
	v.SetDefault("serve-port", DefaultServePort)
	v.SetDefault("serve-bind", DefaultServeBind)
	v.SetDefault("debug", false)
	v.SetDefault("centralized-log", true)
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

	// Track if pre-prompt/post-prompt was explicitly set in either config file
	prePromptExplicit := false
	postPromptExplicit := false
	// Track if any config file was found (home or project)
	configFileFound := false
	// Track which config sources were loaded
	var configSources []string

	// Load home config first (lowest priority for files)
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(homeDir, ".orbit.yaml")
		if _, statErr := os.Stat(homeConfigPath); statErr == nil {
			homeViper := viper.New()
			homeViper.SetConfigFile(homeConfigPath)
			if err := homeViper.ReadInConfig(); err != nil {
				log.Printf("Warning: could not read %s: %v", homeConfigPath, err)
			} else {
				configFileFound = true
				configSources = append(configSources, "home")
				// Track if home config explicitly set pre-prompt or post-prompt
				if homeViper.IsSet("pre-prompt") {
					prePromptExplicit = true
				}
				if homeViper.IsSet("post-prompt") {
					postPromptExplicit = true
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
			configFileFound = true
			configSources = append(configSources, "project")
			// Check if pre-prompt or post-prompt was explicitly set in project config
			if projectViper.IsSet("pre-prompt") {
				prePromptExplicit = true
			}
			if projectViper.IsSet("post-prompt") {
				postPromptExplicit = true
			}

			// Merge project config into main viper
			if err := v.MergeConfigMap(projectViper.AllSettings()); err != nil {
				log.Printf("Warning: could not merge %s: %v", projectConfigPath, err)
			}
		}
	}

	// Get values from config files (with defaults as fallback)
	command := v.GetString("command")
	prePrompt := v.GetString("pre-prompt")
	postPrompt := v.GetString("post-prompt")
	commandTimeoutStr := v.GetString("command-timeout")
	commandTimeout := DefaultCommandTimeout
	if commandTimeoutStr != "" {
		if d, err := time.ParseDuration(commandTimeoutStr); err == nil {
			commandTimeout = d
		}
	}
	dateSubdirs := v.GetBool("date-subdirs")
	continueSession := v.GetBool("continue-session")
	servePort := v.GetInt("serve-port")
	serveBind := v.GetString("serve-bind")
	debug := v.GetBool("debug")
	centralizedLog := v.GetBool("centralized-log")
	// Agent configuration
	agent := v.GetString("agent")
	agentsMap, agentParseErrors := parseAgentsConfig(v)
	// Agent alias configuration (for new type-based agent system)
	agentAliasesMap, aliasParseErrors := parseAgentAliasesConfig(v)
	// Combine all parse errors
	var configParseErrors []error
	configParseErrors = append(configParseErrors, agentParseErrors...)
	configParseErrors = append(configParseErrors, aliasParseErrors...)
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
	envUsed := false
	if envCmd, exists := os.LookupEnv("ORBIT_COMMAND"); exists {
		command = envCmd
		envUsed = true
	}
	if envPrePrompt, exists := os.LookupEnv("ORBIT_PRE_PROMPT"); exists {
		prePrompt = envPrePrompt
		prePromptExplicit = true
		envUsed = true
	}
	if envPostPrompt, exists := os.LookupEnv("ORBIT_POST_PROMPT"); exists {
		postPrompt = envPostPrompt
		postPromptExplicit = true
		envUsed = true
	}
	if envCmdTimeout, exists := os.LookupEnv("ORBIT_COMMAND_TIMEOUT"); exists {
		if d, err := time.ParseDuration(envCmdTimeout); err == nil {
			commandTimeout = d
		}
		envUsed = true
	}
	if envDateSubdirs, exists := os.LookupEnv("ORBIT_DATE_SUBDIRS"); exists {
		dateSubdirs = envDateSubdirs == "true" || envDateSubdirs == "1"
		envUsed = true
	}
	if envContinueSession, exists := os.LookupEnv("ORBIT_CONTINUE_SESSION"); exists {
		continueSession = envContinueSession == "true" || envContinueSession == "1"
		envUsed = true
	}
	if envServePort, exists := os.LookupEnv("ORBIT_SERVE_PORT"); exists {
		if port, err := parsePort(envServePort); err == nil {
			servePort = port
			envUsed = true
		}
		// Invalid port values are silently ignored (use config/default)
	}
	if envServeBind, exists := os.LookupEnv("ORBIT_SERVE_BIND"); exists {
		serveBind = envServeBind
		envUsed = true
	}
	if envDebug, exists := os.LookupEnv("ORBIT_DEBUG"); exists {
		debug = envDebug == "true" || envDebug == "1"
		envUsed = true
	}
	if envCentralizedLog, exists := os.LookupEnv("ORBIT_CENTRALIZED_LOG"); exists {
		// Disable if explicitly set to "false" or "0", otherwise keep enabled
		centralizedLog = envCentralizedLog != "false" && envCentralizedLog != "0"
		envUsed = true
	}
	// Variant environment variable overrides
	if envVariantCount, exists := os.LookupEnv("ORBIT_VARIANT_COUNT"); exists {
		if count, err := parsePositiveInt(envVariantCount); err == nil {
			variantCount = count
			envUsed = true
		}
	}
	if envParallel, exists := os.LookupEnv("ORBIT_PARALLEL"); exists {
		parallel = envParallel == "true" || envParallel == "1"
		envUsed = true
	}
	if envMaxParallel, exists := os.LookupEnv("ORBIT_MAX_PARALLEL"); exists {
		if max, err := parsePositiveInt(envMaxParallel); err == nil {
			maxParallel = max
			envUsed = true
		}
	}
	if envBranchPrefix, exists := os.LookupEnv("ORBIT_BRANCH_PREFIX"); exists {
		branchPrefix = envBranchPrefix
		envUsed = true
	}
	if envGuidanceFile, exists := os.LookupEnv("ORBIT_GUIDANCE_FILE"); exists {
		guidanceFile = envGuidanceFile
		envUsed = true
	}
	if envCompareCommand, exists := os.LookupEnv("ORBIT_COMPARE_COMMAND"); exists {
		compareCommand = envCompareCommand
		envUsed = true
	}
	if envGlobalGuidance, exists := os.LookupEnv("ORBIT_GLOBAL_GUIDANCE"); exists {
		globalGuidance = envGlobalGuidance
		envUsed = true
	}
	// Agent environment variable override
	if envAgent, exists := os.LookupEnv("ORBIT_AGENT"); exists {
		agent = envAgent
		envUsed = true
	}

	// Add env source if any environment variables were used
	if envUsed {
		configSources = append(configSources, "env")
	}

	return &Config{
		Command:            command,
		PrePrompt:          prePrompt,
		PostPrompt:         postPrompt,
		CommandTimeout:     commandTimeout,
		DateSubdirs:        dateSubdirs,
		ContinueSession:    continueSession,
		ServePort:          servePort,
		ServeBind:          serveBind,
		Debug:              debug,
		CentralizedLog:     centralizedLog,
		Agent:              agent,
		Agents:             agentsMap,
		AgentAliases:       agentAliasesMap,
		ConfigFileFound:    configFileFound,
		ConfigParseError:   configParseErrors,
		ConfigSources:      configSources,
		VariantCount:       variantCount,
		Parallel:           parallel,
		MaxParallel:        maxParallel,
		BranchPrefix:       branchPrefix,
		GuidanceFile:       guidanceFile,
		CompareCommand:     compareCommand,
		GlobalGuidance:     globalGuidance,
		prePromptExplicit:  prePromptExplicit,
		postPromptExplicit: postPromptExplicit,
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

// IsPrePromptDisabled returns true if pre-prompt was explicitly set to empty.
// This allows distinguishing "not set" (no pre-prompt) from "explicitly disabled".
func (c *Config) IsPrePromptDisabled() bool {
	return c.prePromptExplicit && c.PrePrompt == ""
}

// IsPostPromptDisabled returns true if post-prompt was explicitly set to empty.
// This allows distinguishing "use default" from "disable".
func (c *Config) IsPostPromptDisabled() bool {
	return c.postPromptExplicit && c.PostPrompt == ""
}

// parseAgentsConfig extracts the agents map from viper configuration.
// It handles the nested YAML structure of agent configurations.
// Returns the parsed configs and a slice of validation errors for invalid model types.
func parseAgentsConfig(v *viper.Viper) (map[string]AgentConfig, []error) {
	agentsMap := make(map[string]AgentConfig)
	var validationErrors []error

	// Get the agents section as a map
	agentsRaw := v.GetStringMap("agents")
	if agentsRaw == nil {
		return agentsMap, nil
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
		// Handle model field with type coercion
		if modelVal, exists := cfgMap["model"]; exists {
			model, err := coerceModelValue(name, modelVal)
			if err != nil {
				validationErrors = append(validationErrors, err)
			} else {
				agentCfg.Model = model
			}
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

	return agentsMap, validationErrors
}

// parseAgentAliasesConfig extracts the agents map as AgentAliasConfig from viper configuration.
// It handles the nested YAML structure of agent alias configurations including the type field.
// Returns the parsed configs and a slice of validation errors for invalid model types.
func parseAgentAliasesConfig(v *viper.Viper) (map[string]AgentAliasConfig, []error) {
	aliasesMap := make(map[string]AgentAliasConfig)
	var validationErrors []error

	// Get the agents section as a map
	agentsRaw := v.GetStringMap("agents")
	if agentsRaw == nil {
		return aliasesMap, nil
	}

	for name, cfg := range agentsRaw {
		// Each agent config is a map[string]interface{}
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			continue
		}

		// Default AutoApprove to true for non-interactive operation.
		// Can be explicitly set to false in config to disable.
		aliasCfg := AgentAliasConfig{
			AutoApprove: true,
		}

		// Parse type field (required for alias configs)
		if v, ok := cfgMap["type"].(string); ok {
			aliasCfg.Type = v
		}
		if v, ok := cfgMap["cli-path"].(string); ok {
			aliasCfg.CLIPath = v
		}
		if v, ok := cfgMap["auto-approve"].(bool); ok {
			aliasCfg.AutoApprove = v
		}
		if v, ok := cfgMap["timeout"].(string); ok {
			aliasCfg.Timeout = v
		}
		// Handle model field with type coercion
		if modelVal, exists := cfgMap["model"]; exists {
			model, err := coerceModelValue(name, modelVal)
			if err != nil {
				validationErrors = append(validationErrors, err)
			} else {
				aliasCfg.Model = model
			}
		}
		// Handle extra-args as a slice
		if v, ok := cfgMap["extra-args"].([]interface{}); ok {
			for _, arg := range v {
				if s, ok := arg.(string); ok {
					aliasCfg.ExtraArgs = append(aliasCfg.ExtraArgs, s)
				}
			}
		}
		// Handle pre-command and post-command shell commands
		if v, ok := cfgMap["pre-command"].(string); ok {
			aliasCfg.PreCommand = v
		}
		if v, ok := cfgMap["post-command"].(string); ok {
			aliasCfg.PostCommand = v
		}

		aliasesMap[name] = aliasCfg
	}

	return aliasesMap, validationErrors
}

// coerceModelValue coerces a YAML model value to a string.
// Valid types: string, int, int64, float64 (coerced to string)
// Invalid types: bool, slice, map (returns error)
func coerceModelValue(aliasName string, value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case float64:
		// Use %v for cleaner output (e.g., "4.5" not "4.500000")
		return fmt.Sprintf("%v", v), nil
	case nil:
		return "", nil
	case bool:
		return "", fmt.Errorf("alias %q: model must be a string or number, got bool", aliasName)
	case []interface{}:
		return "", fmt.Errorf("alias %q: model must be a string or number, got array", aliasName)
	case map[string]interface{}:
		return "", fmt.Errorf("alias %q: model must be a string or number, got map", aliasName)
	default:
		return "", fmt.Errorf("alias %q: model must be a string or number, got %T", aliasName, v)
	}
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

	// Look up the underlying type from AgentAliases
	if alias, ok := c.AgentAliases[name]; ok && alias.Type != "" {
		cfg.Type = alias.Type
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

// ResolveAliases validates all agent aliases and builds the ResolvedAgents map.
// Returns error if validation fails: missing type, invalid name, duplicates, or unknown type.
// This should be called after Load() and before using any agent references.
func (c *Config) ResolveAliases() error {
	// Check for parse errors from config loading
	if len(c.ConfigParseError) > 0 {
		// Return the first parse error
		return c.ConfigParseError[0]
	}

	// Check if agents section is empty
	if len(c.AgentAliases) == 0 {
		return fmt.Errorf("no agents configured in .orbit.yaml\n\nAdd at least one agent:\n\nagents:\n  claude-code:\n    type: claude-code\n    auto-approve: true")
	}

	// Track normalized names for duplicate detection
	normalizedToOriginal := make(map[string]string)
	c.ResolvedAgents = make(map[string]ResolvedAgent)

	// Get registered agent types
	registeredTypes := agents.List()
	registeredTypesSet := make(map[string]bool)
	for _, t := range registeredTypes {
		registeredTypesSet[t] = true
	}

	for name, aliasCfg := range c.AgentAliases {
		// Validate alias name pattern
		if err := ValidateAliasName(name); err != nil {
			return err
		}

		// Normalize name for duplicate checking
		normalized := NormalizeAliasName(name)

		// Check for duplicates after normalization
		if existingName, exists := normalizedToOriginal[normalized]; exists {
			return fmt.Errorf("duplicate agent aliases after case normalization\n\nThe following aliases conflict:\n  - %q and %q both normalize to %q\n\nRemove duplicate definitions from .orbit.yaml", existingName, name, normalized)
		}
		normalizedToOriginal[normalized] = name

		// Validate type field is present
		if aliasCfg.Type == "" {
			return fmt.Errorf("agent alias %q is missing required \"type\" field\n\nAdd a type field to specify the underlying agent:\n\nagents:\n  %s:\n    type: claude-code  # or codex, kiro, copilot", name, name)
		}

		// Validate type is a registered agent
		if !registeredTypesSet[aliasCfg.Type] {
			return fmt.Errorf("unknown agent type %q for alias %q\n\nValid agent types: %v\n\nUpdate the type field in .orbit.yaml to use a registered agent type", aliasCfg.Type, name, registeredTypes)
		}

		// Build resolved agent
		c.ResolvedAgents[normalized] = ResolvedAgent{
			Alias:  normalized,
			Type:   aliasCfg.Type,
			Config: aliasCfg,
		}
	}

	return nil
}

// RequireConfigFile returns an error if no config file was found.
// This should be called before operations that require agent configuration.
func (c *Config) RequireConfigFile() error {
	if !c.ConfigFileFound {
		return fmt.Errorf("configuration file .orbit.yaml not found\n\nRun 'orbit init' to create a default configuration file")
	}
	return nil
}

// GetResolvedAgent returns the resolved agent for an alias.
// Returns error if alias is not found after normalization.
// ResolveAliases() must be called first.
func (c *Config) GetResolvedAgent(alias string) (ResolvedAgent, error) {
	if c.ResolvedAgents == nil {
		return ResolvedAgent{}, fmt.Errorf("ResolveAliases() must be called before GetResolvedAgent()")
	}

	normalized := NormalizeAliasName(alias)
	resolved, ok := c.ResolvedAgents[normalized]
	if !ok {
		// Build list of available agents
		available := make([]string, 0, len(c.ResolvedAgents))
		for name := range c.ResolvedAgents {
			available = append(available, name)
		}

		return ResolvedAgent{}, fmt.Errorf("agent %q is not configured in .orbit.yaml\n\nAvailable agents: %v\n\nTo add this agent:\n\nagents:\n  %s:\n    type: claude-code", alias, available, normalized)
	}

	return resolved, nil
}

// defaultConfigYAML is the template for a default .orbit.yaml file.
const defaultConfigYAML = `# Orbit configuration - see documentation for all options
agents:
  claude-code:
    type: claude-code
    auto-approve: true
`

// GenerateDefaultConfig returns the YAML bytes for a default .orbit.yaml file.
// The default config contains a single claude-code agent with type and auto-approve: true.
func GenerateDefaultConfig() []byte {
	return []byte(defaultConfigYAML)
}

// CheckDeprecation returns an error if deprecated configuration is found.
// It checks for:
// - ORBIT_POST_COMMAND environment variable
// - Top-level post-command key in .orbit.yaml files (home and project)
//
// This function distinguishes between deprecated top-level post-command (error)
// and valid agent-level post-command (allowed under agents.<name>.post-command).
//
// This should be called before Load() to fail fast on deprecated configuration.
func CheckDeprecation(workingDir string) error {
	var errors []string

	// Check environment variable
	if _, exists := os.LookupEnv("ORBIT_POST_COMMAND"); exists {
		errors = append(errors,
			"Environment variable ORBIT_POST_COMMAND is deprecated.\n"+
				"  Rename to: ORBIT_POST_PROMPT")
	}

	// Check home config for deprecated top-level post-command
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(homeDir, ".orbit.yaml")
		if hasDeprecatedTopLevelKey(homeConfigPath, "post-command") {
			errors = append(errors,
				fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
					"  Rename to: 'post-prompt'", homeConfigPath))
		}
	}

	// Check project config for deprecated top-level post-command
	projectConfigPath := filepath.Join(workingDir, ".orbit.yaml")
	if hasDeprecatedTopLevelKey(projectConfigPath, "post-command") {
		errors = append(errors,
			fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
				"  Rename to: 'post-prompt'", projectConfigPath))
	}

	if len(errors) > 0 {
		return fmt.Errorf("deprecated configuration detected:\n\n%s\n\n"+
			"Update your configuration and retry", strings.Join(errors, "\n\n"))
	}
	return nil
}

// hasDeprecatedTopLevelKey checks if a YAML file has a deprecated key at the top level.
// This distinguishes top-level post-command (deprecated) from agents.<name>.post-command (valid).
func hasDeprecatedTopLevelKey(path, key string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false // File doesn't exist or can't be read
	}

	// Parse YAML to check for top-level key
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return false // Invalid YAML, will be caught by config loading
	}

	// Check if the key exists at the top level
	_, exists := config[key]
	return exists
}

// GetResolvedAgentConfig converts a ResolvedAgent to agents.AgentConfig.
// This bridges between the config package's alias system and the agents package.
func GetResolvedAgentConfig(resolved ResolvedAgent) agents.AgentConfig {
	cfg := agents.AgentConfig{
		Type:        resolved.Type,
		CLIPath:     resolved.Config.CLIPath,
		AutoApprove: resolved.Config.AutoApprove,
		ExtraArgs:   resolved.Config.ExtraArgs,
		PreCommand:  resolved.Config.PreCommand,
		PostCommand: resolved.Config.PostCommand,
	}

	// Parse timeout duration
	if resolved.Config.Timeout != "" {
		if d, err := time.ParseDuration(resolved.Config.Timeout); err == nil {
			cfg.Timeout = d
		}
	}

	// Store model in Options map
	if resolved.Config.Model != "" {
		cfg.Options = map[string]string{"model": resolved.Config.Model}
	}

	return cfg
}
