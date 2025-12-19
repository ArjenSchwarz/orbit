---
references:
    - specs/custom-commands/requirements.md
    - specs/custom-commands/design.md
    - specs/custom-commands/decision_log.md
---
# Custom Commands Feature Tasks

## Setup

- [x] 1. Add Viper dependency to go.mod
  - Run: go get github.com/spf13/viper
  - Verify dependency is added to go.mod and go.sum

## Config Package

- [x] 2. Create internal/config package directory
  - Create internal/config/ directory structure

- [x] 3. Implement config.Load() using Viper
  - Create internal/config/config.go
  - Define Config struct with Command and PostCommand fields
  - Define DefaultCommand and DefaultPostCommand constants
  - Implement Load(workingDir string) *Config function
  - Configure Viper for YAML and env vars (ORBIT_COMMAND, ORBIT_POST_COMMAND)
  - Load from $HOME/.orbit.yaml first, then merge .orbit.yaml in cwd
  - Track postCommandExplicit for empty string detection
  - Log warnings for parse errors, continue with remaining sources
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)

- [x] 4. Implement IsPostCommandDisabled() helper
  - Add IsPostCommandDisabled() bool method to Config
  - Returns true when post-command was explicitly set to empty string
  - Requirements: [3.9](requirements.md#3.9), [4.3](requirements.md#4.3)

- [x] 5. Write tests for config package
  - Create internal/config/config_test.go
  - TestLoad_ProjectOnly - only project config exists
  - TestLoad_HomeOnly - only home config exists
  - TestLoad_MergesBoth - project overrides home
  - TestLoad_NoFiles - returns defaults
  - TestLoad_InvalidYAML - logs warning, continues
  - TestLoad_EmptyPostCommand - empty string disables
  - TestLoad_EnvVarOverride - env vars override config
  - TestIsPostCommandDisabled - correct behavior

## Claude Client Updates

- [x] 6. Add Prompt field to claude.Config
  - Update internal/claude/client.go
  - Add Prompt string field to Config struct

- [x] 7. Update RunPhase() to use configurable prompt
  - Modify RunPhase() to use c.config.Prompt instead of hardcoded string
  - Keep fallback to default if Prompt is empty
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4)

- [x] 8. Add tests for Claude client prompt handling
  - Update internal/claude/client_test.go
  - Test that custom Prompt is used when set
  - Test fallback to default when Prompt is empty

## Orbit Orchestrator Updates

- [x] 9. Add Command and PostCommand fields to orbit.Config
  - Update internal/orbit/orbit.go
  - Add Command string field for phase command
  - Add PostCommand string field for post-completion command

- [x] 10. Implement runPostCommand() method
  - Add runPostCommand() error method to Orbit
  - Use claudeClient.RunCustomPrompt() with PostCommand
  - Apply same error classification and retry logic as phases
  - Save session using logManager.SavePostCompletionSession()
  - Requirements: [3.3](requirements.md#3.3), [3.7](requirements.md#3.7)

- [x] 11. Update Run() to call post-command on completion
  - Modify Run() main loop exit points
  - Call runPostCommand() when PostCommand is not empty
  - Log start and completion of post-command
  - Skip post-command on orchestration failure
  - Requirements: [3.4](requirements.md#3.4), [3.6](requirements.md#3.6)

- [x] 12. Update dry-run output to show configured commands
  - Modify dry-run log output to show phase command
  - Show post-command or indicate if disabled
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2)

- [x] 13. Pass Command to claude.Config.Prompt
  - Update New() to set claudeClient config Prompt from orbit.Config.Command

- [x] 14. Add tests for orbit post-command execution
  - Update internal/orbit/orbit_test.go
  - Test runPostCommand() success and failure paths
  - Test post-command skipped when empty
  - Test post-command skipped on orchestration failure

## Log Manager Updates

- [x] 15. Implement SavePostCompletionSession() method
  - Update internal/logs/manager.go
  - Add SavePostCompletionSession(result, startTime) error method
  - Save to post-completion-session.json and post-completion-session.txt
  - Copy and format transcript from ~/.claude/projects
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2)

- [x] 16. Implement formatPostCompletionTranscript()
  - Add formatPostCompletionTranscript helper function
  - Header: Post-Completion Session Transcript
  - Include start time, duration, cost, turns, output
  - Requirements: [5.3](requirements.md#5.3)

- [x] 17. Add tests for post-completion logging
  - Update internal/logs/manager_test.go
  - Test SavePostCompletionSession creates correct files
  - Test formatPostCompletionTranscript output format

## CLI Integration

- [x] 18. Add --command flag to main.go
  - Update cmd/orbit/main.go
  - Add commandFlag := flag.String("command", "", "Custom prompt for Claude phases")
  - Requirements: [2.2](requirements.md#2.2)

- [x] 19. Add --post-command flag to main.go
  - Add postCommandFlag := flag.String("post-command", "", "Command after all tasks complete")
  - Requirements: [3.2](requirements.md#3.2)

- [x] 20. Add --no-post-command flag to main.go
  - Add noPostCommand := flag.Bool("no-post-command", false, "Skip post-completion command")
  - Requirements: [3.8](requirements.md#3.8)

- [x] 21. Integrate config.Load() in main.go
  - Import internal/config package
  - Call cfg := config.Load(workingDir) after determining working directory

- [x] 22. Implement priority resolution for command values
  - Resolve effective command: flag > config/env > default
  - Resolve effective post-command with IsPostCommandDisabled() check
  - Apply --no-post-command flag override
  - Pass resolved values to orbit.Config
  - Requirements: [4.1](requirements.md#4.1)

- [x] 23. Run linter and fix any issues
  - Run make lint
  - Fix any linting issues

- [x] 24. Run all tests and verify pass
  - Run make test
  - Ensure all tests pass
  - Run make test-coverage to verify coverage
