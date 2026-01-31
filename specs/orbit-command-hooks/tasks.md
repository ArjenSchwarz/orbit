---
references:
    - specs/orbit-command-hooks/requirements.md
    - specs/orbit-command-hooks/design.md
    - specs/orbit-command-hooks/decision_log.md
---
# Orbit Command Hooks

## Configuration Layer

- [x] 1. Add new fields to Config struct <!-- id:yvhgqlo -->
  - Add PrePrompt, PostPrompt (renamed from PostCommand), CommandTimeout fields to internal/config/config.go
  - Add tracking fields for explicit setting detection
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [2.1](requirements.md#2.1), [7.6](requirements.md#7.6)

- [x] 2. Add PreCommand and PostCommand to AgentAliasConfig <!-- id:yvhgqlp -->
  - Add pre-command and post-command string fields to AgentAliasConfig struct
  - Add yaml tags for configuration parsing
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [4.1](requirements.md#4.1)

- [x] 3. Implement config loading for new fields <!-- id:yvhgqlq -->
  - Load pre-prompt from config, env (ORBIT_PRE_PROMPT), CLI flag
  - Load post-prompt from config, env (ORBIT_POST_PROMPT), CLI flag
  - Load command-timeout from config, env (ORBIT_COMMAND_TIMEOUT)
  - Handle empty string as explicit disable vs not set
  - Blocked-by: yvhgqlo (Add new fields to Config struct), yvhgqlp (Add PreCommand and PostCommand to AgentAliasConfig)
  - Stream: 1
  - Requirements: [1.3](requirements.md#1.3), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [7.7](requirements.md#7.7), [7.8](requirements.md#7.8)

- [x] 4. Write unit tests for config loading <!-- id:yvhgqlr -->
  - TestLoadPrePrompt, TestLoadPostPrompt, TestLoadCommandTimeout
  - TestLoadAgentPreCommand, TestLoadAgentPostCommand
  - TestIsPrePromptDisabled, TestIsPostPromptDisabled
  - TestEmptyCommandTreatedAsNoOp
  - Blocked-by: yvhgqlq (Implement config loading for new fields)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [3.1](requirements.md#3.1), [3.8](requirements.md#3.8), [4.1](requirements.md#4.1), [4.8](requirements.md#4.8), [7.6](requirements.md#7.6), [7.7](requirements.md#7.7), [7.8](requirements.md#7.8)

## Deprecation Detection

- [ ] 5. Implement CheckDeprecation function <!-- id:yvhgqls -->
  - Create config.CheckDeprecation(workingDir) function
  - Check for ORBIT_POST_COMMAND environment variable
  - Check for top-level post-command key in .orbit.yaml files
  - Distinguish top-level post-command (error) from agent-level (allowed)
  - Blocked-by: yvhgqlo (Add new fields to Config struct)
  - Stream: 1
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6)

- [ ] 6. Add CLI flag deprecation check in run.go <!-- id:yvhgqlt -->
  - Check for --post-command flag before flag parsing
  - Exit with clear error message if deprecated flag found
  - Stream: 1
  - Requirements: [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)

- [ ] 7. Write unit tests for deprecation detection <!-- id:yvhgqlu -->
  - TestCheckDeprecation_TopLevelPostCommand
  - TestCheckDeprecation_EnvVar
  - TestCheckDeprecation_AllowsAgentLevelPostCommand
  - Blocked-by: yvhgqls (Implement CheckDeprecation function), yvhgqlt (Add CLI flag deprecation check in run.go)
  - Stream: 1
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.6](requirements.md#5.6)

## CLI Layer

- [ ] 8. Update CLI flags in run.go <!-- id:yvhgqlv -->
  - Add --pre-prompt and --no-pre-prompt flags
  - Rename --post-command to --post-prompt and add --no-post-prompt
  - Integrate deprecation check before flag parsing
  - Blocked-by: yvhgqls (Implement CheckDeprecation function), yvhgqlt (Add CLI flag deprecation check in run.go)
  - Stream: 1
  - Requirements: [1.2](requirements.md#1.2), [1.4](requirements.md#1.4), [2.2](requirements.md#2.2), [2.4](requirements.md#2.4)

- [ ] 9. Update Orbit Config struct and pass new fields <!-- id:yvhgqlw -->
  - Add PrePrompt, PostPrompt (rename PostCommand), AgentPreCommand, AgentPostCommand, CommandTimeout to orbit.Config
  - Pass fields from CLI to orbit.New()
  - Blocked-by: yvhgqlq (Implement config loading for new fields), yvhgqlv (Update CLI flags in run.go)
  - Stream: 1
  - Requirements: [2.1](requirements.md#2.1), [3.1](requirements.md#3.1), [4.1](requirements.md#4.1), [7.6](requirements.md#7.6)

## Log Manager Updates

- [ ] 10. Add PrePromptState and ShellCommandState to Summary <!-- id:yvhgqlx -->
  - Add PrePromptState struct with SessionID, StartedAt, CompletedAt, Status fields
  - Add ShellCommandState struct with Command, ExitCode, StartedAt, CompletedAt, DurationMS
  - Add PrePrompt, PreCommand, PostCommand fields to Summary struct
  - Stream: 2
  - Requirements: [2.11](requirements.md#2.11), [8.1](requirements.md#8.1)

- [ ] 11. Implement pre-prompt tracking methods <!-- id:yvhgqly -->
  - Implement StartPrePrompt(continueSession) method
  - Implement CompletePrePrompt(sessionID) method
  - Implement GetPrePromptState() returning sessionID and status
  - Blocked-by: yvhgqlx (Add PrePromptState and ShellCommandState to Summary)
  - Stream: 2
  - Requirements: [2.11](requirements.md#2.11), [2.12](requirements.md#2.12)

- [ ] 12. Implement RecordShellCommand method <!-- id:yvhgqlz -->
  - Record pre-command and post-command execution in summary.json
  - Track command, exit_code, started_at, completed_at, duration_ms
  - Blocked-by: yvhgqlx (Add PrePromptState and ShellCommandState to Summary)
  - Stream: 2
  - Requirements: [8.1](requirements.md#8.1)

- [ ] 13. Write unit tests for log manager updates <!-- id:yvhgqm0 -->
  - TestStartPrePrompt, TestCompletePrePrompt, TestGetPrePromptState
  - TestRecordShellCommand
  - TestPreCommandLogFile, TestPostCommandLogFile, TestLogFileFormat
  - Blocked-by: yvhgqly (Implement pre-prompt tracking methods), yvhgqlz (Implement RecordShellCommand method)
  - Stream: 2
  - Requirements: [2.11](requirements.md#2.11), [2.12](requirements.md#2.12), [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)

## Shell Command Execution

- [ ] 14. Create shell.go with ShellCommandResult and executeShellCommand <!-- id:yvhgqm1 -->
  - Create internal/orbit/shell.go
  - Define ShellCommandResult struct
  - Implement executeShellCommand with timeout via context.WithTimeout
  - Set working directory to repository root
  - Set ORBIT_PHASE_COUNT and ORBIT_AGENT environment variables
  - Capture stdout and stderr
  - Blocked-by: yvhgqlw (Update Orbit Config struct and pass new fields)
  - Stream: 1
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [7.4](requirements.md#7.4), [7.5](requirements.md#7.5), [7.6](requirements.md#7.6), [7.9](requirements.md#7.9), [7.10](requirements.md#7.10)

- [ ] 15. Implement saveShellCommandLog function <!-- id:yvhgqm2 -->
  - Save command output to .orbit/pre-command-run-N.txt or post-command-run-N.txt
  - Include command, exit code, timestamps, duration, stdout, stderr
  - Blocked-by: yvhgqm1 (Create shell.go with ShellCommandResult and executeShellCommand)
  - Stream: 1
  - Requirements: [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)

- [ ] 16. Write unit tests for shell command execution <!-- id:yvhgqm3 -->
  - TestExecuteShellCommand_Success
  - TestExecuteShellCommand_NonZeroExit
  - TestExecuteShellCommand_Timeout
  - TestExecuteShellCommand_WorkingDir
  - TestExecuteShellCommand_EnvVars
  - TestExecuteShellCommand_CapturesOutput
  - Blocked-by: yvhgqm1 (Create shell.go with ShellCommandResult and executeShellCommand), yvhgqm2 (Implement saveShellCommandLog function)
  - Stream: 1
  - Requirements: [7.1](requirements.md#7.1), [7.2](requirements.md#7.2), [7.4](requirements.md#7.4), [7.5](requirements.md#7.5), [7.9](requirements.md#7.9), [7.10](requirements.md#7.10)

## Single-Run Mode Hooks

- [ ] 17. Implement runAgentPreCommand method <!-- id:yvhgqm4 -->
  - Execute agent pre-command if configured
  - Abort run on non-zero exit code
  - Handle dry-run mode by printing command without executing
  - Log to pre-command-run-N.txt
  - Blocked-by: yvhgqm1 (Create shell.go with ShellCommandResult and executeShellCommand)
  - Stream: 1
  - Requirements: [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [7.12](requirements.md#7.12)

- [ ] 18. Implement runPrePrompt method <!-- id:yvhgqm5 -->
  - Execute global pre-prompt if configured
  - Start new agent session and store session_id
  - Check for completed pre-prompt state on resume
  - Abort run on failure
  - Blocked-by: yvhgqly (Implement pre-prompt tracking methods), yvhgqm4 (Implement runAgentPreCommand method)
  - Stream: 1
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.9](requirements.md#2.9), [2.12](requirements.md#2.12), [2.14](requirements.md#2.14)

- [ ] 19. Modify runPhase to use pre-prompt session for phase 1 <!-- id:yvhgqm6 -->
  - Pass prePromptSessionID to StartPhase for phase 1
  - Handle SessionInvalid error by starting fresh session
  - Blocked-by: yvhgqm5 (Implement runPrePrompt method)
  - Stream: 1
  - Requirements: [2.7](requirements.md#2.7), [2.10](requirements.md#2.10)

- [ ] 20. Implement runAgentPostCommand method <!-- id:yvhgqm7 -->
  - Execute agent post-command if configured
  - Log warning on failure but complete run
  - Handle dry-run mode by printing command without executing
  - Blocked-by: yvhgqm1 (Create shell.go with ShellCommandResult and executeShellCommand)
  - Stream: 1
  - Requirements: [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.5](requirements.md#4.5), [7.12](requirements.md#7.12)

- [ ] 21. Update runSingle to call hooks in order <!-- id:yvhgqm8 -->
  - Call runAgentPreCommand before runPrePrompt
  - Call runPrePrompt before displayPhaseOverview
  - Call runAgentPostCommand in complete() after post-prompt
  - Blocked-by: yvhgqm4 (Implement runAgentPreCommand method), yvhgqm5 (Implement runPrePrompt method), yvhgqm7 (Implement runAgentPostCommand method)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2)

- [ ] 22. Rename PostCommand to PostPrompt in complete method <!-- id:yvhgqm9 -->
  - Rename config.PostCommand references to config.PostPrompt
  - Update log messages from post-command to post-prompt
  - Blocked-by: yvhgqm8 (Update runSingle to call hooks in order)
  - Stream: 1
  - Requirements: [1.5](requirements.md#1.5), [1.6](requirements.md#1.6)

- [ ] 23. Add StartPrePrompt spinner method <!-- id:yvhgqma -->
  - Add StartPrePrompt method to internal/display/spinner.go
  - Display Running pre-prompt message
  - Stream: 2
  - Requirements: [2.5](requirements.md#2.5)

- [ ] 24. Write unit tests for single-run hooks <!-- id:yvhgqmb -->
  - TestExecutionOrder, TestExecutionOrder_SkipsUnconfigured
  - TestPrePromptSessionPassedToPhase1, TestPrePromptResume
  - TestPrePromptFailureAbortsRun, TestPrePromptInvalidSessionFallback
  - TestAgentPreCommandFailureAbortsRun, TestAgentPostCommandFailureWarns
  - TestDryRunPrintsCommands
  - Blocked-by: yvhgqm8 (Update runSingle to call hooks in order), yvhgqm9 (Rename PostCommand to PostPrompt in complete method)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [2.7](requirements.md#2.7), [2.9](requirements.md#2.9), [2.10](requirements.md#2.10), [2.12](requirements.md#2.12), [3.5](requirements.md#3.5), [4.5](requirements.md#4.5), [7.12](requirements.md#7.12)

## Variant Mode Hooks

- [ ] 25. Implement executeVariantShellCommand method <!-- id:yvhgqmc -->
  - Execute shell command in variant worktree
  - Set ORBIT_VARIANT environment variable
  - Get phase count from variant rune client
  - Save logs to variant-specific directory
  - Blocked-by: yvhgqm1 (Create shell.go with ShellCommandResult and executeShellCommand)
  - Stream: 1
  - Requirements: [6.4](requirements.md#6.4), [7.2](requirements.md#7.2), [8.5](requirements.md#8.5)

- [ ] 26. Implement runVariantPrePrompt method <!-- id:yvhgqmd -->
  - Execute pre-prompt in variant worktree
  - Track state in variant log manager
  - Return session ID for phase 1 continuation
  - Blocked-by: yvhgqm5 (Implement runPrePrompt method), yvhgqmc (Implement executeVariantShellCommand method)
  - Stream: 1
  - Requirements: [2.7](requirements.md#2.7), [6.4](requirements.md#6.4)

- [ ] 27. Update runVariant to integrate hooks <!-- id:yvhgqme -->
  - Call executeVariantShellCommand for agent pre-command
  - Call runVariantPrePrompt before phase loop
  - Modify phase 1 to use pre-prompt session
  - Rename PostCommand to PostPrompt in variant completion
  - Call executeVariantShellCommand for agent post-command after post-prompt
  - Blocked-by: yvhgqmc (Implement executeVariantShellCommand method), yvhgqmd (Implement runVariantPrePrompt method)
  - Stream: 1
  - Requirements: [6.4](requirements.md#6.4), [6.5](requirements.md#6.5)

- [ ] 28. Write integration tests for variant mode hooks <!-- id:yvhgqmf -->
  - TestVariantModeWithHooks
  - TestVariantPreCommandFailureIsolated
  - TestVariantPrePromptSessionContinuity
  - TestVariantHooksInParallel
  - TestVariantLogStructure
  - TestVariantEnvVars
  - TestVariantDifferentAgentCommands
  - Blocked-by: yvhgqme (Update runVariant to integrate hooks)
  - Stream: 1
  - Requirements: [6.4](requirements.md#6.4), [6.5](requirements.md#6.5), [8.5](requirements.md#8.5)

## Integration and Documentation

- [ ] 29. Update index generation for shell command status <!-- id:yvhgqmg -->
  - Include pre-command and post-command status in run index
  - Update index.md and index.html generation
  - Blocked-by: yvhgqlz (Implement RecordShellCommand method)
  - Stream: 2
  - Requirements: [8.6](requirements.md#8.6)

- [ ] 30. Write integration tests for full run with hooks <!-- id:yvhgqmh -->
  - TestFullRunWithAllHooks
  - TestDeprecationBlocksRun
  - TestResumeWithCompletedPrePrompt
  - TestResumeWithStartedPrePrompt
  - TestCommandTimeoutConfigurable
  - TestSignalDuringShellCommand
  - TestSignalDuringPrePrompt
  - Blocked-by: yvhgqmb (Write unit tests for single-run hooks), yvhgqmf (Write integration tests for variant mode hooks)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1), [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [2.12](requirements.md#2.12), [7.6](requirements.md#7.6), [7.9](requirements.md#7.9), [2.9](requirements.md#2.9)

- [ ] 31. Update CLAUDE.md with new configuration options <!-- id:yvhgqmi -->
  - Document pre-prompt and post-prompt configuration
  - Document agent-level pre-command and post-command
  - Document command-timeout configuration
  - Explain execution order and difference between commands and prompts
  - Blocked-by: yvhgqm8 (Update runSingle to call hooks in order), yvhgqme (Update runVariant to integrate hooks)
  - Stream: 2
  - Requirements: [10.1](requirements.md#10.1), [10.2](requirements.md#10.2), [10.3](requirements.md#10.3), [10.4](requirements.md#10.4), [10.5](requirements.md#10.5)
