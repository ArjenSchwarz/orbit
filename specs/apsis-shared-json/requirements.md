# Requirements: Apsis — Shared JSON Output, Codex Exec Streams and Cross-Project Listing

## Introduction

Apsis translates each harness's transcript format into one shared representation and renders it as Markdown or HTML. This feature publishes that representation as versioned JSON, adds Codex CLI `codex exec --json` event streams as an input, records model providers where available, and exposes sessions across projects in the CLI and `apsis serve`. Other tools can consume supported sources through one contract instead of parsing each harness format.

## Non-Goals

- Changing raw `-f json` output for existing sources.
- Reading LM Studio server logs; [apsis-lmstudio-logs](../apsis-lmstudio-logs/) covers them.
- Reading prompt files beside exec streams.
- Marking sessions as local or cloud, or inferring unrecorded models or providers.
- Changing how orbit invokes Codex or depending on unmerged local-execution work.
- Finding exec streams outside configured directories.
- Extracting new token-usage data from sources other than exec streams.
- Following exec streams or emitting shared JSON live with `-F`.
- OpenCode transcript support or a JSON HTTP endpoint for `apsis serve`.

### 1. Shared JSON Output

**User Story:** As a tool author, I want apsis's shared representation as JSON, so that I can consume every supported source through one schema.

**Acceptance Criteria:**

1. <a name="1.1"></a>Apsis SHALL provide a new `-f` value that emits available source, identifier, creation time, project, model and provider metadata followed by ordered entries, preserving available roles, text, thinking, tool inputs/results and their linking identifiers, error state, exit codes, incomplete state, plans, generic-event payloads, timestamps, model, provider, token usage, cost with its unit, and structured parse warnings with source positions.  
2. <a name="1.2"></a>Session documents SHALL use the same field names and structure for every source, and listing documents SHALL use one common listing structure.  
3. <a name="1.3"></a>Session and listing documents SHALL carry an integer version matching the published JSON Schema; removing a field or changing its meaning SHALL raise the version, while additive fields SHALL remain compatible with consumers that ignore unknown fields.  
4. <a name="1.4"></a>Session fixtures for every supported source and listing fixtures SHALL conform to the published schema, including empty results, warnings and omitted optional metadata.  
5. <a name="1.5"></a>Shared JSON SHALL retain content as parsed without the Markdown and HTML renderers' tool-length limits.  
6. <a name="1.6"></a>WHERE session-level model or provider summaries are emitted, apsis SHALL use the first known value in transcript order, preserve subsequent recorded changes on entries, and omit unavailable values.  
7. <a name="1.7"></a>Shared JSON session and listing metadata SHALL include the client entry point or originator a source records, such as Claude Code's `entrypoint` and Codex's `session_meta.payload.originator`, exactly as recorded and using the first known value as in [1.6](#1.6), without classifying it as interactive or automated; WHERE a source records none, apsis SHALL omit it.  
8. <a name="1.8"></a>Each shared JSON entry SHALL carry any identifier the source records for it, such as a Claude Code entry `uuid`, separately from tool-linking identifiers; WHERE a source records none, apsis SHALL omit it.  

### 2. Codex Exec Event Streams

**User Story:** As a user of `codex exec --json`, I want apsis to list and render saved event streams, so that non-interactive runs are reviewable.

**Acceptance Criteria:**

1. <a name="2.1"></a>Apsis SHALL auto-detect a file or stdin as an exec stream when its first parseable, non-empty JSON event is `thread.started` with a nonempty string `thread_id`, allowing a leading BOM; `-a codex-exec` SHALL force parsing of streams that lack that event, including failures before thread creation.  
2. <a name="2.2"></a>Apsis SHALL translate `agent_message` as assistant text, `reasoning` as thinking, `command_execution` as linked tool use/result with command, output and recorded exit code, `file_change`, `mcp_tool_call` and `web_search` as tool activity with recorded inputs/results, `todo_list` as plan entries, and `error` items and `turn.failed` or `error` events as errors; Markdown and both HTML renderers SHALL expose the corresponding content and state.  
3. <a name="2.3"></a>Apsis SHALL use `thread.started`, `turn.started`, `item.started`, `item.updated` and `item.completed` to retain turn membership and one final state per item within its turn, ordered by first appearance; a started item without completion SHALL retain its last known state marked incomplete, without rendering lifecycle updates separately.  
4. <a name="2.4"></a>WHEN an event or item type has no specialised translation, apsis SHALL retain its type and parsed payload as a generic event in shared JSON and expose that content in Markdown and HTML; an unknown item's lifecycle SHALL follow [2.3](#2.3).  
5. <a name="2.5"></a>WHERE directories are configured, apsis SHALL discover qualifying `.jsonl` files at any depth under source `codex-exec`, listing each canonical file once across overlapping roots; recursive descent SHALL skip `.git`, `node_modules` and `testdata` subdirectories, while an explicitly configured root SHALL itself be scanned regardless of its name; WHERE no directories are configured, no exec streams SHALL be listed.  
6. <a name="2.6"></a>An exec stream SHALL belong to its nearest containing git repository or worktree root and appear in a project listing only when that root equals the selected project after path normalisation; streams outside repositories SHALL appear only across projects.  
7. <a name="2.7"></a>WHEN a resolvable Codex rollout with the same thread ID exists, apsis SHALL suppress the exec-stream listing in every scope and use the rollout's source, identifier and recorded project; project filtering SHALL NOT revive the suppressed stream, and explicit stream-file conversion SHALL remain available.  
8. <a name="2.8"></a>Each listed exec stream SHALL have a nonempty identifier derived from its thread ID, unique within `codex-exec`, containing at most 256 ASCII letters, digits, `_` or `-`, and unchanged while its canonical path and recorded thread ID remain unchanged, including after appends, added copies, overlapping roots or configuration-order changes; the original thread ID SHALL remain separate metadata.  
9. <a name="2.9"></a>Apsis SHALL retain every field of `turn.completed.usage` on an entry associated with that turn; for Codex 0.166.0 the values SHALL be labelled as reported thread totals at completion and shown with that turn in Markdown and HTML, without summing snapshots or presenting them as per-turn deltas, and unknown usage fields SHALL remain representable.  
10. <a name="2.10"></a>Exec-stream output SHALL omit source creation time and entry timestamps when unrecorded; file modification time SHALL supply an explicitly identified filesystem fallback for listing order and `latest`, distinct from source creation time, and stdin SHALL receive no filesystem timestamp.  
11. <a name="2.11"></a>WHEN forced conversion has no valid thread identifier, apsis SHALL render available events without inventing one; discovery SHALL omit such files with a warning, and direct conversion SHALL take the recorded thread identifier from content rather than substituting the filename.  

### 3. Model Provider

**User Story:** As a reviewer, I want the recorded provider displayed beside the model, so that I can identify the service recorded by the source.

**Acceptance Criteria:**

1. <a name="3.1"></a>Apsis SHALL preserve the provider a session records, from Codex's `session_meta.payload.model_provider` and Kiro IDE's `metadata.modelProvider`, display it beside the model in Markdown and HTML following the existing metadata pattern, and include it in shared JSON; WHERE a source records no provider, apsis SHALL omit it silently.  

### 4. Listing Across Projects

**User Story:** As a user or tool author, I want to list and open sessions across projects, so that I can review work without changing directories.

**Acceptance Criteria:**

1. <a name="4.1"></a>`apsis --list` and `apsis serve` SHALL accept an all-projects option that lists every supported source and shows project paths where recorded, or the containing repository/worktree for exec streams.  
2. <a name="4.2"></a>Each listed session SHALL open from an unrelated directory through the CLI with the same scope and an explicit source selector, and through its web source/identifier route; the CLI source selector SHALL be distinct from the forced-format option, and legacy bare-identifier resolution SHALL remain available.  
3. <a name="4.3"></a>`apsis --list` with the new `-f` value SHALL emit shared JSON containing source, identifier, size, recorded creation time, project, model, provider and origin where known, any explicitly marked filesystem sort fallback, listing warnings and schema version; unavailable metadata SHALL be omitted.  
4. <a name="4.4"></a>The all-projects option SHALL also scope `apsis latest`; selection SHALL use recorded creation time or the marked filesystem fallback, sort unknown times before known times, and break equal-time ties by source then identifier, consistently with listings.  

### 5. Compatibility, Settings and Verification

**User Story:** As a maintainer, I want predictable settings and checks of parsing and discovery, so that these additions can merge independently.

**Acceptance Criteria:**

1. <a name="5.1"></a>Without the new format, all-projects option or configured exec directories, existing output SHALL remain unchanged except for recorded Codex and Kiro IDE providers; configuring exec directories SHALL also make eligible streams available to ordinary listings and `latest`.  
2. <a name="5.2"></a>IF an exec-stream line is malformed, THEN apsis SHALL report its physical one-based line number, continue at the next line, and expose the diagnostic on stderr, in shared JSON warnings and in the web transcript view without copying the malformed line into the diagnostic.  
3. <a name="5.3"></a>IF `-F` is combined with shared JSON or an exec stream, including one selected by `latest`, THEN apsis SHALL exit with an error naming the unsupported combination; raw `-f json` conversion of exec streams SHALL emit their JSON events following the existing JSONL raw-output convention.  
4. <a name="5.4"></a>Exec-directory settings SHALL be available through environment and CLI for both `apsis` and `apsis serve`, with CLI precedence, no directories by default, an empty environment value disabling discovery, and a documented way to supply several directories.  
5. <a name="5.5"></a>Before discovery reads or identifier-based opening, apsis SHALL resolve file and root symlinks and reject files outside every configured root; discovery SHALL NOT descend through directory symlinks, and explicit file/stdin conversion SHALL remain independent of configured roots.  
6. <a name="5.6"></a>Exec fixtures SHALL contain synthetic content and establish shape provenance from version-pinned authoritative definitions or sanitised structural observations; unobserved types SHALL cite their definition rather than claim capture provenance, and unverified shapes SHALL receive generic-event handling.  
7. <a name="5.7"></a>Verification SHALL cover multiple turns, interleaved tool links, item updates and EOF, failures before thread creation, unknown payloads/usage fields, malformed middle lines, and all new content/state in shared JSON, Markdown, standalone HTML and web HTML.  
8. <a name="5.8"></a>Verification SHALL round-trip CLI and web listings to opening from an unrelated directory for every source, including Kiro execution details, identical identifiers in different sources, duplicate/resumed streams, differing rollout/stream projects, overlapping roots, escaping symlinks and directory cycles.  
9. <a name="5.9"></a>Within one `apsis serve` process, refreshing unchanged inputs SHALL read zero transcript-body bytes after the initial listing; changed, added or removed inputs SHALL be reflected on the next successful refresh without renaming unaffected exec streams, and unreadable configured roots/files SHALL surface warnings without discarding other results.  

## Format Evidence

Codex 0.166.0 specialised fixture shapes are established by the pinned [exec event definitions](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/exec_events.rs). The pinned [JSONL emitter](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/event_processor_with_jsonl_output.rs) copies reported total counters into completion usage. Its [resume path](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/exec/src/lib.rs) reuses the resumed thread identifier, so a thread identifier alone cannot distinguish stream files.
