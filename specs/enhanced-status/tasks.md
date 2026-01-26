---
references:
    - requirements.md
    - design.md
    - decision_log.md
---
# Enhanced Status Command

## Phase 1: Git Operations

- [x] 1. Add HasUncommittedChangesInPath method to Git struct
  - Implement using git status --porcelain -uno
  - Return true if any tracked files have changes
  - Return error for invalid path
  - Requirements: 2.1, 2.2, 2.3, 2.4, 2.6

- [x] 2. Add GetRecentCommits method to Git struct
  - Implement using git log with format %h%x00%s
  - Accept limit parameter for number of commits
  - Use baseCommit..HEAD range
  - Return Commit slice with Hash and Subject
  - Requirements: 1.1, 1.2, 1.3, 1.4

- [x] 3. Add unit tests for git methods
  - Test HasUncommittedChangesInPath with clean, staged, unstaged, and untracked scenarios
  - Test GetRecentCommits with varying commit counts and no commits case
  - Use real git operations in temp repo

## Phase 2: Transcript Reading

- [x] 4. Add GetLastDisplayableEntry function in internal/transcript
  - Read from end of file with expanding window (64KB to 4MB)
  - Re-stat file each iteration for concurrent write safety
  - Skip incomplete JSON lines and non-displayable entries
  - Return nil,nil for empty or no displayable entries
  - Requirements: 3.4, 3.10, 3.13

- [x] 5. Add FormatToolUse and FormatLastAction functions
  - Implement parameter priority order: file_path, path, command, pattern, query, url, prompt
  - Truncate tool input to 60 chars, text to 80 chars
  - Prioritize tool_use over text in FormatLastAction
  - Requirements: 3.5, 3.6, 3.7

- [x] 6. Add unit tests for transcript functions
  - Create fixture files for tool_use, text, mixed, incomplete, large entry scenarios
  - Test parameter extraction priority
  - Test truncation behavior

## Phase 3: Status Package

- [ ] 7. Create internal/status package with types
  - Create types.go with VariantInfo, GitInfo, LastActionResult, LastActionState enum, TaskProgress, PhaseProgress
  - Export all types for use by status command

- [ ] 8. Implement Gatherer struct and methods
  - Implement NewGatherer with git client, specName, baseCommit, repoRoot
  - Implement GatherAllVariants with concurrent goroutines
  - Implement GatherVariantInfo with graceful error handling
  - Implement gatherGitInfo, gatherLastAction, gatherTaskProgress helpers
  - Requirements: 6.1, 6.2, 6.3, 6.4, 6.5

- [ ] 9. Add GetLiveTranscriptPath function
  - Read session ID from summary.json CurrentPhase
  - Build path using claudecode.BuildProjectPath
  - Return empty string for non-Claude agents
  - Requirements: 3.1, 3.2, 3.3, 3.8, 3.9, 3.12

- [ ] 10. Add unit tests for status gatherer
  - Test with mocked git client and file system
  - Test concurrent gathering
  - Test graceful degradation when data sources fail

## Phase 4: Output Types and Rendering

- [ ] 11. Add output types for status rendering
  - Create StatusOutput, VariantOutput, CommitOutput, TaskOutput structs
  - Include JSON tags for serialization
  - Add buildStatusOutput and buildVariantOutput helper functions

- [ ] 12. Implement renderStatus with format support
  - Implement renderJSON using go-output WithObject
  - Implement renderTerminal using go-output Text
  - Add format parameter to choose output mode
  - Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 5.10

## Phase 5: Integration

- [ ] 13. Update cmd/orbit/status.go to use new components
  - Create Gatherer with loaded metadata
  - Call GatherAllVariants for concurrent data collection
  - Call renderStatus with collected data
  - Handle variants.json missing case
  - Requirements: 6.6

- [ ] 14. Add integration test for status command
  - Create temp git repo with worktree structure
  - Create variants.json, summary.json, transcript fixtures
  - Verify output contains all expected sections
