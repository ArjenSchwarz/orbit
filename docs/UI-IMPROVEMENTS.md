# UI/UX Improvements: Multi-Variant Parallel Run Display

## Summary

Review of Orbit's terminal display during parallel multi-variant runs. Currently, `orbit run --variants N --parallel` produces interleaved `log.Printf` lines with no visual structure. The existing single-variant spinner doesn't work for parallel execution, and `orbit status` requires a separate terminal invocation.

Five alternative display approaches were evaluated. However, the review also uncovered a deeper problem: the underlying data model assumes sequential phase execution per variant, while modern agents (particularly Claude Code) use subagents to work on multiple tasks and phases simultaneously. **The display problem and the data model problem must be solved together.**

## Critical Issues

### Issue 1: No structured real-time feedback during parallel runs

**Current State**: Parallel variant execution outputs raw `log.Printf` lines that interleave unpredictably across variants. The single-variant spinner is disabled entirely.

**Problem**: Users cannot answer "are my variants making progress or stuck?" without running `orbit status` in a separate terminal.

**Recommendation**: Implement a layered display system (see approaches below).

### Issue 2: Data model assumes one active phase per variant

**Current State**: `summary.json` uses a single `CurrentPhase *PhaseState` field. The orchestration loop calls `GetNextPhase()` (singular), runs that phase, then checks for the next one. The status display shows one `IsActive` arrow per variant.

**Problem**: Agents with subagent capabilities (Claude Code, potentially others) can work on multiple tasks from different phases simultaneously. When a Claude Code session spawns subagents, they can:
- Work on multiple tasks within the same phase in parallel
- Start tasks from the next phase while the current phase's tasks are still in progress
- Complete phases out of order

The current tracking has no visibility into this. `CurrentPhase` shows Phase 2 while the agent might actually have subagents working on tasks in Phase 2 *and* Phase 3. The `PhaseSummary` code already counts `inProgress` tasks per phase (rune tracks `StatusInProgress` per task), but the orchestration and display ignore this.

**What rune already tracks**: Each task has its own status (pending/in_progress/completed). `GetPhaseSummaries()` already counts in-progress tasks per phase. Multiple phases *can* have `PhaseStatusInProgress` simultaneously (the logic checks `inProgress > 0 || completed > 0`). The data is there; the display doesn't surface it.

**Recommendation**: The display must show task-level granularity, not just phase-level. Any approach needs to handle showing "Phase 2: 6/8 done, 2 active | Phase 3: 0/7 done, 3 active" for a single variant.

---

## Approach Evaluation

All mockups below account for multi-phase concurrent activity within a single variant.

### Approach 1: Full-Screen TUI (bubbletea/lipgloss)

**Concept**: Take over the terminal entirely with an htop-style dashboard. Each variant gets a panel showing all active phases with per-task progress. Real-time updates via event channel. Keyboard navigation between variants for detail views.

**Mockup**:
```
┌─ Orbit: my-feature (3 variants) ────────────────────── 12m 34s elapsed ─┐
│                                                                          │
│  V1 claude-code                                         $0.42   3 commits│
│  ├─ Phase 2: Core   ████████░░ 6/8 done  2 active                       │
│  ├─ Phase 3: API    ░░░░░░░░░░ 0/7 done  3 active                       │
│  └─ Last: [subagent-2] Writing handler for /users                        │
│                                                                          │
│  V2 codex                                               $0.18   1 commit │
│  ├─ Phase 2: Core   ██░░░░░░░░ 2/8 done  1 active                       │
│  └─ Last: Generating test fixtures                                       │
│                                                                          │
│  V3 copilot                                             12 PR   4 commits│
│  ├─ Phase 2: Core   ██████████ 8/8 done  ✓                              │
│  ├─ Phase 3: API    █████░░░░░ 5/7 done  2 active                        │
│  └─ Last: Refactoring auth module                                        │
│                                                                          │
│  [q] Quit  [1-3] Focus variant  [w] Open web UI  [l] Show logs          │
└──────────────────────────────────────────────────────────────────────────┘
```

**Strengths**:
- Can show multiple active phases per variant without cramming
- Keyboard-driven interaction for detail drilldown
- Familiar paradigm (htop, k9s, lazygit)

**Weaknesses**:
- Locks the terminal entirely - no scrollback, can't use for other things
- Heavyweight dependency (bubbletea + lipgloss + bubbles)
- Breaks pipe/redirect workflows and non-TTY environments
- Overkill for 2-5 variants
- Significant implementation complexity (event system, key handling, resize)
- Conflicts with "run in background and check occasionally" workflow

**UX Verdict**: The multi-phase-per-variant display benefits from the vertical space a TUI provides. But the terminal lock-in remains a significant drawback for 10-60 minute runs. Not recommended as the primary approach.

---

### Approach 2: Pinned Footer with Scrolling Logs (Docker Compose Style)

**Concept**: Structured log output scrolls in the main area. A pinned multi-line status region at the bottom shows compact status per variant, updated in place using ANSI cursor movement. Each variant can use 1-2 lines depending on how many phases are active.

**Mockup** (simple case - one active phase per variant):
```
  ... structured log output scrolls above ...
  14:23:01 [V1] Phase 3 started: API Implementation (7 tasks)
  14:23:15 [V2] Retry 1/5: connection timeout
  14:24:01 [V1] Commit: abc1234 "Add user endpoints"
  ─────────────────────────────────────────────────────────────
  V1 claude  Phase 2 [6/8 ██████░░] Phase 3 [0/7 +3 active]  $0.42  18m
  V2 codex   Phase 2 [2/8 ██░░░░░░ +1 active]                 $0.18  18m ⚠
  V3 copilot Phase 2 ✓    Phase 3 [5/7 █████░░ +2 active]     12 PR  18m
```

**Mockup** (expanded - when phases overlap significantly):
```
  ─────────────────────────────────────────────────────────────
  V1 claude  ■ Core 6/8 +2active  ■ API 0/7 +3active   $0.42  18m
  V2 codex   ■ Core 2/8 +1active                        $0.18  18m ⚠
  V3 copilot ✓ Core 8/8           ■ API 5/7 +2active    12 PR  18m
```

**Strengths**:
- Scrollable log history + persistent status
- Proven pattern (Docker Compose, Turborepo)
- Can show multiple active phases per variant in one line using compact notation
- Non-TTY graceful degradation to periodic summary lines

**Weaknesses**:
- One line per variant gets tight when showing multiple active phases + cost + time
- ANSI cursor manipulation needs care across terminal emulators
- Footer height varies if some variants have more active phases than others
- "Active tasks" count within a phase is a new data point that needs polling

**UX Verdict**: Still the strongest real-time option. The compact notation (`Phase 2 [6/8 +2active]`) handles multi-phase concurrency without needing extra lines. The key challenge is fitting it all in one line per variant at reasonable terminal widths (~100 chars).

---

### Approach 3: Timeline/Gantt Visualization

**Concept**: Each variant gets horizontal phase bars. But instead of just showing sequential blocks, overlapping phases are shown as stacked or interleaved bars to visualize the actual parallel execution pattern.

**Mockup**:
```
  Orbit: my-feature                          0m        10m        20m
  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┼┄┄┄┄┄┄┄┄┄┄┼┄┄┄┄┄┄┄┄┄┄┼
  V1 claude  Setup ████│ Core ████████▓▓▓
                       │      API ▓▓▓▓▓▓▓▓░░░░░░░
  V2 codex   Setup ██████│ Core ▓▓▓▓▓▓▓░░░░░
  V3 copilot Setup ███│ Core █████████│ API ▓▓▓▓░░░
                       │              │     Tests ▓░░░

  ████ done  ▓▓▓▓ active  ░░░░ estimated remaining
  5 tasks active across V1 (Phase 2+3), 1 in V2, 2 in V3 (Phase 3+4)
```

**Strengths**:
- Naturally shows phase overlap - stacked bars make concurrency visible
- Reveals which agents parallelize more aggressively
- Good for comparing agent strategies (sequential vs parallel)

**Weaknesses**:
- Variable height per variant (depends on phase overlap) makes layout unpredictable
- Terminal width severely limits time resolution
- Hard to show precise task counts in the bars
- Better for post-hoc analysis than real-time monitoring

**UX Verdict**: The stacked-bar approach for overlapping phases is genuinely useful for understanding agent behavior differences. Recommended for **comparison reports and the web UI**, not for live terminal monitoring.

---

### Approach 4: Structured Event Stream (GitHub Actions / Terraform Style)

**Concept**: Replace raw `log.Printf` with structured, color-coded events. Key addition: task-level events showing when individual tasks start/complete, and periodic summary lines that show all active work across phases.

**Mockup**:
```
  ── my-feature: 3 variants starting ──────────────────────────

  14:20:01  V1 claude   ▶ Phase 1: Setup (5 tasks)
  14:20:01  V2 codex    ▶ Phase 1: Setup (5 tasks)
  14:20:02  V3 copilot  ▶ Phase 1: Setup (5 tasks)
  14:22:30  V3 copilot  ✓ Phase 1 complete (2m 28s, $0.04)
  14:22:30  V3 copilot  ▶ Phase 2: Core Implementation (8 tasks)
  14:23:01  V1 claude   ✓ Phase 1 complete → continuing in Phase 2 (8 tasks)
  14:23:05  V1 claude   ⊕ Phase 3: API Layer started (3 tasks active via subagents)
  14:23:15  V2 codex    ⚠ Retry 1/5: connection timeout (backoff 2s)

  ── Status ──────────────────────────────────────────────────────────
  V1 claude   Phase 2 [6/8] + Phase 3 [0/7, 3 active]    $0.42  18m
  V2 codex    Phase 2 [2/8, 1 active]                     $0.18  18m
  V3 copilot  Phase 2 ✓ → Phase 3 [5/7, 2 active]        12 PR  18m
  ─────────────────────────────────────────────────────────────────────

  14:25:12  V1 claude   ● Commit: abc1234 "Add user model and migrations"
  14:25:30  V1 claude   ✓ Phase 2 complete (all 8 tasks done)
  14:26:44  V3 copilot  ✓ Phase 3 complete (4m 14s, 12 PR)
  14:27:01  V2 codex    ✗ Fatal: authentication failed

  ── Status ──────────────────────────────────────────────────────────
  V1 claude   Phase 3 [2/7, 5 active]                     $0.58  22m
  V2 codex    FAILED                                       $0.23   7m
  V3 copilot  Phase 4 [0/4, 2 active]                     14 PR  22m
  ─────────────────────────────────────────────────────────────────────
```

**Strengths**:
- The `⊕ Phase started via subagents` event type naturally communicates cross-phase work
- Summary blocks show multi-phase state compactly
- Full scrollback preserved - can trace the history of how phases overlapped
- Works in non-TTY environments (pipes, CI, logs)
- Low implementation complexity
- The periodic summary blocks serve as "checkpoints" for scanning

**Weaknesses**:
- No persistent live view - have to find the last summary block
- Summary blocks become stale between injections
- Detecting "subagent started a new phase" requires polling rune, since the orchestration loop doesn't know about it

**UX Verdict**: The best foundation layer. The periodic summary blocks with multi-phase state are informative and handle the subagent case naturally. Should be implemented regardless of other approaches.

---

### Approach 5: Auto-Refreshing Watch Dashboard

**Concept**: `orbit status --watch` auto-refreshes every N seconds. Shows full detail including all active phases per variant, active task counts, and which specific tasks are in progress.

**Mockup**:
```
  ┌ Orbit Status: my-feature ──────────── Refreshed: 14:24:01 (every 3s) ┐

  Variant 1: impl-1-claude [running (clean)]               18m   $0.42
  ┌──────────────────────────────────────────────────────────────────────┐
  │ Phase 2: Core Implementation          ████████░░  6/8 done          │
  │   ◉ Task 2.5: Implement user service  (in progress)                 │
  │   ◉ Task 2.7: Add input validation    (in progress)                 │
  │ Phase 3: API Layer                    ░░░░░░░░░░  0/7 done          │
  │   ◉ Task 3.1: Create route handlers   (in progress, subagent)       │
  │   ◉ Task 3.2: Add middleware chain     (in progress, subagent)       │
  │   ◉ Task 3.3: Error response format    (in progress, subagent)       │
  │ Last: Writing handler for /users                                     │
  └──────────────────────────────────────────────────────────────────────┘

  Variant 2: impl-2-codex [running (dirty)]                 18m   $0.18
  ┌──────────────────────────────────────────────────────────────────────┐
  │ Phase 2: Core Implementation          ██░░░░░░░░  2/8 done          │
  │   ◉ Task 2.3: Database migrations     (in progress)                  │
  │ ⚠ Retry: connection timeout (1m ago)                                │
  └──────────────────────────────────────────────────────────────────────┘

  Variant 3: impl-3-copilot [running (clean)]               18m   12 PR
  ┌──────────────────────────────────────────────────────────────────────┐
  │ Phase 2: Core Implementation          ██████████  8/8 ✓             │
  │ Phase 3: API Layer                    █████░░░░░  5/7 done          │
  │   ◉ Task 3.6: Auth token validation   (in progress)                  │
  │   ◉ Task 3.7: Rate limiting           (in progress)                  │
  └──────────────────────────────────────────────────────────────────────┘

  └────────────────────────────────────────────────── [Ctrl+C to stop] ──┘
```

**Strengths**:
- Has room to show task-level detail (which specific tasks are in progress)
- Makes subagent parallelism visible at the task level
- Reuses `status.Gatherer` infrastructure
- Clear separation: `orbit run` logs events, `orbit status --watch` shows the dashboard
- Per-variant boxes make scanning easy even with variable content height

**Weaknesses**:
- Screen-clear-and-redraw causes flicker
- Loses log history between refreshes
- Requires rune to expose which specific tasks are in progress (currently only counts)
- Full-screen clear is disorienting during long runs
- Doesn't work in non-TTY environments

**UX Verdict**: The task-level detail is the unique value here. No other approach can comfortably show *which specific tasks* are being worked on across multiple phases. Best as a complementary `orbit status --watch` command, not the primary `orbit run` display.

---

## Data Model Changes Required

Regardless of which display approach is chosen, the underlying data needs updating:

### 1. Track active tasks, not just active phase
- `PhaseSummary` already counts `inProgress` tasks - surface this in the display
- Consider adding `InProgress int` to `PhaseSummary` explicitly (currently computed but not stored)
- The status gatherer needs to return per-phase active task counts

### 2. Support multiple active phases in status output
- `TaskOutput` already has `IsActive bool` - multiple phases can be active simultaneously
- The status display rendering needs to handle showing multiple `→` arrows
- The footer/summary needs to show all active phases, not just the "current" one

### 3. Expose which specific tasks are in progress (for watch mode)
- Rune's `ListAll()` returns tasks with `StatusInProgress` - filter and surface these
- Add task titles to the status output for in-progress tasks
- This enables the task-level detail in Approach 5

### 4. Detect cross-phase work during execution
- Poll `GetPhaseSummaries()` periodically during execution (not just between phases)
- Emit events when a new phase has in-progress tasks (indicating subagent started it)
- This doesn't require changing the orchestration loop - just observing rune state

---

## Recommendation: Layered Approach

### Layer 1 (Foundation): Structured Event Stream with Task Awareness
- Replace `log.Printf` with structured, color-coded, variant-prefixed events
- Poll rune periodically during phase execution to detect cross-phase activity
- Emit `⊕ Phase N started` events when new phases gain in-progress tasks
- Inject periodic summary blocks showing all active phases per variant
- **Priority: High** - improves the experience everywhere, handles subagent case

### Layer 2 (Real-time): Pinned Footer with Multi-Phase Status
- When TTY detected, pin compact status below the event stream
- Show all active phases per variant: `V1 claude  Core [6/8 +2] API [0/7 +3]  $0.42  18m`
- Falls back to event stream only when non-TTY
- **Priority: High** - biggest UX win for interactive use

### Layer 3 (Monitoring): Watch Mode with Task-Level Detail
- `orbit status --watch` auto-refreshes every 3s
- Shows which specific tasks are in progress per phase
- Makes subagent parallelism fully visible
- **Priority: Medium** - complementary tool for second terminal

### Layer 4 (Post-run): Phase Overlap Visualization
- Stacked timeline bars in comparison report showing phase overlap patterns
- Reveals which agents parallelized more aggressively
- Better fit for web UI than terminal
- **Priority: Low** - useful for analysis, not monitoring

### Not Recommended: Full-Screen TUI
- Terminal lock-in is a significant drawback for 10-60 minute runs
- The task-level detail that justifies a TUI is better served by watch mode
- Reconsider only if orbit grows to manage 10+ parallel processes

## Positive Observations

- `GetPhaseSummaries()` already counts in-progress tasks per phase - the data pipeline is partially there
- `PhaseSummary.Status` already supports `PhaseStatusInProgress` for multiple phases simultaneously
- The `status.Gatherer` with concurrent data collection provides a solid foundation
- The web UI exists for deep inspection, so terminal display can focus on awareness
- `summary.json` persistence means status can be gathered without IPC to the running process
