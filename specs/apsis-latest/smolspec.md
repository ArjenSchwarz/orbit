# Apsis Latest Session

## Overview

Add the ability to view the most recent session in apsis without manually looking up a session ID. Users can type `apsis latest` to automatically resolve and display the newest session for the current project. This removes the friction of running `apsis -l`, finding the latest ID, and copy-pasting it.

## Requirements

- The system MUST accept `latest` as a special positional argument that resolves to the most recent session (e.g., `apsis latest`)
- The system MUST check for the `latest` keyword before calling `isFilePath()`, so that a file or directory named `latest` in the working directory does not shadow the keyword
- The system MUST resolve "most recent" by using `Lister.ListAll()` and selecting the session with the newest `CreatedAt` timestamp (last element, since `ListAll` sorts oldest-first)
- The system MUST resolve the selected session through the existing `Resolver.Resolve()` flow so that all agent types are supported
- The system MUST work with all existing output flags (`-f`, `-o`, `-a`)
- The system MUST work with follow mode (`apsis latest -F`), provided the resolved session is file-backed
- The system MUST return an error when follow mode is used and the latest session is not file-backed (e.g., Kiro CLI/SQLite): `"latest session is a <source> session which cannot be followed (not file-backed)"`
- The system MUST return a clear error when no sessions are found: `"no sessions found for project"`
- The system SHOULD print the resolved session source, ID, and timestamp to stderr before output so the user knows which session was selected

## Implementation Approach

**Key files to modify:**

1. `cmd/apsis/main.go` — Add `resolveLatestSession()` function, integrate `latest` keyword handling into `run()` flow

**Approach:**

The word `latest` as a positional argument is handled by checking `cfg.Input == "latest"` in `run()`, before reaching `resolveInput()` or `resolveFollowInput()`. This check MUST happen before `isFilePath()` is called, since a file named `latest` in the working directory would cause `os.Stat("latest")` to succeed, treating it as a file path instead of the keyword.

Resolution flow:
1. Create a `sessions.Lister` and call `ListAll(projectPath)`
2. Take the last element (newest, since `ListAll` sorts oldest-first)
3. Print the selected session info to stderr: `"Using [source] session <id> from <timestamp>"`
4. Use `sessions.Resolver.Resolve(source, id)` to get the reader — this reuses the existing resolution path including cost path extraction for Kiro IDE

For follow mode: when `cfg.Follow` is true and `cfg.Input == "latest"`, resolve the latest session first. Then check that the resolved session's source is file-backed (Claude, Codex, Copilot, Kiro IDE). If not (Kiro CLI), return an error. If file-backed, use `Resolver.ResolvePath(source, id)` to get the file path and pass it to the existing follow flow.

Note: The existing `--list` validation already rejects `apsis --list latest` via `"cannot specify both --list and a positional argument"`, so no additional validation is needed.

**Existing patterns to follow:**
- `listSessions()` at `cmd/apsis/main.go:419` (already creates a Lister and calls ListAll — reuse the same pattern)
- `resolveInput()` at `cmd/apsis/main.go:368` (session resolution via Resolver)
- `resolveFollowInput()` at `cmd/apsis/main.go:469` (follow mode resolution with source filtering)

**Dependencies:**
- `internal/sessions.Lister` and `internal/sessions.Resolver` — both already exist and are used by `apsis -l` and session resolution
- No new packages needed

**Out of Scope:**
- `--latest` as a separate flag (positional `latest` keyword is sufficient)
- Filtering by agent source (the `-a` flag means "force parse format", not "filter source" — overloading it would be confusing since agent names like `claude-code` don't match source constants like `claude`)
- Showing the N most recent sessions (e.g., `apsis latest 5`)
- Interactive session picker / fuzzy finder

## Risks and Assumptions

- **Risk:** `ListAll()` scans all agent sources which could be slow if a user has thousands of sessions | **Mitigation:** This is the same cost as `apsis -l` which users already run; the scan is fast enough for typical usage
- **Risk:** A file or directory named `latest` exists in the working directory | **Mitigation:** Check for the `latest` keyword before calling `isFilePath()` so the keyword always takes precedence
- **Assumption:** No real session ID will ever be the literal string `"latest"` — session IDs are UUIDs (Claude, Codex, Copilot) or conversation IDs (Kiro), none of which could be `"latest"`
- **Assumption:** `ListAll()` returns at least one session when sessions exist — the sorted list is non-empty
- **Prerequisite:** The `sessions.Lister` and `sessions.Resolver` packages must remain compatible (no interface changes needed)
