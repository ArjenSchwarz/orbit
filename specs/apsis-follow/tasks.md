---
references:
    - specs/apsis-follow/requirements.md
    - specs/apsis-follow/design.md
    - specs/apsis-follow/decision_log.md
---
# Apsis Follow Mode Implementation

## Core Infrastructure

- [x] 1. Add hashLine function and lineWithHash type
  - Implement SHA-256 truncated to 16 bytes
  - Add lineWithHash struct with raw bytes and hash fields
  - Write unit tests first (table-driven)
  - Requirements: [4.4](requirements.md#4.4), [4.5](requirements.md#4.5)
  - References: internal/transcript/follow.go

- [x] 2. Add getFileInfo function for mtime/inode/size
  - Implement Unix inode access via syscall.Stat_t
  - Add fallback for non-Unix platforms (inode=0)
  - Write unit tests with temp files
  - Requirements: [3.3](requirements.md#3.3), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7)
  - References: internal/transcript/follow.go

- [x] 3. Add readAndHashLines function
  - Read file line by line with bufio.Scanner
  - Hash raw bytes before parsing
  - Handle incomplete JSON at EOF (skip silently)
  - Log warning for corrupt mid-file lines
  - Write unit tests for normal, incomplete EOF, and corrupt mid-file cases
  - Requirements: [7.5](requirements.md#7.5), [7.6](requirements.md#7.6)
  - References: internal/transcript/follow.go

## Incremental Rendering

- [ ] 4. Add RenderEntries function to markdown.go
  - Extract entry rendering logic from RenderMarkdown
  - Accept pre-built toolMeta and skillDescriptions
  - Render without header
  - Write unit tests comparing output with RenderMarkdown minus header
  - Requirements: [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.7](requirements.md#4.7)
  - References: internal/transcript/markdown.go

- [ ] 5. Export buildToolMeta and buildSkillDescriptionMap
  - Rename to exported functions
  - Update RenderMarkdown to use exported versions
  - Ensure existing tests still pass
  - Requirements: [4.7](requirements.md#4.7)
  - References: internal/transcript/markdown.go

## Follower Component

- [ ] 6. Implement Follower struct and NewFollower
  - Add struct with all fields per design
  - NewFollower validates file exists (requirement 7.1)
  - Initialize seenHashes map
  - Add maxSeenHashes constant (10000)
  - Write unit tests for constructor validation
  - Requirements: [7.1](requirements.md#7.1)
  - References: internal/transcript/follow.go

- [ ] 7. Implement Follower.poll method
  - Check mtime for changes
  - Detect truncation via size decrease
  - Detect replacement via inode change
  - Clear seenHashes on truncation/replacement
  - Write unit tests for each detection scenario
  - Requirements: [3.3](requirements.md#3.3), [3.4](requirements.md#3.4), [3.5](requirements.md#3.5), [3.6](requirements.md#3.6), [3.7](requirements.md#3.7)
  - References: internal/transcript/follow.go

- [ ] 8. Implement Follower.processFile method
  - Call readAndHashLines
  - Parse entries and build toolMeta/skillDescriptions
  - Filter to unseen entries via hash comparison
  - Render with RenderMarkdown (initial) or RenderEntries (subsequent)
  - Update seenHashes with addSeenHash (respects cap)
  - Write unit tests for initial render, incremental render, and cap reset
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.6](requirements.md#4.6), [4.8](requirements.md#4.8)
  - References: internal/transcript/follow.go

- [ ] 9. Implement Follower.Run method
  - Create 500ms ticker
  - Poll loop with context cancellation
  - Call processFile on changes
  - Return nil on clean shutdown
  - Write integration test with goroutine and context cancellation
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [6.1](requirements.md#6.1), [6.2](requirements.md#6.2)
  - References: internal/transcript/follow.go

## CLI Integration

- [ ] 10. Add Follow flag to Config struct
  - Add -F and --follow flags
  - Update parseFlags function
  - Update printUsage with follow mode documentation
  - Write unit tests for flag parsing
  - Requirements: [1.1](requirements.md#1.1), [1.2](requirements.md#1.2), [1.4](requirements.md#1.4)
  - References: cmd/apsis/main.go

- [ ] 11. Add validateFollowMode function
  - Check for -o with --follow conflict
  - Check for -f html with --follow conflict
  - Return appropriate error messages per requirements
  - Write unit tests for each validation case
  - Requirements: [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.5](requirements.md#5.5), [5.6](requirements.md#5.6), [5.7](requirements.md#5.7)
  - References: cmd/apsis/main.go

- [ ] 12. Add resolveFollowInput function
  - Resolve session ID to file path
  - Resolve file path directly
  - Return error for stdin input
  - Write unit tests for each input type
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4)
  - References: cmd/apsis/main.go

- [ ] 13. Add runFollow function with signal handling
  - Use signal.NotifyContext for SIGINT
  - Create Follower and call Run
  - Exit with code 130 on SIGINT
  - Write integration test for signal handling
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)
  - References: cmd/apsis/main.go

- [ ] 14. Integrate follow mode into run function
  - Call validateFollowMode early
  - Branch to runFollow when Follow is true
  - Ensure non-follow path unchanged
  - Run existing tests to verify no regression
  - Requirements: [1.3](requirements.md#1.3)
  - References: cmd/apsis/main.go

## Final Validation

- [ ] 15. Add end-to-end integration tests
  - Test basic follow with entry append
  - Test file truncation and re-render
  - Test file replacement and re-render
  - Test SIGINT termination with exit code 130
  - Use waitForOutput helper with generous timeouts
  - Requirements: [3.5](requirements.md#3.5), [3.7](requirements.md#3.7), [6.3](requirements.md#6.3)
  - References: internal/transcript/follow_test.go, cmd/apsis/main_test.go

- [ ] 16. Run linter and fix any issues
  - Run make lint
  - Fix any golangci-lint warnings
  - Ensure code follows project conventions
  - References: Makefile
