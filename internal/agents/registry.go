package agents

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	registry = make(map[string]func(AgentConfig) Agent)
	mu       sync.RWMutex
)

// AgentConfig holds per-agent configuration from .orbit.yaml.
type AgentConfig struct {
	CLIPath     string            // Override CLI command path
	AutoApprove bool              // Tool approval behavior
	ExtraArgs   []string          // Additional CLI arguments
	Timeout     time.Duration     // Execution timeout
	Options     map[string]string // Agent-specific options
}

// Register adds an agent factory to the registry.
func Register(name string, factory func(AgentConfig) Agent) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// Get returns an agent by name with configuration.
func Get(name string, cfg AgentConfig) (Agent, error) {
	mu.RLock()
	factory, ok := registry[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown agent: %s (available: %v)", name, List())
	}
	return factory(cfg), nil
}

// List returns all registered agent names sorted alphabetically.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Default returns the default agent name.
func Default() string {
	return "claude-code"
}
