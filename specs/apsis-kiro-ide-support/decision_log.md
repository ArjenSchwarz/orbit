# Decision Log: Kiro IDE Apsis Support

## Decision 1: Spec Naming

**Date**: 2026-02-06
**Status**: accepted

### Context

Need a name for the spec directory that aligns with existing conventions in the repository.

### Decision

Use `apsis-kiro-ide-support` as the spec name.

### Rationale

User preferred this over `kiro-ide-apsis-support`. The `apsis-` prefix groups it with `apsis-copilot-support` and similar specs.

### Alternatives Considered

- **kiro-ide-apsis-support**: Agent-first naming - Rejected by user preference
- **apsis-kiro-ide**: Shorter variant - Rejected by user preference

### Consequences

**Positive:**
- Consistent grouping with other apsis-related specs

**Negative:**
- None identified

---

## Decision 2: Session ID Format

**Date**: 2026-02-06
**Status**: accepted

### Context

Kiro IDE has two potential identifiers for sessions: `executionId` (found in `.chat` files, maps directly to execution detail files) and `chatSessionId`/workspace-sessions `sessionId` (which has associated titles but requires extra lookup).

### Decision

Use `executionId` as the session ID displayed in `apsis -l` and used for session resolution.

### Rationale

The `executionId` maps directly to `.chat` files and can be used to deterministically locate execution detail files via `SHA-256[:32](executionId)`. The `chatSessionId` requires cross-referencing workspace-sessions index files and adds complexity without clear benefit for CLI usage.

### Alternatives Considered

- **chatSessionId**: Maps to workspace-sessions index with titles - Rejected because it adds a lookup step and the title information isn't used in apsis list output

### Consequences

**Positive:**
- Simple, direct mapping to source files
- Deterministic path computation for execution details
- Consistent UUID format matching other agents

**Negative:**
- Session titles from workspace-sessions are not surfaced (could be added later)

---

## Decision 3: Cost Tracking Approach

**Date**: 2026-02-06
**Status**: accepted

### Context

Cost/usage data for Kiro IDE sessions is stored in execution detail files (separate from `.chat` transcript files). Execution detail files contain a `usageSummary` array with credit costs per turn.

### Decision

Extract costs from execution detail files, matching via `executionId`.

### Rationale

Cost tracking is valuable for users monitoring their usage. The execution detail file can be located deterministically at `{workspace_dir}/414d1636299d2b9e4ce7e17fb11f63e9/{sha256_32(executionId)}`, making extraction straightforward without scanning.

### Alternatives Considered

- **Skip costs initially**: Simpler implementation - Rejected because cost extraction is straightforward given the deterministic file path

### Consequences

**Positive:**
- Users can see session costs in transcript headers
- Consistent with Kiro CLI which already surfaces cost data

**Negative:**
- Additional file I/O per session conversion (one extra file read)

---

## Decision 4: Platform Support

**Date**: 2026-02-06
**Status**: accepted

### Context

Kiro IDE stores session data in platform-specific locations. The known path is macOS-specific (`$HOME/Library/Application Support/Kiro/...`).

### Decision

Support all platforms (macOS, Linux, Windows) from the start.

### Rationale

The Kiro CLI agent already implements cross-platform path resolution. Adding all platforms upfront avoids needing a follow-up change.

### Alternatives Considered

- **macOS only initially**: Less work - Rejected because cross-platform support is minimal additional effort and avoids a future breaking change

### Consequences

**Positive:**
- Works on all platforms from day one
- Consistent with other agent implementations

**Negative:**
- Linux and Windows paths are inferred (not yet verified against real installations)

---

## Decision 5: Transcript Source Strategy

**Date**: 2026-02-06
**Status**: accepted

### Context

Kiro IDE has two sources of transcript data: `.chat` files (simple human/bot/tool role-based messages) and execution detail files (rich action logs with tool names, inputs, outputs, timing). Both contain the full session but in different structures.

### Decision

Use `.chat` files as the primary transcript source, with execution detail files for cost enrichment only.

### Rationale

The `.chat` format maps cleanly to the existing `Entry` type (user/assistant/tool roles). The execution detail file's flat action list would require complex reconstruction of conversation flow. Using `.chat` for structure and execution details for cost gives the best of both approaches.

### Alternatives Considered

- **.chat files only**: Simpler but no cost data - Rejected because cost extraction is valuable and straightforward
- **Execution detail files only**: Richer data but complex parsing to reconstruct conversation turns - Rejected because the action-based format doesn't map naturally to the Entry model

### Consequences

**Positive:**
- Simple parser for the primary transcript
- Cost data still available via execution detail enrichment
- Clean separation of concerns

**Negative:**
- Tool call details (names, arguments, outputs) from execution details are not included in the transcript — only the bot's narrative descriptions from `.chat` files are shown

---

## Decision 6: No Follow Mode

**Date**: 2026-02-06
**Status**: accepted

### Context

Apsis supports follow mode (`-F`) for Claude, Codex, and Copilot sessions (all JSONL-based, suitable for tailing). Kiro IDE uses JSON `.chat` files that are cumulative snapshots — the file is rewritten entirely on each update rather than appended to.

### Decision

Do not support follow mode for Kiro IDE sessions initially.

### Rationale

The `.chat` files are not streaming-friendly (complete JSON rewrites, not appended lines). Implementing follow mode would require polling and re-parsing the entire file on each check, which is fundamentally different from the JSONL tail approach used by other agents.

### Alternatives Considered

- **Follow via polling**: Poll for updated .chat files at intervals - Rejected because it would require a different implementation pattern from existing follow mode and adds complexity for limited benefit

### Consequences

**Positive:**
- Simpler initial implementation
- Avoids introducing a polling-based follow pattern that differs from existing implementations

**Negative:**
- Users cannot monitor Kiro IDE sessions in real-time via apsis

---

## Decision 7: Review-Driven Requirements Refinements

**Date**: 2026-02-06
**Status**: accepted

### Context

After the initial requirements draft, design-critic and peer-review-validator reviews identified several issues: a priority conflict between requirements 1.7 and 7.4, a timestamp source inconsistency, missing path normalization, missing `.chat` file extension recognition, and an undocumented magic constant.

### Decision

Address all "must fix" and "should fix" items from the reviews:

1. Fix priority conflict: kiro-cli stays at 3, kiro ide gets 4 (requirement 8.4 corrected)
2. Simplify timestamp extraction: use representative file's `metadata.startTime` with mtime fallback (requirement 1.5 corrected)
3. Add path normalization via `filepath.Abs()` + `filepath.Clean()` before SHA-256 hashing (requirement 1.1 updated)
4. Add `.chat` extension recognition in `isFilePath()` (new requirement section 7)
5. Document the `414d1636299d2b9e4ce7e17fb11f63e9` magic constant origin in a Background section
6. Tighten system prompt filtering condition (requirement 4.5)
7. Add malformed entry handling (requirement 4.10)
8. Add `metadata.startTime` fallback to file mtime (requirement 1.5)
9. Clarify Kiro IDE vs Kiro CLI format detection distinction (requirement 4.7)
10. Use `os.UserConfigDir()` explicitly for cross-platform path resolution (requirement 2.4)

### Rationale

The reviews identified genuine gaps and internal inconsistencies that would cause implementation confusion if left unresolved.

### Alternatives Considered

- **Defer minor fixes**: Address only the priority conflict and timestamp mismatch - Rejected because the other issues are low-effort to fix now and prevent implementation ambiguity

### Consequences

**Positive:**
- Requirements are internally consistent
- Edge cases addressed upfront
- Clear guidance for implementers on path normalization and format detection

**Negative:**
- None identified

---
