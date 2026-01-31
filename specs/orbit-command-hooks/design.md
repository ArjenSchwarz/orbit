# Design: Orbit Command Hooks

## Overview

This design document describes the implementation of command hooks and prompt renaming for Orbit. The feature introduces:

1. **Prompt Renaming**: Rename `post-command` to `post-prompt` to clarify it's an AI prompt
2. **Global Pre-Prompt**: A new AI prompt that runs before phases and shares its session with phase 1
3. **Agent-Level Commands**: Shell commands (`pre-command`, `post-command`) that run before/after agent execution
4. **Migration Enforcement**: Detection and rejection of deprecated configuration

### Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Orbit Run Start                          │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              1. Deprecation Check (fatal if found)              │
│  - Check for top-level post-command in .orbit.yaml              │
│  - Check for ORBIT_POST_COMMAND env var                         │
│  - Check for --post-command CLI flag                            │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              2. Agent Pre-Command (shell)                        │
│  - Execute: /bin/sh -c "<command>"                              │
│  - Working dir: repository root                                 │
│  - Timeout: configurable (default 5m)                           │
│  - Failure: abort run                                           │
│  - Log output to: .orbit/pre-command-run-N.txt                  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              3. Global Pre-Prompt (AI)                           │
│  - Start new agent session                                      │
│  - Execute prompt via agent.Run()                               │
│  - Store session_id for phase 1 continuation                    │
│  - Failure: abort run                                           │
│  - Track in summary.json: pre_prompt state                      │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              4. Phase Loop (existing logic)                      │
│  - Phase 1: Resume pre-prompt session (if exists)               │
│  - Phase 2..N: Continue or fresh session per config             │
│  - Each phase: agent.Run() or agent.Resume()                    │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              5. Global Post-Prompt (AI)                          │
│  - Execute prompt via agent (existing post-command logic)       │
│  - Retry logic: up to 5 attempts with exponential backoff       │
│  - Failure after retries: complete with warnings                │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              6. Agent Post-Command (shell)                       │
│  - Execute: /bin/sh -c "<command>"                              │
│  - Working dir: repository root                                 │
│  - Timeout: configurable (default 5m)                           │
│  - Failure: complete with warnings                              │
│  - Log output to: .orbit/post-command-run-N.txt                 │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Run Complete                              │
└─────────────────────────────────────────────────────────────────┘
```

---

## Critical Implementation Details

This section addresses specific implementation questions that require concrete answers.

### Q1: How does runSingle() call hooks in order?

The existing `runSingle()` method must be modified to integrate hooks. Here is the concrete modification:

```go
// runSingle executes a single orchestration run (non-variant mode).
func (o *Orbit) runSingle() error {
    // Setup shutdown handler (existing)
    o.setupShutdownHandler()

    // === NEW: Step 1 - Agent Pre-Command (shell) ===
    if err := o.runAgentPreCommand(); err != nil {
        return o.fail(err)
    }

    // === NEW: Step 2 - Pre-Prompt (AI) ===
    if err := o.runPrePrompt(); err != nil {
        return o.fail(err)
    }

    // Display phase overview (existing) - this populates phaseSummaries
    if err := o.displayPhaseOverview(); err != nil {
        return o.fail(err)
    }

    // Check for pending tasks (existing)
    pending, err := o.runeClient.GetPendingTasks()
    if err != nil {
        return o.fail(err)
    }

    if len(pending) == 0 {
        log.Println("No pending tasks found - orchestration complete")
        return o.complete()
    }

    // Phase loop (existing, but modified for pre-prompt session)
    for {
        nextPhase, err := o.runeClient.GetNextPhase()
        if err != nil {
            return o.fail(err)
        }
        if nextPhase == nil {
            break // All phases complete
        }

        phaseNum := o.getPhaseNumber(nextPhase.PhaseName)

        if o.config.DryRun {
            // ... existing dry-run logic ...
            return nil
        }

        log.Printf("Starting phase %d: %s", phaseNum, nextPhase.PhaseName)

        // Run the phase (modified to use pre-prompt session for phase 1)
        if err := o.runPhaseWithRetry(phaseNum); err != nil {
            return o.fail(err)
        }
    }

    // === Step 3 - complete() handles post-prompt and post-command ===
    return o.complete()
}

// complete handles successful orchestration completion.
// Modified to include agent post-command after post-prompt.
func (o *Orbit) complete() error {
    // Run post-prompt if configured (renamed from post-command)
    if o.config.PostPrompt != "" {
        log.Println("Running post-prompt...")
        if err := o.runPostPromptWithRetry(); err != nil {
            log.Printf("Post-prompt failed: %v", err)
            // Don't fail the run, just warn
        }
    }

    // === NEW: Agent Post-Command (shell) ===
    if err := o.runAgentPostCommand(); err != nil {
        // Post-command failure is warning only, already logged
    }

    // Existing completion logic...
    if o.logManager != nil {
        if err := o.logManager.Complete(); err != nil {
            return err
        }
    }
    return nil
}
```

### Q2: How does phase 1 use prePromptSessionID?

The `logManager.StartPhase()` method must be modified to accept an optional override session ID. Here's the concrete change:

```go
// In internal/logs/manager.go - modify StartPhase signature
func (m *Manager) StartPhase(phase int, continueSession bool, overrideSessionID string) (string, bool, error) {
    // If an override session ID is provided (from pre-prompt), use it for phase 1
    if overrideSessionID != "" && phase == 1 {
        // Record that we're using the pre-prompt session
        m.summary.CurrentPhase = &PhaseState{
            Phase:     phase,
            SessionID: overrideSessionID,
            StartedAt: time.Now(),
        }
        if err := m.writeSummary(); err != nil {
            return "", false, err
        }
        // Return the override session ID, with isResume=true since we're continuing
        return overrideSessionID, true, nil
    }

    // Existing logic for phases without override...
    // ...
}

// In internal/orbit/orbit.go - modify runPhase to pass pre-prompt session
func (o *Orbit) runPhase(phase int) error {
    startTime := time.Now()

    // Determine if phase 1 should use pre-prompt session
    var overrideSessionID string
    if phase == 1 && o.prePromptSessionID != "" {
        overrideSessionID = o.prePromptSessionID
        o.debug.Log("Phase 1 will continue pre-prompt session: %s", overrideSessionID)
    }

    // Get session ID from log manager with optional override
    sessionID, isResume, err := o.logManager.StartPhase(phase, o.config.ContinueSession, overrideSessionID)
    if err != nil {
        return err
    }

    // Rest of existing runPhase logic...
}
```

### Q3: CheckDeprecation timing with workingDir

The deprecation check happens in two stages:

1. **CLI flag check** - happens before flag parsing, doesn't need workingDir
2. **Config/env check** - happens after workingDir is known

```go
func runCommand(args []string) error {
    // Stage 1: Check CLI flags FIRST (no workingDir needed)
    for _, arg := range args {
        if arg == "--post-command" || strings.HasPrefix(arg, "--post-command=") {
            return fmt.Errorf("flag --post-command is deprecated.\n\n"+
                "  Rename to: --post-prompt")
        }
    }

    // Stage 2: Check environment variable (no workingDir needed)
    if _, exists := os.LookupEnv("ORBIT_POST_COMMAND"); exists {
        return fmt.Errorf("environment variable ORBIT_POST_COMMAND is deprecated.\n\n"+
            "  Rename to: ORBIT_POST_PROMPT")
    }

    // Now parse flags
    fs := flag.NewFlagSet("run", flag.ExitOnError)
    // ... flag definitions ...
    if err := fs.Parse(args); err != nil {
        return err
    }

    // Get working directory
    workingDir, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("failed to get working directory: %w", err)
    }

    // Stage 3: Check config files (requires workingDir)
    if err := config.CheckDeprecatedConfigFiles(workingDir); err != nil {
        return err
    }

    // Continue with normal loading...
}
```

The `config.CheckDeprecatedConfigFiles()` function only checks YAML files:

```go
// CheckDeprecatedConfigFiles checks .orbit.yaml files for deprecated keys.
// This is separate from environment variable checks.
func CheckDeprecatedConfigFiles(workingDir string) error {
    var errors []string

    // Check home config
    if homeDir, err := os.UserHomeDir(); err == nil {
        homeConfigPath := filepath.Join(homeDir, ".orbit.yaml")
        if hasDeprecatedTopLevelKey(homeConfigPath, "post-command") {
            errors = append(errors,
                fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
                    "  Rename to: 'post-prompt'", homeConfigPath))
        }
    }

    // Check project config
    projectConfigPath := filepath.Join(workingDir, ".orbit.yaml")
    if hasDeprecatedTopLevelKey(projectConfigPath, "post-command") {
        errors = append(errors,
            fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
                "  Rename to: 'post-prompt'", projectConfigPath))
    }

    if len(errors) > 0 {
        return fmt.Errorf("deprecated configuration detected:\n\n%s",
            strings.Join(errors, "\n\n"))
    }
    return nil
}
```

### Q4: Agent-level commands extraction from AgentAliasConfig

The `run.go` file must extract pre/post commands when building the orbit config:

```go
// In cmd/orbit/run.go - after resolving the agent alias

// Resolve agent alias
resolved, err := cfg.GetResolvedAgent(aliasName)
if err != nil {
    return err
}

// Build agent config (existing)
agentCfg := buildAgentConfig(resolved)

// === NEW: Extract agent-level commands ===
agentPreCommand := resolved.Config.PreCommand
agentPostCommand := resolved.Config.PostCommand

// Build orbit config
orbitConfig := orbit.Config{
    // ... existing fields ...
    PostPrompt:       postPrompt,  // Renamed from PostCommand
    PrePrompt:        prePrompt,   // New
    AgentPreCommand:  agentPreCommand,   // New: from agent alias config
    AgentPostCommand: agentPostCommand,  // New: from agent alias config
    CommandTimeout:   commandTimeout,    // New: from config
    // ...
}
```

For variant mode with different agents, each variant gets its own commands from its assigned agent:

```go
// In variant execution loop
for _, variant := range variants {
    // Get the agent for this variant
    variantAgent := getVariantAgent(variant.ID, o.config.VariantAgents)

    // Resolve the agent's config
    resolved, _ := cfg.GetResolvedAgent(variantAgent)

    // Each variant uses its own agent's commands
    variantOrbitConfig := orbit.Config{
        // ...
        AgentPreCommand:  resolved.Config.PreCommand,
        AgentPostCommand: resolved.Config.PostCommand,
    }
}
```

### Q5: Pre-prompt state tracking (never started vs started but crashed)

The `PrePromptState` struct and methods need to distinguish three states:

```go
// PrePromptState tracks pre-prompt execution for crash recovery.
type PrePromptState struct {
    SessionID   string     `json:"session_id"`
    StartedAt   time.Time  `json:"started_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    // Status explicitly tracks the state
    Status      string     `json:"status"` // "started", "completed"
}

// PrePromptStatus constants
const (
    PrePromptStatusNotStarted = ""          // nil PrePrompt in summary
    PrePromptStatusStarted    = "started"   // Started but not completed
    PrePromptStatusCompleted  = "completed" // Successfully completed
)

// GetPrePromptState returns the pre-prompt state for resumption decisions.
// Returns: sessionID, status (one of PrePromptStatus* constants)
func (m *Manager) GetPrePromptState() (sessionID string, status string) {
    if m.summary.PrePrompt == nil {
        return "", PrePromptStatusNotStarted
    }
    return m.summary.PrePrompt.SessionID, m.summary.PrePrompt.Status
}

// StartPrePrompt begins pre-prompt tracking.
func (m *Manager) StartPrePrompt(continueSession bool) (string, bool, error) {
    // Check for existing in-progress pre-prompt
    if m.summary.PrePrompt != nil {
        if m.summary.PrePrompt.Status == PrePromptStatusStarted && continueSession {
            // Resume existing session that was interrupted
            return m.summary.PrePrompt.SessionID, true, nil
        }
        if m.summary.PrePrompt.Status == PrePromptStatusCompleted {
            // Already completed - caller should have checked GetPrePromptState first
            return m.summary.PrePrompt.SessionID, false, nil
        }
        // Status is "started" but not continuing - clear and start fresh
        m.summary.PrePrompt = nil
    }

    // Generate new session ID
    sessionID := uuid.NewString()

    m.summary.PrePrompt = &PrePromptState{
        SessionID: sessionID,
        StartedAt: time.Now(),
        Status:    PrePromptStatusStarted,
    }

    if err := m.writeSummary(); err != nil {
        return "", false, err
    }

    return sessionID, false, nil
}

// CompletePrePrompt marks pre-prompt as completed.
func (m *Manager) CompletePrePrompt(sessionID string) error {
    if m.summary.PrePrompt == nil {
        return nil
    }
    now := time.Now()
    m.summary.PrePrompt.CompletedAt = &now
    m.summary.PrePrompt.Status = PrePromptStatusCompleted
    m.summary.PrePrompt.SessionID = sessionID // Update in case agent returned different ID
    return m.writeSummary()
}
```

Updated `runPrePrompt()` to use the new state tracking:

```go
func (o *Orbit) runPrePrompt() error {
    if o.config.PrePrompt == "" {
        return nil
    }

    // Check pre-prompt state for resumption
    if o.logManager != nil {
        sessionID, status := o.logManager.GetPrePromptState()
        switch status {
        case logs.PrePromptStatusCompleted:
            // Already completed in previous run - use stored session
            o.prePromptSessionID = sessionID
            o.debug.Log("Pre-prompt already completed, using session: %s", sessionID)
            return nil
        case logs.PrePromptStatusStarted:
            // Started but crashed - will attempt resume below
            o.debug.Log("Pre-prompt was interrupted, will resume session: %s", sessionID)
        // case PrePromptStatusNotStarted: fall through to start fresh
        }
    }

    // ... rest of execution logic ...
}
```

### Q6: spinner.StartPrePrompt() definition

Add new method to `internal/display/spinner.go`:

```go
// StartPrePrompt starts the spinner for pre-prompt execution.
func (s *Spinner) StartPrePrompt() {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.message = "Running pre-prompt"
    s.phase = 0  // Use 0 to indicate pre-prompt (before phases)
    s.start()
}

// The existing StartPhase, StartPostCompletion methods remain unchanged
```

### Q7: phaseSummaries population timing

The `phaseSummaries` field is populated by `displayPhaseOverview()` which is called AFTER pre-command and pre-prompt. This means `ORBIT_PHASE_COUNT` will be 0 during pre-command execution.

**Resolution**: Get phase count directly from rune client at execution time:

```go
func (o *Orbit) executeShellCommand(command, logName string) (*ShellCommandResult, error) {
    // ...

    // Get phase count directly from rune client, not from cached phaseSummaries
    phaseCount := 0
    if summaries, err := o.runeClient.GetPhaseSummaries(); err == nil {
        phaseCount = len(summaries)
    }

    cmd.Env = append(os.Environ(),
        fmt.Sprintf("ORBIT_PHASE_COUNT=%d", phaseCount),
        fmt.Sprintf("ORBIT_AGENT=%s", o.agent.Name()),
    )

    // ...
}
```

---

## Variant Mode Execution

Requirements 6.4 and 6.5 specify that variant mode must execute hooks independently per variant. This section details the variant-specific hook integration.

### Variant Mode Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    Variant Run Start                            │
│  (For each variant, in parallel or sequential)                  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              1. Variant Agent Pre-Command (shell)               │
│  - Execute: /bin/sh -c "<command>"                              │
│  - Working dir: variant worktree root                           │
│  - Command from: variantAgentConfig.PreCommand                  │
│  - Timeout: configurable (default 5m)                           │
│  - Failure: mark variant failed, continue others                │
│  - Log output to: .orbit/logs/variant-N/pre-command-run-M.txt   │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              2. Variant Pre-Prompt (AI)                         │
│  - Only if global pre-prompt is configured                      │
│  - Start new agent session in variant worktree                  │
│  - Store session_id for phase 1 continuation                    │
│  - Failure: mark variant failed, continue others                │
│  - Track in variant's summary.json                              │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              3. Phase Loop (existing logic)                     │
│  - Phase 1: Resume pre-prompt session (if exists)               │
│  - Phase 2..N: Continue or fresh session per config             │
│  - Each phase uses variant-specific agent and worktree          │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              4. Variant Post-Prompt (AI)                        │
│  - Execute global post-prompt in variant's worktree             │
│  - Retry logic: up to 5 attempts with exponential backoff       │
│  - Failure after retries: variant completed with warnings       │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              5. Variant Agent Post-Command (shell)              │
│  - Execute: /bin/sh -c "<command>"                              │
│  - Working dir: variant worktree root                           │
│  - Command from: variantAgentConfig.PostCommand                 │
│  - Timeout: configurable (default 5m)                           │
│  - Failure: variant completed with warnings                     │
│  - Log output to: .orbit/logs/variant-N/post-command-run-M.txt  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Variant Complete                             │
└─────────────────────────────────────────────────────────────────┘
```

### Variant Mode Implementation

The `runVariant()` method must be modified to integrate hooks:

```go
func (o *Orbit) runVariant(ctx context.Context, v *variants.Variant) error {
    startTime := time.Now()
    log.Printf("Starting variant %d (branch: %s, agent: %s)", v.ID, v.Branch, v.Agent)

    // ... existing logger, agent, and rune client setup ...

    // Get agent config which includes pre-command and post-command
    variantAgentConfig := o.getAgentConfig(v.Agent)

    // === NEW: Step 1 - Variant Agent Pre-Command (shell) ===
    if variantAgentConfig.PreCommand != "" {
        if o.config.DryRun {
            log.Printf("[DRY RUN] Variant %d would execute pre-command: %s", v.ID, variantAgentConfig.PreCommand)
            log.Printf("[DRY RUN] Working directory: %s", v.WorktreePath)
        } else {
            result, err := o.executeVariantShellCommand(ctx, v, variantAgentConfig.PreCommand, "pre-command", variantLogManager)
            if err != nil {
                variantErr := fmt.Errorf("pre-command failed (exit code %d): %w", result.ExitCode, err)
                if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
                    log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
                }
                return variantErr
            }
        }
    }

    // === NEW: Step 2 - Variant Pre-Prompt (AI) ===
    var prePromptSessionID string
    if o.config.PrePrompt != "" {
        if o.config.DryRun {
            log.Printf("[DRY RUN] Variant %d would execute pre-prompt", v.ID)
        } else {
            sessionID, err := o.runVariantPrePrompt(ctx, v, variantAgent, variantLogManager)
            if err != nil {
                variantErr := fmt.Errorf("pre-prompt failed: %w", err)
                if updateErr := o.variantManager.UpdateStatus(v.ID, variants.StatusFailed, variantErr); updateErr != nil {
                    log.Printf("Warning: failed to update variant %d status: %v", v.ID, updateErr)
                }
                return variantErr
            }
            prePromptSessionID = sessionID
        }
    }

    // Phase loop (existing, modified to use prePromptSessionID for phase 1)
    for {
        // ... existing phase loop logic ...
        // Modified: if phaseNum == 1 && prePromptSessionID != "", use resume
    }

    // === Step 3 - Variant Post-Prompt (AI) (renamed from PostCommand) ===
    if o.config.PostPrompt != "" {
        log.Printf("Variant %d: running post-prompt...", v.ID)
        // ... existing post-completion logic with retry ...
    }

    // === NEW: Step 4 - Variant Agent Post-Command (shell) ===
    if variantAgentConfig.PostCommand != "" {
        if o.config.DryRun {
            log.Printf("[DRY RUN] Variant %d would execute post-command: %s", v.ID, variantAgentConfig.PostCommand)
            log.Printf("[DRY RUN] Working directory: %s", v.WorktreePath)
        } else {
            result, err := o.executeVariantShellCommand(ctx, v, variantAgentConfig.PostCommand, "post-command", variantLogManager)
            if err != nil {
                // Post-command failure is warning, not fatal
                log.Printf("Warning: variant %d post-command failed (exit code %d): %v", v.ID, result.ExitCode, err)
            }
        }
    }

    // Mark variant as completed
    // ... existing completion logic ...
}
```

### Variant Shell Command Execution

A new helper method executes shell commands in variant worktrees:

```go
// executeVariantShellCommand runs a shell command in a variant's worktree.
func (o *Orbit) executeVariantShellCommand(
    ctx context.Context,
    v *variants.Variant,
    command, logName string,
    logManager *logs.Manager,
) (*ShellCommandResult, error) {
    startTime := time.Now()
    result := &ShellCommandResult{
        Command:   command,
        StartedAt: startTime,
    }

    // Create context with timeout
    cmdCtx, cancel := context.WithTimeout(ctx, o.config.CommandTimeout)
    defer cancel()

    // Build command
    cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-c", command)
    cmd.Dir = v.WorktreePath  // Use variant worktree, not main repo

    // Get phase count from variant's rune client
    phaseCount := 0
    if summaries, err := rune.NewClient(o.getVariantTasksFile(v)).GetPhaseSummaries(); err == nil {
        phaseCount = len(summaries)
    }

    // Set up environment
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("ORBIT_PHASE_COUNT=%d", phaseCount),
        fmt.Sprintf("ORBIT_AGENT=%s", v.Agent),
        fmt.Sprintf("ORBIT_VARIANT=%d", v.ID),  // Additional env var for variants
    )

    // Capture output
    var stdout, stderr strings.Builder
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // Execute
    err := cmd.Run()
    result.CompletedAt = time.Now()
    result.Duration = result.CompletedAt.Sub(startTime)
    result.Stdout = stdout.String()
    result.Stderr = stderr.String()

    // Get exit code
    if exitErr, ok := err.(*exec.ExitError); ok {
        result.ExitCode = exitErr.ExitCode()
    } else if err != nil {
        result.ExitCode = -1
    }

    // Save log file to variant's log directory
    if logManager != nil {
        o.saveVariantShellCommandLog(result, logName, logManager)
        logManager.RecordShellCommand(logName, result.Command, result.ExitCode,
            result.StartedAt, result.CompletedAt, result.Duration)
    }

    if err != nil {
        return result, err
    }

    return result, nil
}
```

### Variant Pre-Prompt Execution

```go
// runVariantPrePrompt executes pre-prompt for a variant and returns the session ID.
func (o *Orbit) runVariantPrePrompt(
    ctx context.Context,
    v *variants.Variant,
    agent agents.Agent,
    logManager *logs.Manager,
) (string, error) {
    log.Printf("Variant %d: running pre-prompt...", v.ID)

    // Check for existing pre-prompt state (crash recovery)
    if logManager != nil {
        sessionID, status := logManager.GetPrePromptState()
        if status == logs.PrePromptStatusCompleted {
            log.Printf("Variant %d: pre-prompt already completed, using session: %s", v.ID, sessionID)
            return sessionID, nil
        }
    }

    // Start pre-prompt tracking
    var sessionID string
    var isResume bool
    if logManager != nil {
        var err error
        sessionID, isResume, err = logManager.StartPrePrompt(o.config.ContinueSession)
        if err != nil {
            return "", fmt.Errorf("start pre-prompt: %w", err)
        }
    } else {
        sessionID = uuid.NewString()
    }

    // Execute pre-prompt in variant's worktree
    opts := agents.RunOptions{
        Prompt:    o.config.PrePrompt,
        WorkDir:   v.WorktreePath,  // Variant worktree
        SessionID: sessionID,
    }

    var result *agents.RunResult
    var err error
    if isResume {
        result, err = agent.Resume(ctx, sessionID, opts)
    } else {
        result, err = agent.Run(ctx, opts)
    }

    if err != nil {
        return "", err
    }

    // Mark complete
    if logManager != nil {
        if err := logManager.CompletePrePrompt(result.SessionID); err != nil {
            log.Printf("Warning: variant %d failed to complete pre-prompt: %v", v.ID, err)
        }
    }

    log.Printf("Variant %d: pre-prompt completed", v.ID)
    return result.SessionID, nil
}
```

### Parallel Execution Considerations

Per Requirement 6.5, in parallel variant mode, pre-commands and post-commands MAY run concurrently:

1. **Isolation**: Each variant operates in its own isolated worktree, so there are no shared file conflicts
2. **Agent Isolation**: Each variant creates its own agent instance, so there are no shared session conflicts
3. **Logging Isolation**: Each variant has its own log directory (`specs/{spec}/.orbit/logs/variant-N/`)
4. **Environment**: Shell commands inherit the process environment plus variant-specific vars (`ORBIT_VARIANT`)

The existing `runVariantsParallel()` method with semaphore limiting already handles this correctly - each variant's hooks run within its own goroutine.

### Variant Mode Log Structure

Shell command logs for variants are stored in variant-specific directories:

```
specs/{spec}/.orbit/
├── logs/
│   ├── variant-1/
│   │   ├── summary.json           # Includes pre_prompt, pre_command, post_command state
│   │   ├── pre-command-run-1.txt  # Shell command output
│   │   ├── phase-1-run-1-session.json
│   │   ├── phase-1-run-1-session.txt
│   │   └── post-command-run-1.txt # Shell command output
│   ├── variant-2/
│   │   └── ...
│   └── variant-3/
│       └── ...
├── variants.json                   # Variant metadata
└── worktrees/
    ├── variant-1-my-spec/
    ├── variant-2-my-spec/
    └── variant-3-my-spec/
```

---

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                           cmd/orbit                              │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ run.go                                                       ││
│  │  - Parse CLI flags (--pre-prompt, --post-prompt, etc.)       ││
│  │  - Validate deprecation (--post-command error)               ││
│  │  - Call resolvePrompts() and resolveCommands()               ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                       internal/config                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ config.go                                                    ││
│  │  - Load pre-prompt, post-prompt, command-timeout             ││
│  │  - Parse agent pre-command/post-command in AgentAliasConfig  ││
│  │  - Check for deprecated post-command key                     ││
│  │  - CheckDeprecation() returns error if deprecated found      ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                        internal/orbit                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ orbit.go                                                     ││
│  │  - Config struct: PrePrompt, PostPrompt, AgentPreCommand...  ││
│  │  - runSingle(): execute hooks in order                       ││
│  │  - runPrePrompt(): new method for pre-prompt execution       ││
│  │  - runShellCommand(): new method for shell command execution ││
│  │  - Pass pre-prompt session_id to phase 1                     ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ shell.go (new file)                                          ││
│  │  - ShellCommandResult struct                                 ││
│  │  - ExecuteShellCommand() with timeout, env vars, logging     ││
│  │  - SaveShellCommandLog() to .orbit/ directory                ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         internal/logs                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ manager.go                                                   ││
│  │  - Summary struct: add PrePrompt, PreCommand, PostCommand    ││
│  │  - StartPrePrompt(), CompletePrePrompt() methods             ││
│  │  - RecordShellCommand() for pre/post command tracking        ││
│  │  - Update index generation for shell command status          ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

---

## Components and Interfaces

### 1. Configuration Layer (`internal/config/config.go`)

#### New Fields in Config struct

```go
type Config struct {
    // Existing fields...

    // Renamed field (was PostCommand)
    PostPrompt string  // AI prompt after phases complete

    // New fields
    PrePrompt       string        // AI prompt before phases start
    CommandTimeout  time.Duration // Timeout for shell commands (default 5m)

    // Tracking for explicit setting (for empty = disabled vs not set = default)
    prePromptExplicit  bool
    postPromptExplicit bool
}
```

#### New Fields in AgentAliasConfig struct

```go
type AgentAliasConfig struct {
    // Existing fields...

    // New fields for shell commands
    PreCommand  string `yaml:"pre-command"`   // Shell command before agent runs
    PostCommand string `yaml:"post-command"`  // Shell command after agent runs
}
```

#### New Functions

```go
// CheckDeprecation returns an error if deprecated configuration is found.
// Must be called before Load() processing to fail fast.
// Checks: top-level post-command in YAML, ORBIT_POST_COMMAND env var
func CheckDeprecation(workingDir string) error

// IsPrePromptDisabled returns true if pre-prompt was explicitly set to empty.
func (c *Config) IsPrePromptDisabled() bool

// IsPostPromptDisabled returns true if post-prompt was explicitly set to empty.
// (Renamed from IsPostCommandDisabled)
func (c *Config) IsPostPromptDisabled() bool
```

#### Deprecation Detection Logic

```go
func CheckDeprecation(workingDir string) error {
    var errors []string

    // Check environment variable
    if _, exists := os.LookupEnv("ORBIT_POST_COMMAND"); exists {
        errors = append(errors,
            "Environment variable ORBIT_POST_COMMAND is deprecated.\n"+
            "  Rename to: ORBIT_POST_PROMPT")
    }

    // Check home config
    if hasDeprecatedKey(homeConfigPath) {
        errors = append(errors,
            fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
            "  Rename to: 'post-prompt'", homeConfigPath))
    }

    // Check project config
    if hasDeprecatedKey(projectConfigPath) {
        errors = append(errors,
            fmt.Sprintf("Config file %s uses deprecated 'post-command' key.\n"+
            "  Rename to: 'post-prompt'", projectConfigPath))
    }

    if len(errors) > 0 {
        return fmt.Errorf("deprecated configuration detected:\n\n%s\n\n"+
            "Update your configuration and retry.", strings.Join(errors, "\n\n"))
    }
    return nil
}

// hasDeprecatedKey checks if a YAML file has a top-level post-command key
func hasDeprecatedKey(path string) bool {
    // Parse YAML and check for top-level "post-command" key
    // This distinguishes from agents.<name>.post-command which is valid
}
```

### 2. CLI Layer (`cmd/orbit/run.go`)

#### Updated Flag Definitions

```go
// Prompt flags (renamed from command terminology)
prePromptFlag := fs.String("pre-prompt", "", "AI prompt before phases start")
postPromptFlag := fs.String("post-prompt", "", "AI prompt after phases complete")
noPrePrompt := fs.Bool("no-pre-prompt", false, "Disable pre-prompt")
noPostPrompt := fs.Bool("no-post-prompt", false, "Disable post-prompt")

// Note: --post-command is NOT defined as a flag
// It will be caught by flag parsing and show standard "unknown flag" error
// which is acceptable since the deprecation check happens before flag parsing
```

#### Deprecation Check in runCommand

```go
func runCommand(args []string) error {
    // Check for deprecated --post-command flag before parsing
    for _, arg := range args {
        if arg == "--post-command" || strings.HasPrefix(arg, "--post-command=") {
            return fmt.Errorf("flag --post-command is deprecated.\n\n"+
                "  Rename to: --post-prompt\n\n"+
                "Update your command and retry.")
        }
    }

    // Check for deprecated config/env before loading
    if err := config.CheckDeprecation(workingDir); err != nil {
        return err
    }

    // Continue with normal flag parsing and config loading...
}
```

### 3. Orchestration Layer (`internal/orbit/orbit.go`)

#### Updated Config struct

```go
type Config struct {
    // Existing fields...

    // Renamed field
    PostPrompt string  // AI prompt after phases (was PostCommand)

    // New fields
    PrePrompt         string        // AI prompt before phases
    AgentPreCommand   string        // Shell command before agent
    AgentPostCommand  string        // Shell command after agent
    CommandTimeout    time.Duration // Timeout for shell commands
}
```

#### Updated Orbit struct

```go
type Orbit struct {
    // Existing fields...

    // New field for pre-prompt session tracking
    prePromptSessionID string  // Session ID from pre-prompt to pass to phase 1
}
```

#### New Methods

```go
// runPrePrompt executes the pre-prompt and stores the session ID for phase 1.
func (o *Orbit) runPrePrompt() error {
    if o.config.PrePrompt == "" {
        return nil  // No pre-prompt configured
    }

    startTime := time.Now()

    // Check if resuming interrupted run
    if o.logManager != nil {
        sessionID, completed := o.logManager.GetPrePromptState()
        if completed {
            // Pre-prompt already completed in previous run
            o.prePromptSessionID = sessionID
            return nil
        }
    }

    // Start spinner
    if o.spinner != nil {
        o.spinner.StartPrePrompt()
    }

    // Get or generate session ID
    var sessionID string
    var isResume bool
    if o.logManager != nil {
        var err error
        sessionID, isResume, err = o.logManager.StartPrePrompt(o.config.ContinueSession)
        if err != nil {
            return fmt.Errorf("failed to start pre-prompt: %w", err)
        }
    } else {
        sessionID = uuid.NewString()
    }

    // Execute pre-prompt
    opts := agents.RunOptions{
        Prompt:    o.config.PrePrompt,
        WorkDir:   o.config.WorkingDir,
        SessionID: sessionID,
    }

    var result *agents.RunResult
    var err error
    if isResume {
        result, err = o.agent.Resume(o.shutdownCtx, sessionID, opts)
    } else {
        result, err = o.agent.Run(o.shutdownCtx, opts)
    }

    if o.spinner != nil {
        o.spinner.Stop()
    }

    if err != nil {
        // Pre-prompt failure is fatal
        return fmt.Errorf("pre-prompt failed: %w", err)
    }

    // Store session ID for phase 1
    o.prePromptSessionID = result.SessionID

    // Mark complete in log manager
    if o.logManager != nil {
        if err := o.logManager.CompletePrePrompt(result.SessionID); err != nil {
            log.Printf("Warning: failed to complete pre-prompt: %v", err)
        }
    }

    return nil
}

// runAgentPreCommand executes the agent's pre-command shell script.
func (o *Orbit) runAgentPreCommand() error {
    if o.config.AgentPreCommand == "" {
        return nil  // No pre-command configured
    }

    if o.config.DryRun {
        log.Printf("[DRY RUN] Would execute pre-command: %s", o.config.AgentPreCommand)
        log.Printf("[DRY RUN] Working directory: %s", o.config.WorkingDir)
        return nil
    }

    result, err := o.executeShellCommand(o.config.AgentPreCommand, "pre-command")
    if err != nil {
        return fmt.Errorf("pre-command failed (exit code %d): %w", result.ExitCode, err)
    }

    return nil
}

// runAgentPostCommand executes the agent's post-command shell script.
func (o *Orbit) runAgentPostCommand() error {
    if o.config.AgentPostCommand == "" {
        return nil  // No post-command configured
    }

    if o.config.DryRun {
        log.Printf("[DRY RUN] Would execute post-command: %s", o.config.AgentPostCommand)
        log.Printf("[DRY RUN] Working directory: %s", o.config.WorkingDir)
        return nil
    }

    result, err := o.executeShellCommand(o.config.AgentPostCommand, "post-command")
    if err != nil {
        // Post-command failure is warning, not fatal
        log.Printf("Warning: post-command failed (exit code %d): %v", result.ExitCode, err)
        return nil  // Don't fail the run
    }

    return nil
}
```

### 4. Shell Command Execution (`internal/orbit/shell.go` - new file)

```go
package orbit

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

// ShellCommandResult holds the result of a shell command execution.
type ShellCommandResult struct {
    Command    string        // The command that was executed
    ExitCode   int           // Exit code (0 = success)
    Stdout     string        // Standard output
    Stderr     string        // Standard error
    Duration   time.Duration // Execution duration
    StartedAt  time.Time     // When the command started
    CompletedAt time.Time    // When the command completed
}

// executeShellCommand runs a shell command with timeout and environment setup.
func (o *Orbit) executeShellCommand(command, logName string) (*ShellCommandResult, error) {
    startTime := time.Now()
    result := &ShellCommandResult{
        Command:   command,
        StartedAt: startTime,
    }

    // Create context with timeout
    ctx, cancel := context.WithTimeout(o.shutdownCtx, o.config.CommandTimeout)
    defer cancel()

    // Build command
    cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
    cmd.Dir = o.config.WorkingDir

    // Set up environment
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("ORBIT_PHASE_COUNT=%d", len(o.phaseSummaries)),
        fmt.Sprintf("ORBIT_AGENT=%s", o.agent.Name()),
    )

    // Capture output
    var stdout, stderr strings.Builder
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // Execute
    err := cmd.Run()
    result.CompletedAt = time.Now()
    result.Duration = result.CompletedAt.Sub(startTime)
    result.Stdout = stdout.String()
    result.Stderr = stderr.String()

    // Get exit code
    if exitErr, ok := err.(*exec.ExitError); ok {
        result.ExitCode = exitErr.ExitCode()
    } else if err != nil {
        result.ExitCode = -1  // Command didn't start
    }

    // Save log file
    if o.logManager != nil {
        o.saveShellCommandLog(result, logName)
    }

    // Record in summary.json
    if o.logManager != nil {
        o.logManager.RecordShellCommand(logName, result.Command, result.ExitCode,
            result.StartedAt, result.CompletedAt, result.Duration)
    }

    if err != nil {
        return result, err
    }

    return result, nil
}

// saveShellCommandLog writes the command output to a log file.
func (o *Orbit) saveShellCommandLog(result *ShellCommandResult, logName string) {
    filename := fmt.Sprintf("%s-run-%d.txt", logName, o.logManager.RunNumber())
    path := filepath.Join(o.logManager.SessionDir(), filename)

    content := fmt.Sprintf(`Orbit Shell Command Log
========================================

Command: %s
Exit Code: %d
Started: %s
Completed: %s
Duration: %s

Stdout:
----------------------------------------
%s

Stderr:
----------------------------------------
%s
`,
        result.Command,
        result.ExitCode,
        result.StartedAt.Format(time.RFC3339),
        result.CompletedAt.Format(time.RFC3339),
        result.Duration.String(),
        result.Stdout,
        result.Stderr,
    )

    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        log.Printf("Warning: failed to save %s log: %v", logName, err)
    }
}
```

### 5. Log Manager Updates (`internal/logs/manager.go`)

#### Updated Summary struct

```go
type Summary struct {
    // Existing fields...

    // New fields for pre-prompt tracking
    PrePrompt *PrePromptState `json:"pre_prompt,omitempty"`

    // New fields for shell command tracking
    PreCommand  *ShellCommandState `json:"pre_command,omitempty"`
    PostCommand *ShellCommandState `json:"post_command,omitempty"`
}

// PrePromptState tracks pre-prompt execution for crash recovery.
type PrePromptState struct {
    SessionID   string     `json:"session_id"`
    StartedAt   time.Time  `json:"started_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ShellCommandState tracks shell command execution.
type ShellCommandState struct {
    Command     string     `json:"command"`
    ExitCode    int        `json:"exit_code"`
    StartedAt   time.Time  `json:"started_at"`
    CompletedAt time.Time  `json:"completed_at"`
    DurationMS  int64      `json:"duration_ms"`
}
```

#### New Methods

```go
// StartPrePrompt begins pre-prompt execution tracking.
func (m *Manager) StartPrePrompt(continueSession bool) (string, bool, error)

// CompletePrePrompt marks pre-prompt as completed with the session ID.
func (m *Manager) CompletePrePrompt(sessionID string) error

// GetPrePromptState returns the pre-prompt session ID and whether it completed.
func (m *Manager) GetPrePromptState() (sessionID string, completed bool)

// RecordShellCommand records a shell command execution in summary.json.
func (m *Manager) RecordShellCommand(name, command string, exitCode int,
    startedAt, completedAt time.Time, duration time.Duration) error
```

---

## Data Models

### Configuration File Schema

```yaml
# .orbit.yaml with new fields

# Global prompts (AI agent interactions)
pre-prompt: "Review the codebase structure before implementation."  # Optional
post-prompt: "Review the implementation and fix any issues."        # Has default

# Shell command timeout (default: 5m)
command-timeout: "15m"

# Agent configuration with shell commands
agents:
  claude-code:
    type: claude-code
    auto-approve: true
    pre-command: "make lint && make test-short"   # Optional shell command
    post-command: "make format && make lint"      # Optional shell command
```

### Summary.json Schema Updates

```json
{
  "started_at": "2025-01-31T10:00:00Z",
  "status": "running",
  "pre_prompt": {
    "session_id": "abc-123",
    "started_at": "2025-01-31T10:00:01Z",
    "completed_at": "2025-01-31T10:00:30Z"
  },
  "pre_command": {
    "command": "make lint && make test-short",
    "exit_code": 0,
    "started_at": "2025-01-31T10:00:00Z",
    "completed_at": "2025-01-31T10:00:15Z",
    "duration_ms": 15000
  },
  "sessions": [...],
  "post_completion": {...},
  "post_command": {
    "command": "make format",
    "exit_code": 0,
    "started_at": "2025-01-31T10:30:00Z",
    "completed_at": "2025-01-31T10:30:05Z",
    "duration_ms": 5000
  }
}
```

---

## Error Handling

### Deprecation Errors (Requirement 5)

| Condition | Error Message |
|-----------|---------------|
| `post-command` in YAML | `Config file {path} uses deprecated 'post-command' key. Rename to: 'post-prompt'` |
| `ORBIT_POST_COMMAND` env | `Environment variable ORBIT_POST_COMMAND is deprecated. Rename to: ORBIT_POST_PROMPT` |
| `--post-command` flag | `Flag --post-command is deprecated. Rename to: --post-prompt` |

### Shell Command Errors (Requirements 3, 4)

| Condition | Behavior |
|-----------|----------|
| Pre-command non-zero exit | Abort run with error including exit code and output |
| Pre-command timeout | Abort run with timeout error |
| Post-command non-zero exit | Log warning, complete run with warnings |
| Post-command timeout | Log warning, complete run with warnings |

### Prompt Errors (Requirements 2, 9)

| Condition | Behavior |
|-----------|----------|
| Pre-prompt failure | Abort run with error |
| Pre-prompt session invalid on resume | Start fresh session for phase 1, log warning |
| Post-prompt failure | Retry up to 5 times with exponential backoff |
| Post-prompt failure after retries | Complete with warnings |

---

## Signal Handling

Shell command execution must integrate with the existing graceful shutdown handling.

### Context Propagation

Shell commands use `context.WithTimeout` wrapping `o.shutdownCtx`. When SIGINT arrives:

1. The parent `shutdownCtx` is cancelled
2. The timeout context inherits the cancellation
3. `exec.CommandContext` terminates the shell process with SIGKILL

```go
func (o *Orbit) executeShellCommand(command, logName string) (*ShellCommandResult, error) {
    // Create context that respects both timeout AND shutdown
    ctx, cancel := context.WithTimeout(o.shutdownCtx, o.config.CommandTimeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
    // ...

    err := cmd.Run()

    // Check if this was a shutdown vs timeout
    if ctx.Err() == context.Canceled && o.shutdownCtx.Err() != nil {
        // Shutdown was requested
        return result, fmt.Errorf("command interrupted by shutdown")
    }
    if ctx.Err() == context.DeadlineExceeded {
        // Timeout occurred
        return result, fmt.Errorf("command timed out after %v", o.config.CommandTimeout)
    }

    // Normal execution (success or command error)
    // ...
}
```

### Pre-Prompt Shutdown

Pre-prompt execution follows the same pattern as existing phase execution - the agent's `Run()` or `Resume()` methods receive `o.shutdownCtx` and handle cancellation internally.

---

## Testing Strategy

### Unit Tests

#### Config Package (`internal/config/config_test.go`)

| Test | Requirement |
|------|-------------|
| `TestLoadPrePrompt` | 2.1, 2.3 |
| `TestLoadPostPrompt` | 1.1, 1.3 |
| `TestLoadCommandTimeout` | 7.6, 7.7, 7.8 |
| `TestLoadAgentPreCommand` | 3.1 |
| `TestLoadAgentPostCommand` | 4.1 |
| `TestCheckDeprecation_TopLevelPostCommand` | 5.1 |
| `TestCheckDeprecation_EnvVar` | 5.2 |
| `TestCheckDeprecation_AllowsAgentLevelPostCommand` | 5.6 |
| `TestIsPrePromptDisabled` | 2.4 |
| `TestIsPostPromptDisabled` | 1.4 |
| `TestEmptyCommandTreatedAsNoOp` | 3.8, 4.8 |

#### Orbit Package (`internal/orbit/orbit_test.go`)

| Test | Requirement |
|------|-------------|
| `TestExecutionOrder` | 6.1 |
| `TestExecutionOrder_SkipsUnconfigured` | 6.2 |
| `TestPrePromptSessionPassedToPhase1` | 2.7 |
| `TestPrePromptResume` | 2.12 |
| `TestPrePromptWithContinueSession` | 2.13 |
| `TestPrePromptFailureAbortsRun` | 2.9 |
| `TestPrePromptInvalidSessionFallback` | 2.10 |
| `TestAgentPreCommandFailureAbortsRun` | 3.5 |
| `TestAgentPostCommandFailureWarns` | 4.5 |
| `TestDryRunPrintsCommands` | 7.12 |

#### Shell Package (`internal/orbit/shell_test.go`)

| Test | Requirement |
|------|-------------|
| `TestExecuteShellCommand_Success` | 3.2, 4.2 |
| `TestExecuteShellCommand_NonZeroExit` | 3.5, 4.5 |
| `TestExecuteShellCommand_Timeout` | 7.9 |
| `TestExecuteShellCommand_WorkingDir` | 7.2 |
| `TestExecuteShellCommand_EnvVars` | 7.4, 7.5 |
| `TestExecuteShellCommand_CapturesOutput` | 7.10 |

#### Log Manager (`internal/logs/manager_test.go`)

| Test | Requirement |
|------|-------------|
| `TestStartPrePrompt` | 2.11 |
| `TestCompletePrePrompt` | 2.11 |
| `TestGetPrePromptState` | 2.12 |
| `TestRecordShellCommand` | 8.1 |
| `TestPreCommandLogFile` | 8.2 |
| `TestPostCommandLogFile` | 8.3 |
| `TestLogFileFormat` | 8.4 |

#### Display Package (`internal/display/spinner_test.go`)

| Test | Requirement |
|------|-------------|
| `TestSpinner_StartPrePrompt` | 2.5 (new spinner method) |

### Integration Tests

| Test | Requirements Covered |
|------|---------------------|
| `TestFullRunWithAllHooks` | 6.1, 6.2, 6.3 |
| `TestDeprecationBlocksRun` | 5.1, 5.2, 5.3, 5.4, 5.5 |
| `TestResumeWithCompletedPrePrompt` | 2.12 |
| `TestResumeWithStartedPrePrompt` | 2.12 (crash recovery) |
| `TestResumePreCommandCompletedPrePromptNotStarted` | 2.12 (edge case) |
| `TestCommandTimeoutConfigurable` | 7.6, 7.7, 7.8 |
| `TestSignalDuringShellCommand` | 7.9 (graceful shutdown) |
| `TestSignalDuringPrePrompt` | 2.9 (graceful shutdown) |

### Variant Mode Integration Tests

| Test | Requirements Covered |
|------|---------------------|
| `TestVariantModeWithHooks` | 6.4, 6.5, 8.5 |
| `TestVariantPreCommandFailureIsolated` | 6.4 (failure doesn't affect other variants) |
| `TestVariantPostCommandWarningIsolated` | 6.4, 4.5 |
| `TestVariantPrePromptSessionContinuity` | 2.7, 6.4 |
| `TestVariantHooksInParallel` | 6.5 (concurrent execution) |
| `TestVariantLogStructure` | 8.5 (logs in variant-specific directories) |
| `TestVariantEnvVars` | 7.4, 7.5 (ORBIT_VARIANT env var) |
| `TestVariantDifferentAgentCommands` | 6.4 (each variant uses its agent's commands) |
| `TestVariantResumeWithPrePrompt` | 2.12, 6.4 (crash recovery per variant) |

---

## Requirements Traceability

| Requirement | Design Element |
|-------------|----------------|
| 1.1-1.6 | Config.PostPrompt, CLI --post-prompt flag, ORBIT_POST_PROMPT env |
| 2.1-2.14 | Config.PrePrompt, Orbit.runPrePrompt(), logs.PrePromptState |
| 3.1-3.8 | AgentAliasConfig.PreCommand, Orbit.runAgentPreCommand() |
| 4.1-4.8 | AgentAliasConfig.PostCommand, Orbit.runAgentPostCommand() |
| 5.1-5.6 | config.CheckDeprecation(), CLI flag check in run.go |
| 6.1-6.3 | Orbit.runSingle() execution order |
| 6.4 | Orbit.runVariant() with hooks, executeVariantShellCommand(), runVariantPrePrompt() |
| 6.5 | runVariantsParallel() with per-variant hooks in isolated worktrees |
| 7.1-7.12 | Orbit.executeShellCommand(), shell.go, executeVariantShellCommand() |
| 8.1-8.4 | logs.RecordShellCommand(), saveShellCommandLog() |
| 8.5 | Variant log structure: specs/{spec}/.orbit/logs/variant-N/ |
| 8.6 | Index generation with pre-command/post-command status |
| 9.1-9.4 | Existing post-command retry logic, pre-prompt abort logic |
| 10.1-10.5 | CLAUDE.md updates |
