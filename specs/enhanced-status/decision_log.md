# Decision Log: Enhanced Status Command

## Decision 1: Feature Scope - Running and Failed Variants

**Date**: 2025-01-26
**Status**: accepted

### Context

The enhanced status command could potentially show detailed information for all variants regardless of their status (pending, running, completed, failed, canceled). Initially, only "running" variants were considered. During review, it was noted that failed variants would benefit from the same detailed view to help diagnose what went wrong.

### Decision

Display enhanced information (commits, git state, last action, task progress) for variants with status "running" or "failed". Other statuses (pending, completed, canceled) are shown in a compact summary format.

### Rationale

Running variants need monitoring for progress. Failed variants need the same information to diagnose issues - seeing the last commits, last action, and task progress at time of failure provides valuable debugging context. Completed variants already have full reports available. Pending variants have no data yet.

### Alternatives Considered

- **Running only**: Rejected - Failed variants lose valuable diagnostic context
- **Show details for all variants**: Rejected - Completed/canceled have full reports; pending has no data
- **Flag to select which statuses to show details for**: Rejected - Adds complexity without clear use case

### Consequences

**Positive:**
- Failed variants show diagnostic information immediately
- Consistent view of "active work" (running) and "recent failures" (failed)
- Matches user mental model of "what needs attention"

**Negative:**
- Slightly more output when failures exist

---

## Decision 2: Commit Display Count

**Date**: 2025-01-26
**Status**: accepted

### Context

Needed to determine how many recent commits to display per running variant. Options considered were 1, 3, or 5 commits.

### Decision

Display the 3 most recent commits since the base commit for each running variant.

### Rationale

Three commits provides enough context to understand recent progress without overwhelming the display. It balances visibility of recent work with screen real estate.

### Alternatives Considered

- **1 commit (latest only)**: Rejected - Too little context for understanding progress
- **5 commits**: Rejected - May be too verbose for multi-variant runs

### Consequences

**Positive:**
- Good balance of context vs brevity
- Shows a meaningful window of recent activity

**Negative:**
- May miss earlier commits in fast-moving variants

---

## Decision 3: Last Action Source - Transcript JSONL

**Date**: 2025-01-26
**Status**: accepted

### Context

The last action summary could be derived from either the summary.json session metadata or by parsing the live JSONL transcript file. The user noted that transcript parsing functionality already exists in apsis follow mode.

### Decision

Parse the current phase's transcript JSONL file to extract the last meaningful action, leveraging existing transcript parsing functionality.

### Rationale

The JSONL transcript contains detailed action information (tool names, inputs, outputs) that provides much richer context than the summary.json metadata. The transcript package already has proven parsing logic that can be reused.

### Alternatives Considered

- **summary.json metadata**: Rejected - Contains only aggregate statistics (phase, cost, duration), not individual actions

### Consequences

**Positive:**
- Rich, detailed action summaries (tool name, file paths, etc.)
- Reuses existing, tested code from transcript package
- Shows what the agent is actually doing

**Negative:**
- Transcript file may be large; need to read efficiently (from end)
- Transcript may be incomplete (agent actively writing)

---

## Decision 4: Output Format - Replace Existing Table

**Date**: 2025-01-26
**Status**: accepted

### Context

The enhanced status could either replace the existing simple table format, add a --verbose flag to show enhanced output, or add a --simple flag to show the old format.

### Decision

Replace the existing simple table format entirely with the new enhanced format. No backward compatibility flag.

### Rationale

The enhanced format is strictly more useful than the simple table. The simple table provided minimal value (just ID, branch, path, status). Users wanting minimal output can use other tools or parse the variants.json directly.

### Alternatives Considered

- **Add --verbose flag**: Rejected - Increases complexity without clear benefit
- **Add --simple flag**: Rejected - Old format has little value; adds maintenance burden

### Consequences

**Positive:**
- Simpler implementation (one code path)
- Better default experience for all users
- No feature fragmentation

**Negative:**
- Breaking change for any scripts parsing old output (unlikely given limited adoption)

---

## Decision 5: Displayable Entry Types for Last Action

**Date**: 2025-01-26
**Status**: accepted

### Context

The transcript JSONL contains many entry types: assistant messages with text, tool_use, thinking blocks, system messages, and meta entries. We needed to define which entries constitute a "meaningful action" to display.

### Decision

Display only these entry types as the "last action":
- `tool_use` content items from assistant messages (shows tool name + key input)
- `text` content items from assistant messages (shows truncated preview)

Exclude:
- Entries with `isMeta: true` (internal markers)
- `thinking` content items (internal reasoning)
- System messages
- User messages (prompts, not actions)

### Rationale

Tool uses represent concrete actions (editing files, running commands). Text represents the agent communicating or explaining. These are the user-visible "work" the agent performs. Thinking blocks are internal reasoning not relevant to status. Meta entries are system bookkeeping.

### Alternatives Considered

- **Include thinking blocks**: Rejected - Too verbose and not user-actionable
- **Show all entry types**: Rejected - Would show system noise instead of actual work

### Consequences

**Positive:**
- Shows meaningful, actionable information
- Filters out internal system noise
- Matches what users would see in transcript viewer

**Negative:**
- If agent is in a long thinking phase, last action may appear stale

---

## Decision 6: Last Action Limited to Claude Code Only

**Date**: 2025-01-26
**Status**: accepted

### Context

Orbit supports multiple AI coding agents: Claude Code, OpenAI Codex, AWS Kiro, and GitHub Copilot. The Last Action feature needs to read the live transcript file while the agent is running. However, the transcript file location differs by agent:

- Claude Code: Transcripts are stored in `~/.claude/projects/{project-hash}/{session-id}.jsonl` and we have the session ID in summary.json
- Other agents: Transcript locations are not consistently accessible during execution

The transcript is only copied to the `.orbit/` directory AFTER a phase completes, so we cannot read from there for running variants.

### Decision

Limit the Last Action feature to Claude Code variants only. For other agents (Codex, Copilot), display "Last action tracking not available for {agent_type}".

### Rationale

Claude Code is the primary agent used with orbit, and we have a reliable way to locate its live transcript using the session ID stored in summary.json combined with the claude.BuildProjectPath function. Other agents don't provide a consistent way to access their live transcript.

### Alternatives Considered

- **Support all agents**: Rejected - Cannot reliably locate live transcripts for non-Claude agents
- **Skip Last Action for all agents**: Rejected - Claude support is valuable and implementable

### Consequences

**Positive:**
- Reliable implementation for the most common use case (Claude Code)
- Clear messaging for unsupported agents
- No false assumptions about transcript locations

**Negative:**
- Reduced functionality for Codex/Copilot users

---

## Decision 7: Use go-output for Flexible Output Formatting

**Date**: 2025-01-26
**Status**: accepted

### Context

The enhanced status command produces structured output that users may want to consume programmatically (JSON) or in different visual formats (table, markdown).

### Decision

Use the go-output library for rendering status output, enabling future support for multiple output formats.

### Rationale

The codebase already uses go-output for other commands. Using it here maintains consistency and enables --format flags for JSON/table/markdown output without additional implementation work.

### Alternatives Considered

- **Direct fmt.Printf**: Rejected - No structured output support, harder to add formats later
- **Custom formatting**: Rejected - Reinvents what go-output already provides

### Consequences

**Positive:**
- Consistent with existing orbit output patterns
- Enables programmatic consumption via JSON format
- Future-proof for additional formats

**Negative:**
- Slight learning curve for go-output API

---
