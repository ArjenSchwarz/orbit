---
references:
    - specs/apsis-copilot-support/smolspec.md
---
# Apsis Copilot Support

## Parser Improvements

- [x] 1. Extended thinking appears in Copilot transcripts
  - Add reasoningText field to CopilotData struct in copilot_types.go. Update convertCopilotToEntries() in copilot_parser.go to emit thinking ContentItem before text content.
  - Verify: Run apsis on a session with extended thinking and confirm thinking block renders in collapsible details.

- [x] 2. All Copilot event types are recognized for format detection
  - Add session.model_change, skill.invoked, and abort to copilotTypes map in parser.go.
  - Verify: Format detection succeeds on sessions containing these event types without warnings.

- [x] 3. Parser improvements verified with real session data
  - Test parsing against real Copilot sessions in ~/.copilot/session-state/.
  - Verify: Extended thinking renders correctly, all event types parse without errors, tool results display properly.

## Session Discovery

- [ ] 4. Copilot workspace.yaml metadata can be parsed
  - Add CopilotWorkspace struct to parse workspace.yaml (id, cwd, git_root, created_at fields). Use gopkg.in/yaml.v3. Handle missing/malformed files gracefully.
  - Verify: Unit tests parse sample workspace.yaml correctly.

- [ ] 5. Copilot sessions appear in apsis -l listings
  - Create listCopilotSessions() function following listKiroSessions() pattern. Read ~/.copilot/session-state/ directories, parse workspace.yaml, filter by project path matching git_root or cwd. Update listAllSessions() to include Copilot.
  - Verify: apsis -l shows [copilot] sessions sorted correctly.

- [ ] 6. Copilot session UUIDs resolve to transcript files
  - Create findCopilotSession() function following findCodexSession() pattern. Look up UUID in ~/.copilot/session-state/{uuid}/events.jsonl with case-insensitive matching. Update resolveInput() to try Copilot lookup.
  - Verify: apsis <uuid> resolves and renders Copilot sessions.

- [ ] 7. Session discovery verified end-to-end
  - End-to-end verification: apsis -l shows Copilot sessions, apsis <uuid> works, project filtering works, sorting is correct (Copilot after Claude, before Codex/Kiro in ties).
  - Run existing tests to ensure no regressions.
