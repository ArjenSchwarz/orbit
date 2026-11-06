# Requirements: Apsis — LM Studio Server Logs

## Introduction

LM Studio server logs can retain inference requests from clients that keep no transcript. This feature translates those logs into apsis's shared representation for Markdown, HTML and JSON, making missing content and uncertain attribution visible. It builds on [apsis-shared-json](../apsis-shared-json/requirements.md) and targets the server-log grammar reported for LM Studio 0.4.26.

## Dependencies and Portability

- Shared JSON, its integer schema version, explicit source selection and all-projects scope come from `apsis-shared-json`; LM Studio adds fields without replacing the common session or listing structure.
- Public endpoint documentation establishes payload shapes, not the disk-log grammar. Synthetic fixtures must distinguish documented payloads from reported log-grammar observations.
- The source label, forced format and provider are `lmstudio`. Provider does not assert that inference ran locally.
- Explicit time-zone overrides have the same meaning on every supported release platform, including Windows; valid IANA zones must not depend on a separately installed system zone database.
- This document is a merged agent proposal. [Open questions](drafts/requirements/questions.md) identify unsettled user-owned choices; no LM Studio requirements approval is recorded.

## Terms

- **Header:** a physical line starting `[YYYY-MM-DD HH:MM:SS][LEVEL]`, optionally followed by tags.
- **Record:** a header and continuation lines up to the next header or end of input. Source positions are one-based physical line ranges within a file or stdin.
- **Request:** a `Received request: POST to <path>` record for an exact included endpoint in [2.1](#2.1), optionally followed by `with body` and multiline JSON.
- **Logged response:** a reported `Generated prediction:` or `Generated synchronous v1/responses response:` record containing response data; a `Returning` record alone does not establish inference output.
- **Logical session:** requests sharing the supported explicit client thread identifier, or one unidentified request. A direct log transcript can contain several logical sessions.
- **Source order:** physical order within input; discovery orders files by filename date, numeric `.N` suffix, then physical record position.

## Non-Goals

- Reading saved LM Studio conversations, `lms log stream` output or an LM Studio database.
- Calling a model server, changing logging settings or recovering content the log omits.
- Inferring project paths from prompts, model names or provider names.
- Merging server evidence into a client harness transcript or suppressing either listing.
- Grouping by prompt similarity, time windows, cache keys or response chains.
- Copying unattributed response or diagnostic bodies into logical-session views.
- Rendering sampling settings and tool definitions as conversation messages.
- Specialised translation of `/v1/decisions`; its typed question/answer contract is outside this conversation-endpoint slice, and unsupported POST paths remain visible through warnings.
- Live following with `-F`, raw `-f json` output for this source or a persistent disk index.
- Serving direct log files through browser file routes or accepting web uploads/stdin; full log inspection uses explicit CLI file/stdin conversion.
- Depending on unmerged `orbit local` work or an orbit-specific file layout.
- Promising support for undocumented log grammars beyond the reported 0.4.26 grammar and the recovery outcomes below.

### 1. Input and Discovery

**User Story:** As a reviewer, I want to open and discover LM Studio server logs, so that I can inspect recorded inference traffic through apsis.

**Acceptance Criteria:**

1. <a name="1.1"></a>Apsis SHALL accept `-a lmstudio` for files and stdin, and SHALL auto-detect LM Studio only when the first 65,536 input bytes contain a header with `Received request: POST to` and an exact endpoint from [2.1](#2.1); body strings resembling another format's markers SHALL NOT override that signature. A filename extension or timestamp-and-level header alone SHALL NOT identify this format; WHEN headers are present but the bounded probe finds no included request, the detection error SHALL suggest `-a lmstudio`.  
2. <a name="1.2"></a>WHERE discovery is enabled, apsis SHALL discover `YYYY-MM/YYYY-MM-DD.N.log` files under `~/.lmstudio/server-logs` by default and SHALL offer an environment setting and CLI option for both `apsis` and `apsis serve`, with CLI precedence and an explicitly empty setting disabling LM Studio discovery; discovered matching files SHALL be interpreted as LM Studio logs even when the auto-detection probe finds no request, so rotated notice-only records remain available.  
3. <a name="1.3"></a>Before discovery reads or identifier-based opening, apsis SHALL resolve file and root symlinks, reject candidate files outside the configured root and list each canonical file once; discovery SHALL NOT descend through directory symlinks, while explicit file and stdin conversion SHALL remain independent of that root.  
4. <a name="1.4"></a>WHEN converting a file or stdin, apsis SHALL render only the supplied records as one log transcript in source order, identifying each request's logical session; shared JSON SHALL retain the same outer session-document structure used for every other source.  
5. <a name="1.5"></a>WHEN opening a listed logical session, apsis SHALL select only that session's request records across the configured root, plus the body-free references defined in [4.2](#4.2).  

### 2. Request Translation and Missing Content

**User Story:** As a tool author, I want recorded requests translated consistently, so that every output describes the same evidence.

**Acceptance Criteria:**

1. <a name="2.1"></a>Apsis SHALL include only `POST` requests whose path exactly equals `/v1/responses`, `/v1/chat/completions`, `/v1/completions`, `/v1/messages`, `/api/v0/chat/completions`, `/api/v0/completions` or `/api/v1/chat`; other methods and paths, including embeddings, model operations, `/v1/responses/input_tokens` and `/v1/messages/count_tokens`, SHALL NOT produce inference requests.  
2. <a name="2.2"></a>For each included request, apsis SHALL preserve its endpoint, recorded model, source location and request boundary, set provider `lmstudio`, and translate recorded conversation content with its supplied roles: `messages` for chat completions, `messages` and separate `system` content for Messages, `input` and `instructions` for Responses, and `prompt` for completions; native chat's string `input` and typed input items SHALL become supplied user context, with `system_prompt` retained separately.  
3. <a name="2.3"></a>WHERE recorded content contains thinking, tool calls or tool results, apsis SHALL expose the corresponding shared entries and recorded linking identifiers; unrecognised content types or unsupported body shapes SHALL retain their parsed payload as labelled generic content and produce a warning rather than silently disappearing.  
4. <a name="2.4"></a>Apsis SHALL preserve a recognised logged response's recorded assistant output, tool activity and usage, including native chat's `output` and `stats`, as a log-level event with the attribution state from [4.1](#4.1); shared JSON SHALL retain unknown response attribution per request, human-readable views SHALL explain that common state once per transcript without claiming responses were absent, and assistant messages resent as request context SHALL remain labelled as supplied context.  
5. <a name="2.5"></a>IF an included request has no logged body, THEN apsis SHALL retain available metadata with `body not logged`; IF a body is malformed, unfinished or too large to parse under the documented parser limit, THEN apsis SHALL retain its metadata with the corresponding state and warning from [6.2](#6.2).  
6. <a name="2.6"></a>WHERE content contains `<Truncated in logs>`, apsis SHALL retain the available text and mark that content as truncated in every renderer; shared JSON SHALL apply no additional renderer length limits to successfully parsed content.  
7. <a name="2.7"></a>WHERE a request records `previous_response_id` or another reference to server-held context, apsis SHALL preserve that reference and mark context absent from the supplied log as unavailable, without reconstructing it or treating the reference alone as a session or request-attribution key.  
8. <a name="2.8"></a>Tool links and rendering groups SHALL remain within one request occurrence or one logged-response event, even when separate clients or repeated snapshots reuse a tool identifier; apsis SHALL preserve original identifiers separately, retain unmatched results, and compare original recorded identifiers for history-prefix equality.  
9. <a name="2.9"></a>For each parsed included request, shared JSON SHALL retain remaining top-level body fields as request parameters, including tools, stream/store, sampling/reasoning settings, cache keys, integration headers and complete client metadata; Markdown and HTML SHALL expose the same parameters in a labelled non-conversation details block, summarising stream state and offered tool names where recorded. Values and truncation markers SHALL remain as logged without field-specific redaction or invented defaults.  

### 3. Session Identity and Repeated History

**User Story:** As a reviewer, I want stable sessions and explicit history boundaries, so that repeated context and compaction do not appear as new conversations.

**Acceptance Criteria:**

1. <a name="3.1"></a>WHERE a parsed request contains a nonempty string `client_metadata.thread_id`, apsis SHALL group exact matching values within the configured root into one logical session across daily and numbered files, including side/compaction requests carrying that value; model changes, endpoint changes and shorter histories SHALL NOT split it or establish that every request is a dialogue turn.  
2. <a name="3.2"></a>WHERE that thread identifier is absent, empty or invalid, apsis SHALL create a singleton logical session for the request; matching model names, prompt text, history prefixes, `session_id` or `prompt_cache_key` SHALL NOT establish shared identity.  
3. <a name="3.3"></a>Every request and listed logical session SHALL have a deterministic identifier; listed LM Studio identifiers SHALL have an `lmstudio-` namespace, contain at most 256 ASCII letters, digits, `_` or `-`, and remain unchanged for surviving requests when files are appended, earlier or later files are added, or other files are removed, while the root, contributing file names and recorded identity remain unchanged.  
4. <a name="3.4"></a>For unchanged input, direct conversion of a file or stdin SHALL produce reproducible request and logical-session identifiers; for a canonical file inside the configured root, those request/membership identifiers SHALL equal discovery's identifiers without reading other log bodies. Outside that root, and for stdin, they SHALL be labelled input-scoped; the outer direct-log document's identifier SHALL remain file/input-scoped rather than being claimed as a listed logical-session identifier.  
5. <a name="3.5"></a>Apsis SHALL retain every request boundary and its complete parsed input sequence in shared JSON; WHEN consecutive requests in the same logical session repeat an identical leading sequence of logged items, human-readable outputs SHALL reference the earlier prefix and show the changed suffix, preserving repetitions outside that prefix.  
6. <a name="3.6"></a>WHEN a request is not an extension of its predecessor's logged input, apsis SHALL mark a context discontinuity without changing thread identity or deleting either request's recorded input; identical and shorter requests SHALL still retain their request boundaries.  
7. <a name="3.7"></a>Apsis SHALL preserve `client_metadata.thread_id` separately as correlation metadata in transcript and listing output, while retaining separate server and harness sessions and leaving the LM Studio project unknown; parent-thread identifiers and a completely recorded request-kind value SHALL remain request metadata, while a truncated/unparseable metadata blob SHALL remain raw marked content without an inferred request kind.  

For [3.5](#3.5), logged-item equality includes supplied instructions, role, content, type and recorded identifiers; object-key order is irrelevant, but array order and string contents are significant. Equal truncated representations permit only a reference to the same logged representation, never a claim that the original complete strings matched. Prefix comparison does not cross singleton-session boundaries or suppress a separately logged response.

### 4. Attribution and Log-Level Events

**User Story:** As a reviewer, I want uncertainty visible, so that concurrent requests do not acquire each other's responses or warnings.

**Acceptance Criteria:**

1. <a name="4.1"></a>For the supported disk-log grammar, apsis SHALL keep response, WARN and ERROR request attribution unknown because no evidence-backed request-scoped correlation field is established; thread identifiers, `response_id`, `previous_response_id`, adjacency, model tags, timestamps, apparent serial execution and file boundaries SHALL NOT establish request ownership.  
2. <a name="4.2"></a>A direct log transcript SHALL retain logged response, WARN and ERROR content with `request attribution unknown`, event identity and source position; a logical-session view SHALL expose only body-free counts, root-relative source locations and CLI file-inspection instructions for contributing files and their next discovered files, without labelling those events as candidates for that session. Browser references SHALL state that full inspection requires CLI access to the server's log files and distinguish forced file conversion from source-selected session opening; they SHALL NOT accept arbitrary file paths as web routes.  
3. <a name="4.3"></a>Apsis SHALL keep attribution unknown across rotation, including for a response before the first request in a file; the end of a file or arrival of another request SHALL NOT imply completion or ownership.  
4. <a name="4.4"></a>WARN and ERROR retention under [4.2](#4.2) SHALL take precedence over routine-category suppression, including startup, model-management and excluded-endpoint diagnostics; apsis SHALL ignore routine records at other levels that carry no included conversation content, while an unrecognised record claiming to contain an included request or logged response SHALL produce a parse warning.  
5. <a name="4.5"></a>For the same supported view scope, Markdown, standalone HTML, web HTML and shared JSON SHALL expose the same recorded content, request boundaries, context provenance, truncation, attribution and incomplete states, using existing message-metadata, notice and error presentation patterns where available.  
6. <a name="4.6"></a>WHEN a logged POST path is neither an included endpoint nor a recognised embedding, token-counting or model-management endpoint, apsis SHALL retain its endpoint/source-position metadata as unsupported, without creating an inference session, and emit one body-free warning per path with a count; this includes `/v1/decisions`. Future unknown POST paths SHALL NOT vanish silently, while known non-inference requests remain excluded.  

### 5. Timestamps and Ordering

**User Story:** As a reviewer, I want reproducible ordering and declared time assumptions, so that time-zone interpretation does not change the evidence.

**Acceptance Criteria:**

1. <a name="5.1"></a>Apsis SHALL preserve each logged wall time and interpret it in the zone selected by `TZ` or, when unset, the host's local zone; explicitly empty `TZ` SHALL select UTC, and all outputs SHALL identify the zone and state that it is an assumption because the log records no zone.  
2. <a name="5.2"></a>WHERE a wall time has exactly one interpretation in the selected zone, apsis SHALL expose that assumed timestamp with an explicit UTC offset; WHERE it is invalid, nonexistent or ambiguous, apsis SHALL retain the wall time, omit an absolute timestamp and expose the uncertainty.  
3. <a name="5.3"></a>Apsis SHALL preserve source order regardless of wall-time validity, backward clock changes or repeated daylight-saving hours.  
4. <a name="5.4"></a>A logical session's creation time SHALL derive only from its first request under [5.2](#5.2); WHERE that request has no unique interpreted time, creation time SHALL remain absent and listing and `latest` SHALL follow the sibling spec's unknown-time policy without substituting filesystem or current-clock time.  
5. <a name="5.5"></a>Apsis SHALL support UTC and standard IANA zones on every supported release platform even without a system zone database, and SHALL accept native absolute paths to readable TZif files; every accepted nonempty form SHALL allow an optional leading `:`. Unix absolute paths start with `/`, while Windows paths use native drive/UNC forms. IF an explicit override is invalid or cannot be loaded, THEN apsis SHALL report a visible zone warning, retain wall times and omit interpreted absolute times without silently selecting a different zone or relying on whether the host's local-zone API honours `TZ`.  

### 6. Partial Input and Recovery

**User Story:** As a reviewer, I want partial evidence retained with useful diagnostics, so that a damaged or growing log does not hide other requests.

**Acceptance Criteria:**

1. <a name="6.1"></a>WHEN a final body is incomplete at end of file, apsis SHALL mark it incomplete without concatenating bytes from another file; WHEN the original file later grows into a complete record, the next successful refresh SHALL replace its incomplete state without changing its request identity.  
2. <a name="6.2"></a>IF an included record cannot be translated, THEN apsis SHALL retain available request or event metadata, report its file or stdin label and physical start/end line numbers without body text, and recover at the next recognised header; diagnostics SHALL appear on CLI stderr, in shared JSON warnings and in web transcript warnings.  
3. <a name="6.3"></a>WHEN forced parsing encounters non-empty text before the first recognised header, malformed header-like text outside a recognised body, or a non-empty input with no recognised records, apsis SHALL report the affected source range without inventing a request or timestamp, and SHALL retain any later recoverable records.  
4. <a name="6.4"></a>IF a record exceeds the documented parser byte limit, THEN apsis SHALL report an oversized-record state and recover at the next header; the limit and measured record size SHALL be discoverable from the diagnostic or help, and later valid records SHALL remain readable.  

### 7. Listings, Refresh and Exposure

**User Story:** As a user of `apsis serve`, I want predictable discovery without prompt previews, so that I can browse server logs alongside other sources.

**Acceptance Criteria:**

1. <a name="7.1"></a>LM Studio sessions SHALL appear and open by identifier only in the sibling spec's explicit all-projects scope, through CLI source selection or the web source/identifier route, including from an unrelated working directory; single-project listings SHALL read no LM Studio log bodies. Every browser instruction for reopening a logical session, including oversized-session errors, SHALL include all-projects scope and source `lmstudio`; direct-log inspection SHALL instead identify the file and forced format.  
2. <a name="7.2"></a>Listings SHALL contain metadata only: source, identifier, request count, recorded or assumed time state, first known model, provider `lmstudio`, correlation identifier and source warnings where available, with project omitted; size SHALL count only that session's request records, excluding unrelated records and body-free references.  
3. <a name="7.3"></a>Within one `apsis serve` process, refreshes of an unchanged log tree SHALL read zero log-body bytes after the initial listing; added, appended, replaced, truncated or removed files SHALL affect the next successful refresh without renaming unaffected requests or sessions, and a session with no surviving requests SHALL disappear.  
4. <a name="7.4"></a>IF the default log root is absent or reading is disabled, THEN apsis SHALL return no LM Studio sessions without a warning; IF an explicitly selected root is absent, a candidate is unreadable or a non-empty discovered file has no recognised log records, THEN apsis SHALL expose a source warning while retaining results from other readable files and sources.  
5. <a name="7.5"></a>Help SHALL state that server logs can hold full prompts, request parameters including credential-bearing integration headers, and traffic from every client of the LM Studio server; WHERE LM Studio discovery is enabled, the existing `apsis serve` network warning for `0.0.0.0` SHALL include this disclosure without logged content, and the localhost default SHALL remain unchanged.  
6. <a name="7.6"></a>Help SHALL state that singleton listing rows grow one-for-one with unthreaded requests and that enabled LM Studio discovery participates in all-projects `latest`, which can select an unsupported follow/raw-JSON source; the error SHALL identify the selected source and explain disabling LM Studio discovery or explicitly opening another session, without silently choosing an older session.  

### 8. Compatibility and Verification

**User Story:** As a maintainer, I want explicit unsupported outcomes and meaningful checks, so that this parser can ship independently upstream.

**Acceptance Criteria:**

1. <a name="8.1"></a>IF LM Studio input or a selected LM Studio session is combined with raw `-f json` or `-F`, including selection through `latest`, THEN apsis SHALL exit unsuccessfully naming the unsupported combination; the raw JSON error SHALL direct users to shared JSON, and existing sources' behaviour SHALL remain unchanged.  
2. <a name="8.2"></a>Synthetic fixtures SHALL verify every endpoint in [2.1](#2.1), both native chat input forms, unsupported `/v1/decisions`/future paths versus excluded non-inference paths, parameter retention and shared-schema conformance; logical-session views SHALL have content/state parity across Markdown, standalone HTML, web HTML and shared JSON, while direct file/stdin inputs containing multiple sessions SHALL have parity across CLI Markdown, standalone HTML and shared JSON.  
3. <a name="8.3"></a>Verification SHALL cover appends, older-file import, deletion/replacement/truncation, symlink boundaries, direct/discovered identity round trips, empty/invalid identifiers, colliding tool IDs and unmatched results, repeated/compacted/side-request context, concurrent/rotated unattributed events, retained WARN/ERROR versus ignored routine records, missing/truncated/malformed/oversized bodies, pre-header and notice-only excerpts, Kiro-marker detection collisions, ambiguous/nonexistent wall times, portable valid/invalid/unavailable/empty zone overrides without system zone data, visible warnings, scoped browser CLI instructions and LM Studio `latest` mode errors.  
4. <a name="8.4"></a>Fixtures SHALL contain synthetic content, identifiers, paths and model names only; provenance SHALL distinguish reported 0.4.26 log structure from publicly documented endpoint payloads, including `/v1/messages` and `/api/v0/completions`, whose disk-log forms were not established by the supplied inspection reports.  
5. <a name="8.5"></a>Cold-discovery and unchanged-refresh benchmarks SHALL record generated input bytes, request counts, listing-row counts, rendered list-fragment bytes, environment and elapsed time, including a generated large unthreaded batch; they SHALL verify [7.3](#7.3)'s zero-body-read outcome without asserting a hardware-independent cold-time threshold.  

## Public References

- [OpenAI-compatible endpoints](https://lmstudio.ai/docs/developer/openai-compat) and [Responses](https://lmstudio.ai/docs/developer/openai-compat/responses) establish compatible endpoint payloads.
- [Anthropic-compatible endpoints](https://lmstudio.ai/docs/developer/anthropic-compat) documents `POST /v1/messages`.
- [REST API v0](https://lmstudio.ai/docs/developer/rest/endpoints) documents both v0 completion endpoints.
- [Native chat](https://lmstudio.ai/docs/developer/rest/chat) documents `system_prompt`, typed `input`, `output` and `stats`; [stateful chats](https://lmstudio.ai/docs/developer/rest/stateful-chats) explains context referenced by `previous_response_id`.
- [`lms log stream`](https://lmstudio.ai/docs/cli/serve/log-stream) is a separate live interface and does not establish this disk-log grammar.
- [Go's Unix local-zone initialization](https://go.dev/src/time/zoneinfo_unix.go) distinguishes unset/empty `TZ` and silently falls back to UTC after a failed load; LM Studio conversion must validate explicit overrides rather than infer success from that fallback.
- [Go's embedded zone database](https://pkg.go.dev/time/tzdata) establishes a standard-library path to IANA-zone availability without a system database; exact integration belongs to design.
- [Decisions](https://lmstudio.ai/docs/developer/openai-compat/decisions) documents the separate typed-question endpoint, which this slice exposes as unsupported rather than silently dropping.
- Codex's pinned [response metadata implementation](https://raw.githubusercontent.com/openai/codex/rust-v0.166.0/codex-rs/core/src/responses_metadata.rs) establishes client thread/parent metadata and request-kind provenance; it does not make a thread ID request-scoped attribution evidence.
