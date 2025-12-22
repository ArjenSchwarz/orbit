---
references:
    - specs/session-management/requirements.md
    - specs/session-management/design.md
    - specs/session-management/decision_log.md
---
# Session Management Implementation

## Pre-work

- [ ] 1. Add UUID dependency to go.mod
  - Run: go get github.com/google/uuid
  - Verify import works in a test file
  - Requirements: [2.1](requirements.md#2.1)

## Log Manager Changes

- [ ] 2. Add new types to logs package
  - Add ManagerOptions struct
  - Add PhaseState struct
  - Add PostCompletionState struct
  - File: internal/logs/manager.go
  - Requirements: [1.5](requirements.md#1.5), [2.4](requirements.md#2.4)

- [ ] 3. Update Summary struct with new fields
  - Add CurrentPhase *PhaseState field
  - Add PostCompletion *PostCompletionState field
  - Add RunNumber int field
  - Add BranchName string field
  - File: internal/logs/manager.go
  - Requirements: [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [2.3](requirements.md#2.3)

- [ ] 4. Update SessionEntry struct
  - Add RunNumber int field
  - File: internal/logs/manager.go
  - Requirements: [1.2](requirements.md#1.2)

- [ ] 5. Update Manager struct
  - Add useSubdirs bool field
  - Add branchName string field
  - File: internal/logs/manager.go
  - Requirements: [1.1](requirements.md#1.1), [1.5](requirements.md#1.5)

- [ ] 6. Write tests for NewManager with flat mode
  - Test flat directory creation
  - Test loading existing summary
  - Test run number increment
  - Test branch mismatch warning
  - Test malformed summary handling
  - File: internal/logs/manager_test.go
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [5.2](requirements.md#5.2)

- [ ] 7. Implement NewManager with ManagerOptions
  - Support UseSubdirs option
  - Load existing summary in flat mode
  - Increment run number on resume
  - Warn on branch mismatch
  - Handle malformed summary.json
  - File: internal/logs/manager.go
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5), [5.2](requirements.md#5.2)

- [ ] 8. Write tests for loadExistingSummary
  - Test successful load
  - Test file not found
  - Test malformed JSON
  - File: internal/logs/manager_test.go
  - Requirements: [1.3](requirements.md#1.3), [5.2](requirements.md#5.2)

- [ ] 9. Implement loadExistingSummary method
  - Read and unmarshal summary.json
  - Return error if not found or malformed
  - File: internal/logs/manager.go
  - Requirements: [1.3](requirements.md#1.3), [5.2](requirements.md#5.2)

- [ ] 10. Write tests for StartPhase
  - Test new session ID generation
  - Test resume existing session
  - Test continueSession=false clears state
  - Test summary written before return
  - File: internal/logs/manager_test.go
  - Requirements: [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [5.1](requirements.md#5.1)

- [ ] 11. Implement StartPhase method
  - Check for existing CurrentPhase
  - Return existing session ID if continuing
  - Generate new UUID if starting fresh
  - Write summary before returning
  - Return (sessionID, isResume, error)
  - File: internal/logs/manager.go
  - Requirements: [2.1](requirements.md#2.1), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [3.1](requirements.md#3.1), [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [5.1](requirements.md#5.1)

- [ ] 12. Write tests for SetCurrentPhaseSessionID
  - Test updates CurrentPhase.SessionID
  - Test writes summary to disk
  - File: internal/logs/manager_test.go
  - Requirements: [3.8](requirements.md#3.8)

- [ ] 13. Implement SetCurrentPhaseSessionID method
  - Update CurrentPhase.SessionID
  - Write summary to disk
  - File: internal/logs/manager.go
  - Requirements: [3.8](requirements.md#3.8)

- [ ] 14. Write tests for ReconcileSessionID
  - Test updates CurrentPhase.SessionID when different
  - Test no-op when IDs match
  - File: internal/logs/manager_test.go
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6)

- [ ] 15. Implement ReconcileSessionID method
  - Compare returned ID with stored ID
  - Update if different
  - File: internal/logs/manager.go
  - Requirements: [2.5](requirements.md#2.5), [2.6](requirements.md#2.6)

- [ ] 16. Write tests for CompletePhase
  - Test clears CurrentPhase
  - Test writes summary
  - File: internal/logs/manager_test.go
  - Requirements: [2.7](requirements.md#2.7)

- [ ] 17. Implement CompletePhase method
  - Set CurrentPhase to nil
  - Write summary to disk
  - File: internal/logs/manager.go
  - Requirements: [2.7](requirements.md#2.7)

- [ ] 18. Write tests for post-completion session methods
  - Test StartPostCompletion returns session info
  - Test CompletePostCompletion clears state
  - Test resume existing post-completion
  - File: internal/logs/manager_test.go
  - Requirements: [3.1](requirements.md#3.1)

- [ ] 19. Implement StartPostCompletion and CompletePostCompletion
  - Similar logic to StartPhase but for PostCompletion state
  - Track in-progress post-completion command
  - File: internal/logs/manager.go
  - Requirements: [3.1](requirements.md#3.1)

- [ ] 20. Write tests for phaseFileName helper
  - Test run-numbered filename when RunNumber > 1
  - Test standard filename when RunNumber = 1
  - Test behavior with UseSubdirs
  - File: internal/logs/manager_test.go
  - Requirements: [1.2](requirements.md#1.2)

- [ ] 21. Implement phaseFileName helper method
  - Return run-numbered filename when RunNumber > 1 in flat mode
  - Return standard filename otherwise
  - File: internal/logs/manager.go
  - Requirements: [1.2](requirements.md#1.2)

- [ ] 22. Update SaveSession to use phaseFileName
  - Replace hardcoded filename format with phaseFileName
  - Include RunNumber in SessionEntry
  - File: internal/logs/manager.go
  - Requirements: [1.2](requirements.md#1.2), [2.8](requirements.md#2.8)

## Claude Client Changes

- [ ] 23. Write tests for RunPhase with session parameters
  - Test --session-id flag when resume=false
  - Test --resume flag when resume=true
  - Test command argument order
  - File: internal/claude/client_test.go
  - Requirements: [2.2](requirements.md#2.2), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3)

- [ ] 24. Update RunPhase signature and implementation
  - Change signature to RunPhase(sessionID string, resume bool)
  - Add --resume sessionID when resume=true
  - Add --session-id sessionID when resume=false
  - File: internal/claude/client.go
  - Requirements: [2.2](requirements.md#2.2), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3)

## Orchestrator Changes

- [ ] 25. Update claudeRunner interface
  - Change RunPhase signature to (sessionID string, resume bool)
  - File: internal/orbit/orbit.go
  - Requirements: [3.2](requirements.md#3.2), [3.3](requirements.md#3.3)

- [ ] 26. Update orbit.Config struct
  - Add DateSubdirs bool field
  - Add ContinueSession bool field
  - File: internal/orbit/orbit.go
  - Requirements: [1.5](requirements.md#1.5), [1.6](requirements.md#1.6), [3.4](requirements.md#3.4), [3.6](requirements.md#3.6)

- [ ] 27. Update orbit.New to pass ManagerOptions
  - Create logs.ManagerOptions with UseSubdirs
  - Pass to logs.NewManager
  - File: internal/orbit/orbit.go
  - Requirements: [1.1](requirements.md#1.1), [1.5](requirements.md#1.5)

- [ ] 28. Write tests for isSessionInvalidError
  - Test detection of session not found
  - Test detection of invalid session
  - Test detection of session expired
  - Test non-session errors return false
  - File: internal/orbit/orbit_test.go
  - Requirements: [3.7](requirements.md#3.7)

- [ ] 29. Implement isSessionInvalidError function
  - Check stderr and output for session error messages
  - Return true for session-related errors
  - File: internal/orbit/orbit.go
  - Requirements: [3.7](requirements.md#3.7)
