---
references:
    - specs/enhanced-status/review-overview-1.md
---
# PR Review Fixes - Iteration 1

- [x] 1. Fix summary.json path to read from main spec log directory instead of worktree

- [x] 2. Change renderTerminal to use output.Text() instead of output.Markdown()

- [x] 3. Fix TestGetLastDisplayableEntry_LargeFile to actually exceed 64KB for window expansion

- [x] 4. Fix decision_log.md path from sessions/{id}.jsonl to {id}.jsonl
