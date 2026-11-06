# Decision Log: Apsis — Shared JSON Output, Codex Exec Streams and Cross-Project Listing

## Quick Decisions

| ID | Date | Decision | Rationale |
|----|------|----------|-----------|
| Q1 | 2026-11-04 | Name the feature `apsis-codex-exec-lmstudio` | superseded by Q10 |
| Q2 | 2026-11-04 | Use the full spec lane | The feature adds a CLI output format and a versioned JSON schema that other tools depend on, which is expensive to reverse |
| Q3 | 2026-11-04 | Provider and locality come only from what a source records; otherwise they are omitted | Matches the message-metadata convention of omitting unavailable fields; guessing from model names would mislabel sessions |
| Q4 | 2026-11-04 | LM Studio sessions appear only in all-project listings | moved to `apsis-lmstudio-logs` per Decision 5 |
| Q5 | 2026-11-04 | Correlating LM Studio log entries with the Codex or Claude Code session they served is left to consumers | moved to `apsis-lmstudio-logs` per Decision 5 |
| Q6 | 2026-11-04 | JSON output is not cut to the renderer's 2000/3000-character tool limits | superseded by Q29 |
| Q7 | 2026-11-04 | Fixtures are anonymised captures from the inspected versions; documented types never seen in captures get fixtures built from the documented shape, labelled as such | superseded by Q22 |
| Q8 | 2026-11-04 | Default local providers are `lmstudio` and `ollama` | superseded by Decision 6 |
| Q9 | 2026-11-04 | Each new setting is an environment variable and a command-line option, the option winning | apsis has no config file; options suit one-off runs and tests, environment variables suit `apsis serve` and tools that call apsis |
| Q10 | 2026-11-05 | Rename the feature to `apsis-shared-json` | Chosen by the user after LM Studio moved out; it names the main outcome, one JSON format across all harnesses |
| Q11 | 2026-11-05 | Unknown exec event and item types render as generic events, unlike the Codex rollout parser, which warns and skips | The exec format changes between Codex versions; JSON consumers should still see every event |
| Q12 | 2026-11-05 | An exec stream belongs to the git repository or worktree that contains it; outside any repository it appears only across projects | Worktrees list as their own projects elsewhere in apsis; plain path containment would credit worktree streams to the parent |
| Q13 | 2026-11-05 | The schema version is one integer, raised only when a field is removed or changes meaning; consumers ignore fields they do not know | Additive changes stay compatible without a version bump, so one number is enough |
| Q14 | 2026-11-05 | Follow mode rejects the new format value and exec streams with an error | Follow mode parses lines as Claude entries and supports Markdown only; live JSON output has no consumer yet |
| Q15 | 2026-11-05 | A `codex exec` run that also wrote a rollout is listed once, under `codex` | The rollout records the model and provider the stream lacks; rollout IDs equal the stream's `thread_id` |
| Q16 | 2026-11-05 | Exec-stream identifiers derive from `thread_id`, kept unique when several files share one | Copies of one stream exist in practice (a tool copying a run into an evaluation folder); identifiers must be safe in `apsis <id>` and URLs |
| Q17 | 2026-11-05 | Correct the aggregate log-volume evidence through Decision 7 and the handoff's stale metadata claims | Checked size aggregates cover nine months; current Copilot and Kiro converters preserve available timestamps/models |
| Q18 | 2026-11-05 | Exec resume can reuse a thread identifier | The pinned 0.166.0 resume path sends that identifier and the JSONL emitter emits it; distinct files need distinct artifact identifiers |
| Q19 | 2026-11-05 | Keep completion usage as reported totals without summing snapshots or deriving turn deltas | The pinned JSONL emitter copies `ThreadTokenUsage.total`, despite per-turn comments on the event type |
| Q20 | 2026-11-05 | Resolve lifecycle state per turn/item and retain first-appearance ordering | Interleaved tools and incomplete items need deterministic relationships without displaying every update |
| Q21 | 2026-11-05 | Use explicit source selection distinct from forced format while preserving legacy bare IDs | Source lookup and input parsing are separate operations; identical IDs can occur in different sources |
| Q22 | 2026-11-05 | Use synthetic fixtures with pinned authoritative shape provenance; unverified types remain generic | Supersedes Q7's capture requirement; the short public guide alone does not define every payload |
| Q23 | 2026-11-05 | Force parsing for failures before thread creation; list only streams with a valid thread ID | Available errors remain convertible without inventing a discoverable identity |
| Q24 | 2026-11-05 | Keep raw exec JSON as source events, expose JSON listings through CLI shared format, and preserve settings defaults | Matches existing JSONL output and avoids changing serve's HTML fragment endpoint |
| Q25 | 2026-11-05 | Human approved global rollout preference with the rollout's recorded project; direct stream-file conversion remains available | Q1 answer: a copied stream must not change the canonical source or project between listing scopes; confirms requirements 2.7 and Decision 9's scope policy |
| Q26 | 2026-11-05 | Human approved labelled file modification time for sorting and latest, with source creation time and message timestamps absent when unrecorded | Q2 answer: retain useful ordering while distinguishing filesystem metadata from source evidence; confirms requirements 2.10, 4.3-4.4 and Decision 10's fallback policy |
| Q27 | 2026-11-05 | Human approved skipping .git, node_modules and testdata during recursive descent while explicitly configured roots remain scanned | Q3 answer: avoid internals, dependencies and parser fixtures without preventing an explicitly selected root or direct file conversion; confirms requirement 2.5 and Decision 11's skip policy |
| Q28 | 2026-11-05 | Human approved requirements revision 3 after answering Q1-Q3; Decisions 8-11 are accepted | Explicit whole-document approval advances the feature to independent design drafts |
| Q29 | 2026-11-05 | Shared JSON emits tool content exactly as parsed, with no length limit | Replaces Q6's rationale: `MaxToolInputRunes`, `MaxToolResultRunes` and `truncateString` are defined in `markdown.go` but only tests call them, so no production renderer truncates today; the projection must not add a limit either (1.5) |
| Q30 | 2026-11-05 | The new format value is `-f shared-json` | Names the outcome and the spec; sits beside raw `-f json` without implying a harness |
| Q31 | 2026-11-05 | `--all-projects` is a long-only flag on `apsis` and `apsis serve`, with no environment variable, and is a usage error with `-p`/`--project` | It selects a scope per invocation like serve's `--project`, which also has no variable; Q9's flag-plus-environment rule covers settings such as exec roots |
| Q32 | 2026-11-05 | Source selection is `-s`/`--source <source>` using source names (`claude`, `codex`, `codex-exec`, `copilot`, `kiro-cli`, `kiro-ide`); it is a usage error with stdin, `--list` or `latest` | Distinct from `-a`, which forces a parser and uses different names (Q21); rejecting other combinations avoids an implicit filter meaning; `-s` is unused |
| Q33 | 2026-11-05 | Repeatable `--codex-exec-dir` replaces `APSIS_CODEX_EXEC_DIRS` entirely; the variable is split with `filepath.SplitList`; one empty flag value means no roots; mixing empty and nonempty values is a usage error | Repeated flags need no escaping; the OS path-list separator is the platform convention (5.4); an empty flag lets one run override a set variable |
| Q34 | 2026-11-05 | The schema lives at `docs/schemas/apsis-shared-json.v1.schema.json` with `$id` set to its raw URL on upstream `main` | `docs/` already holds published project documents; the version in the name lets a future version 2 file sit beside it |
| Q35 | 2026-11-05 | Document types and projection live in a new `internal/sharedjson` package | Listing projection needs `sessions` types and `sessions` imports `transcript`, so the projection cannot live in `transcript` without a cycle |
| Q36 | 2026-11-05 | Session metadata sits in a `session` object whose schema definition listing rows reuse, with `project` and `size_bytes` names | One `$defs` entry serves both documents (1.2, 4.3); the unit in `size_bytes` avoids ambiguity |
| Q37 | 2026-11-05 | Shared JSON uses `created_at` only for recorded times and `file_modified_at` for the filesystem fallback, for every source; text and web listings label the fallback only on `codex-exec` rows | Q26 requires a marked fallback; every existing fallback is a modification time, so one named field is enough; 5.1 forbids changing existing sources' text and web output |
| Q38 | 2026-11-05 | Version-1 entries carry `role`, `timestamp`, `model`, `provider`, `turn` and `content`; Claude wire fields (`uuid`, `parent_uuid`, `isMeta`, `toolUseResult`) stay out | 1.1 needs none of them; Q13 lets them be added later without a version change |
| Q39 | 2026-11-05 | Warning objects carry `code`, `message`, and `line` (physical line) or `chat_message` (Kiro IDE chat index); listing warnings add `source` | Superseded by Q75: the blanket Kiro IDE chat-index assumption omitted execution-detail action positions; original proposed field meanings remain recorded here |
| Q40 | 2026-11-05 | The provider renders as `model (provider)`, or `(provider)` without a model, inside the existing assistant meta | Binds the provider to the model it served without adding a meta part to existing headers |
| Q41 | 2026-11-05 | Usage blocks use basis `thread_total_at_turn_completion` and keep counters as raw JSON values in source key order | Labels the 0.166.0 total-counter semantics (2.9, Q19) and keeps unknown or non-integer counters representable |
| Q42 | 2026-11-05 | Exec tool names are the item types; every exec tool item emits a `tool_result`; tool IDs are `t<turn>:<item id>` with the original kept as `item_id` | Matches the rollout parser's use of source names; non-Task tools render only with their result; the 0.166.0 emitter restarts `item_N` numbering per process, so appended resumed output reuses IDs that transcript-wide tool maps would conflate |
| Q43 | 2026-11-05 | Under exec roots, `.jsonl` files whose first parseable object is not an exec event are ignored silently; exec-looking files without a valid `thread_id` are omitted with a warning | Roots can hold other JSONL; warning on every unrelated file would bury real problems (2.11) |
| Q44 | 2026-11-05 | A later `thread.started` with the same ID is dropped; one with a different ID becomes a generic event plus a `thread_mismatch` warning | Resumed output appended to one file reuses the ID (Q18); nothing recorded is lost and the file's identity does not change |
| Q45 | 2026-11-05 | Malformed-line warnings use fixed text plus the JSON syntax error's byte offset | 5.2 forbids copying the line; `encoding/json` error messages can quote source characters |
| Q46 | 2026-11-05 | The web `codex-exec` filter pill appears only when exec roots are configured | Keeps the unconfigured web page unchanged (5.1) |
| Q47 | 2026-11-05 | The web transcript view shows parse warnings for `codex-exec` sessions only | 5.2 requires them for exec streams; 5.1 keeps other sources' web output unchanged; widening this needs a requirements change |
| Q48 | 2026-11-05 | The exec parser discards lines over 10 MiB with an `oversized_line` warning; raw `-f json` keeps its existing line handling apart from stripping a leading BOM for exec input | Existing scanners stop at 10 MiB; changing the shared raw path for other sources would alter existing output (5.1), while a BOM-prefixed exec stream would otherwise lose `thread.started` (5.3) |
| Q49 | 2026-11-05 | In all-projects scope a `(source, id)` pair lists once, keeping the resolver's candidate, with a `duplicate_session` warning; project-scope listings keep current behaviour | Cross-directory duplicates appear in all-projects scope, where a second row would open the first row's content (4.2); project scope stays unchanged (5.1) |
| Q50 | 2026-11-05 | Equal times break by the existing source priority with `codex-exec` last, then by identifier | superseded by Q60 |
| Q51 | 2026-11-05 | Kiro CLI all-projects opening uses a new `GetSessionByID` ordered by `updated_at DESC, key ASC`; `DiscoverAll` gains the same secondary order | `GetSession` re-normalises its directory, so a stored key that no longer normalises to itself misses; one shared order makes listing and opening pick the same row |
| Q52 | 2026-11-05 | Repository roots are found from `.git` directories or `gitdir:` files without invoking `git` | Apsis has no external CLI dependency today |
| Q53 | 2026-11-05 | Listing `model`/`provider` come only from the bounded extraction reads; the session document is authoritative | superseded by Q57 |
| Q54 | 2026-11-05 | The web listing warning banner moves into the polled `sessions_list` fragment | Warnings from roots and files must follow each refresh (5.9); the page includes the fragment, so the banner is not duplicated |
| Q55 | 2026-11-05 | Agent proposal: specialised exec items require a nonempty string source ID; known shapes without one become generic events with item_decode_failed and no tool links | The pinned ThreadItem definition requires id; typed Go decoding alone does not enforce presence, and two missing IDs would otherwise share t1: across renderer maps |
| Q56 | 2026-11-05 | Agent proposal: only the probe's exec outcome selects FormatCodexExec automatically; exec-without-thread stays forced-only, while discovery warns and omits it | Makes detection and missing-thread outcomes explicit without widening approved 2.1 |
| Q57 | 2026-11-05 | Agent proposal: preserve existing timestamp/cwd scans for existing sources; bound only newly extracted optional Claude cwd metadata | Caching must not replace a recorded Claude timestamp after line 50 or a long Codex header with filesystem time, which changes default listings/latest against 5.1; session documents remain authoritative |
| Q58 | 2026-11-05 | Shared-design critique and peer review used fresh non-author contexts on this harness/model; neither is independent-model evidence from the other | Both reviewed the completed 18:21Z Claude merge without reading each other; the reviewers differ from the merge author, and no human approval was supplied |
| Q59 | 2026-11-05 | Preserve the completed 18:21Z merged design, decisions and questions as drafts/design/*merged-r1.md before revision | Retains the exact reviewed inputs and concurrent contributions; all code and the canonical task file remain frozen |
| Q60 | 2026-11-05 | Agent proposal: deterministic source/identifier tie ordering is enabled for shared JSON, all-projects scope or configured exec roots; no-flag project listings/latest retain legacy ordering | The explain-like self-check exposed that applying the new tie rule unconditionally could change default output against 5.1 |
| Q61 | 2026-11-05 | Agent proposal: Lister and Resolver apply the same Option functions to a shared Options value rather than a Lister-specific closure | The design's earlier Option func(*Lister) cannot directly configure a Resolver; one settings type keeps root precedence consistent without coupling their runtime state |
| Q62 | 2026-11-05 | Clarify Decision 22's listing scope: the global selected winner governs suppression in every scope and all-projects row deduplication; project-scope rollout rows keep existing per-file cwd filtering under Q49 | The final revision check found ambiguous global-listing wording that could remove a later duplicate from its legacy project list, violating 5.1; this does not change human-approved global stream suppression |
| Q63 | 2026-11-05 | Correct the freeze attribution: it was the dispatching coordinator's ownership instruction, not a human decision | Checked 19:00Z note and current SPEC-DRIVE authorisation permit reviewed local planning and implementation; preserve human decisions and existing owners, and never infer human approval |
| Q64 | 2026-11-05 | Keep current Claude-model and Kiro-IDE-provider omission inside approved scope | U1/U2 remain unanswered possible scope expansions, not prerequisites or answered choices |
| Q65 | 2026-11-05 | Admit the test-only schema validator as a reversible local technical choice | Both design drafts and non-author reviews support it; U3 remains unanswered and no human approval is implied |
| Q66 | 2026-11-05 | Publish the complete known v1 schema initially, with an unknown-block extension branch (Decision 25) | Initial-schema validation must keep accepting later producers; a mutable upstream $id cannot update a cached validator |
| Q67 | 2026-11-05 | Isolate metadata fingerprints in internal/fileidentity (Decision 26) | Separate native tests and disjoint package ownership let the cache capability gate run without blocking sessions work |
| Q68 | 2026-11-05 | Four PR slices and three streams; module writes serialized after task 3.2 (Decision 27) | Native Windows blocks cache only, and go.mod/go.sum have one writer at a time |
| Q69 | 2026-11-05 | Record reproducible synthetic benchmark dimensions and cold/warm measurements; 1.5 s is only an investigation budget | No evidence supports a host-independent timing acceptance test; zero unchanged body reads stays mandatory |
| Q70 | 2026-11-05 | Report Kiro CLI's actual matched database key in project scope and inject all-project discovery too | Current GetSession returns only a reader; listKiro's all-projects path bypasses its discover injection, so both need explicit coverage |
| Q71 | 2026-11-05 | Shared JSON alone enables ParseOptions.AllowEmptyCodex through backward-compatible parser options | Header-only Codex currently errors; retain legacy acceptance/errors while the new format preserves header metadata with entries: [] |
| Q72 | 2026-11-05 | Inject the path-digest helper only in tests to exercise identifier conflicts | Synthetic fixtures cannot generate a real SHA-256 collision; production identity remains unchanged |
| Q73 | 2026-11-05 | Preserve original canonical numbered/stable IDs 1–11, statuses and owners while mapping their scopes into the reviewed plan | Byte-exact pre-merge tasks and the mapping in drafts/tasks retain provenance; task 1 remains InProgress under codex-gpt-6 until its separately verified completion |
| Q74 | 2026-11-05 | ParseResolved accepts caller ParseOptions; overlay source-bound Kiro cost data without losing Codex flags, and preserve explicit CLI forced-format precedence | Current peer major 1 found that a no-options helper would undo shared-json-only acceptance; CLI enables AllowEmptyCodex/PreserveModelChanges only for shared JSON, web uses defaults |
| Q75 | 2026-11-05 | Supersede Q39: warning positions are line for physical lines, chat_message for Unit chat-message, and execution_action for Unit execution-action | Checked both Kiro IDE warning creation sites: unknown roles use chat indexes and unknown execution actions use action indexes; keep their numeric Line and existing stderr messages unchanged |
| Q76 | 2026-11-05 | Shared-json-only PreserveModelChanges reconciles model-change retention with default consolidation; identical metadata never splits, and provider transitions may split under 3.1 (Decision 28) | Existing ensureAssistantEntry does not split model-only changes; one explicit parser option preserves approved 1.6 in the new format without changing provider-less legacy output under 5.1 |
| Q77 | 2026-11-05 | Codex session creation time comes from outer CodexEntry.Timestamp on session_meta, never payload.timestamp | Matches the current listing extractor; a missing outer timestamp remains unavailable parsed evidence, rather than acquiring a new fallback |
| Q78 | 2026-11-05 | Preserve exact reviewed task/design/decision/question/handoff/prerequisite inputs as drafts/tasks/*revised-r1.md before revision | Retains the reviewed state including separately verified task-1 completion under its original owner; no task identities/statuses/owners change during revision |
| Q79 | 2026-11-05 | Weigh current fresh non-author same-model critique and peer review; schedule a non-author follow-up for option forwarding, warning positions and compatibility-option parity | Critique reported no defects; peer reported one major integration gap; same-model fresh contexts are not independent-model evidence or human approval |
| Q80 | 2026-11-05 | Existing parser warnings carry codes set where they are created: `malformed_line` for a JSON decode failure on a physical line, `oversized_line` for the Claude buffer limit, `invalid_record` for a decodable record missing a required field or with an undecodable payload, `unrecognized_record` for an unknown record, event, item, role, content or action type, and `unmatched_tool_result` for a Codex output without its call; stderr text is unchanged | Every projected warning carries `code`, and no default existed for existing sources (independent peer review M2) |
| Q81 | 2026-11-05 | Codex `StartedAt` takes the first `session_meta` outer timestamp that parses as RFC3339 or RFC3339Nano and keeps scanning otherwise | Matches the listing extractor's accepted values, so a session document and its listing row cannot disagree on an unparseable header (peer m2) |
| Q82 | 2026-11-05 | A Codex metadata transition starts a new assistant entry only at a model-produced item (message, reasoning, function call, agent message or reasoning); a function call output joins the current entry | Keeps a tool result in the same entry as its call (peer m4) |
| Q83 | 2026-11-05 | Shared JSON listings keep extended ordering in every scope; the README states that no-flag `apsis latest` keeps the legacy tie order, and task 6.1 tests the documented difference | Changing no-flag `latest` would break 5.1, and selecting by output format would surprise users (peer m3) |
| Q84 | 2026-11-05 | Reopen task 2 for one red/green follow-up pair, reset tasks 3/3.1 and 9/9.1 to pending, and release the owners of 3, 3.1, 3.2, 9, 9.1 and 9.2 | Their claims were released at 22:49Z with no output; both now depend on the reopened task 2 (peer m1). Task 2 keeps its recorded owner for provenance; the new subtasks are unowned |
| Q85 | 2026-11-05 | Ask the user U4: should Markdown and HTML assistant headers show the Codex model? Until answered, the Codex model appears only in shared JSON documents and listings | Showing it in headers changes existing output beyond the provider exception in 5.1; the user's direction asks for Markdown/HTML parity, so this is the user's choice (peer M1) |
| Q86 | 2026-11-05 | The user approved requirements revision 4, adding 1.7 (recorded session origin) and 1.8 (recorded entry identifiers) in shared JSON only | Decision 31; design and tasks must absorb both before task 3 publishes the schema |
| Q87 | 2026-11-05 | Session origin is Claude Code's first nonempty `entrypoint` and Codex's first nonempty `session_meta.payload.originator`; no other source field is mapped to origin. Copilot's `session.start` `data.producer` stays unmapped until the user answers U5, and Codex `session_meta.payload.source`/`thread_source` are not used | 1.7 names these two fields. Pinned rust-v0.166.0 `SessionMeta.originator` is a required string, set to `codex_exec` by `codex exec` and defaulting to `codex_cli_rs`. `producer` names the program that wrote the log rather than a client entry point (one value across six local sessions), and Codex `source`/`thread_source` are classifications, which Decision 31 rejects. Adding a mapping later is additive under 1.3 |
| Q88 | 2026-11-05 | Shared JSON names the fields `origin` in the shared session-metadata definition (so listing rows inherit it) and `id` on the entry | 4.3 names origin; the entry `id` sits at entry level, apart from the block-level tool `id` and `tool_use_id`, and is never a tool-linking identifier (1.8) |
| Q89 | 2026-11-05 | Claude's `entrypoint` is decoded by the existing `Entry.UnmarshalJSON` alias into `Entry.Origin` (`json:"-"`), which feeds `ParseResultMetadata.Origin` | A second decode per line would cost every Markdown/HTML render of transcripts without `entrypoint`; raw `-f json` passes source lines through unchanged and never re-serialises `Entry` |
| Q90 | 2026-11-05 | Listing `origin` comes from the existing bounded extraction reads: Claude's first `entrypoint` within the same 50-line/1 MiB head scan as `cwd`, Codex's `originator` from the `session_meta` head scan; the session document is authoritative | Keeps listing reads bounded, as for model and provider; a Claude value beyond the bound is absent from the row only |
| Q91 | 2026-11-05 | Add task pair 2.5/2.6 after 2.4 for parser origin and record identifiers; extend tasks 3, 5, 6 and 10 (titles, details, requirement links) before task 3 publishes the schema; change no status or owner. Exact prior inputs are preserved as `drafts/tasks/*revised-r3.md` | 2.3/2.4 are awaiting a non-author check, so a separate pair keeps that check's scope unchanged; 2.5/2.6 edit the same parser files, so they follow 2.4 |
| Q92 | 2026-11-05 | U1 answered by the user: Claude Code sessions carry no model in this feature; a follow-up amends 5.1 and adds it to every output at once | Keeps the first PR inside approved scope, with session documents and listings consistent |
| Q93 | 2026-11-05 | U2 answered by the user: preserve Kiro IDE's `metadata.modelProvider` and show it beside the model in Markdown and HTML as well as shared JSON; requirements revision 5 amends 3.1 and 5.1 | The user chose rendered parity now over keeping existing Kiro IDE output unchanged; design and tasks must add the extraction and header change |
| Q94 | 2026-11-05 | U3 answered by the user: add `github.com/santhosh-tekuri/jsonschema/v6`, imported only from `_test.go` files | A maintained Draft 2020-12 validator checks fixtures; it stays out of the binaries (Decision 25) |
| Q95 | 2026-11-05 | U4 answered by the user: the Codex model appears in shared JSON and listings only; Markdown and HTML headers show the provider without it until a follow-up amends 5.1 | Meets approved 1.6 without changing protected Codex output |
| Q96 | 2026-11-05 | U5 answered by the user: Copilot's `producer` is not mapped to origin | It records the program that wrote the log rather than a client entry point, and does not tell sessions apart |
| Q97 | 2026-11-05 | The user approved design and tasks; PR 4 (unchanged-refresh cache, former tasks 15–16) moves to `tasks-cache.md`, and orbit drives `tasks.md` (PR 1–3) with local commits | `orbit run` re-runs a phase whose tasks cannot complete (orbit `specs/run-termination/`), and task 15.2 needs native Windows evidence; a separate file lets PR 1–3 run unattended and PR 4 run once that evidence exists |
| Q98 | 2026-11-05 | Agent decision (task 3): the closed test copy of the schema adds `unevaluatedProperties: false`, not `additionalProperties: false`, to the apsis-owned definitions | Listing rows and blocks reuse shared definitions through `$ref`, which `additionalProperties` cannot see, so it would reject valid rows; `unevaluatedProperties` closes composed objects correctly |
| Q99 | 2026-11-05 | Agent decision (task 3): `sharedjson.SessionContext` carries `Source`, `ID` and `Resolved` only; the canonical input path joins it with its first consumer, exec-stream project assignment | superseded by Q110 |
| Q100 | 2026-11-05 | Agent decision (task 3): `other` blocks carry `source_type` only when the parsed item records a type | Avoids inventing a value for an untyped item; the field stays defined for every recorded kind |
| Q101 | 2026-11-05 | Agent decision (task 3): the schema lists warning codes as `examples` on `warning.code` rather than an `enum`; tests require every projected code to be in that list | An enum would make codes added later invalid under the version 1 schema, against 1.3 |
| Q102 | 2026-11-05 | Agent decision (task 3): shared JSON emits `created_at` and entry timestamps only when they parse as RFC3339, and writes resolver times in UTC | A malformed recorded time would otherwise fail the schema's `date-time` format; UTC keeps resolver-derived output independent of the host zone |
| Q103 | 2026-11-06 | Agent decision (task 4): the resolver returns the Kiro CLI database `created_at` in a new `SessionMetadata.DatabaseCreatedAt` field, used by shared JSON between the parsed start record and a recorded resolver `CreatedAt`; Kiro CLI `CreatedAt` stays unset | The web transcript view prints `CreatedAt` whenever it is set, so filling it would add a time to every Kiro CLI web transcript, a change 5.1 forbids; whether the web view should show it is open question U6 |
| Q104 | 2026-11-06 | Agent decision (task 6): `apsis --list` writes every format, shared JSON included, to stdout and still ignores `-o` | Writing the text listing to `-o` would change no-flag behaviour, which 5.1 protects; one destination for both listing formats avoids a format-dependent surprise |
| Q105 | 2026-11-06 | Agent decision (task 5): a Codex rollout whose head has no parseable `session_meta` timestamp is a filesystem fallback in the resolver too (`TimeFromFilesystem`), not only in the listing | The removed `getCodexSessionTimestamp` returned the modification time without an error, so PR 1's resolver reported it as a recorded time and shared JSON could emit it as `created_at`, against Q37 |
| Q106 | 2026-11-06 | Agent decision (task 7): each collector de-duplicates beside its own selection rule (Claude: first project directory; Codex: first walk match, `walkCodexRollouts`; Kiro IDE: `bestKiroIDECandidate` across workspaces; Kiro CLI: `DiscoverAll`), still before sorting, rather than in one generic pass after collection | Kiro IDE rows come from map iteration and an invalid Codex winner must block later copies, so a generic keep-first pass would choose the wrong candidate; collectors therefore return their own warnings |
| Q107 | 2026-11-06 | Agent decision (task 7): `selectCodexCandidate` keeps the first walk match even when it lies inside a followed directory symlink | `filepath.SkipAll` stops only the inner walk of a followed symlink, so the old `findCodexSession` could overwrite the first match with a later one; keeping the first matches its documented choice and the listing's walk |
| Q108 | 2026-11-06 | Agent decision (task 8): listing warnings keep the existing `could not list`/`Could not load` wording on stderr and in the web banner for every code, and `-F` with `-s` follows only the selected source, rejecting Kiro CLI and Kiro IDE | New codes appear only across projects; one wording keeps existing output unchanged (5.1). Follow mode already supports only JSONL file-backed sources |
| Q109 | 2026-11-06 | Agent decision (task 8): all-projects web rows and text rows show `-` for an unrecorded project, and `apsisweb.Server.Handler()` exposes the router for `httptest` round trips | One placeholder across surfaces; the accessor lets one fixture set exercise CLI and web listings against the same rows (5.8) |
| Q110 | 2026-11-06 | Agent decision (task 12): `sharedjson.SessionContext` carries `Source`, `ID`, `Project` and `Resolved`; the CLI derives an exec stream file's containing repository as `Project` rather than passing its canonical path | Keeps the projection free of filesystem reads; resolver input supplies the same project through `SessionMetadata.Project` (supersedes Q99) |
| Q111 | 2026-11-06 | Agent decision (task 9): exec events with a missing type are skipped with `invalid_record`; `thread.started` without a thread ID, `turn.completed` without a usage object and `error`/`turn.failed` without a message become generic events with `invalid_record`; an `item.*` event without an item object becomes a generic event with `item_decode_failed` | Nothing recorded is dropped except an untyped object, which has no type to show; codes follow Q80's existing meanings |
| Q112 | 2026-11-06 | Agent decision (task 9): `error` blocks keep the source JSON as `ContentItem.Payload`, but shared JSON emits only `message` | The published version 1 `errorBlock` has no payload field and the closed test copy rejects one; adding it later is additive |
| Q113 | 2026-11-06 | Agent decision (task 12): follow mode rejects files whose probe outcome is `exec`; `exec-without-thread` files keep the legacy follow behaviour | Mirrors Q56: only a valid `thread.started` establishes an exec stream automatically |
| Q114 | 2026-11-06 | Agent decision (task 13): configured exec roots turn on extended ordering inside `sessions` option handling, for both the Lister and the Resolver | One rule for the CLI and the web instead of two call-site conditions (design section 4) |
| Q115 | 2026-11-06 | Agent decision (task 14): a `codex-exec` identifier opens only when exactly one stream under the roots derives it | Listings omit conflicting identifiers (Decision 14), so opening one of them would pick a copy the listing never showed |
| Q116 | 2026-11-06 | Agent decision (task 10): incomplete items show ⏳ and `(incomplete)` in tool, plan and event summaries, after thinking headers and below incomplete text; complete plan and event summaries show their own icon | Exposes 2.3's incomplete state in every new content kind without changing complete items |
| Q117 | 2026-11-06 | Agent decision (task 14): the web `file modified` label sits beside the `<time>` element, and only exec rows change markup | The layout script replaces `<time>` text with a localised date, which would erase a nested label; other rows stay unchanged (5.1) |
| Q118 | 2026-11-06 | Agent decision (task 13): `ProbeExecStream` reads through a 4 KiB buffer | The 10,000-file benchmark allocated about 674 MB per pass with a 64 KiB buffer per probe and about 71 MB with 4 KiB; long lines are still joined by `readExecLine` |
| Q119 | 2026-11-06 | Agent decision (task 14): the apsisweb end-to-end test closes the HTTP client's idle connections before `Shutdown` | `Shutdown` treats a dialled connection that never sent a request as active for 5 s, matching the recorded 5 s timeout; the test failed 4 of 4 package runs here before and passed 6 of 6 after |

---

## Decision 1: Emit the Shared Representation Through a New Format Value

**Date**: 2026-11-04
**Status**: accepted

### Context

`-f json` currently emits raw session data, and existing users may depend on that. Tools that want one view across harnesses have no way to obtain apsis's shared representation, so they re-parse each harness format.

### Decision

Add a new `-f` value for the shared representation, with a schema version and a JSON Schema file in the repository. Leave `-f json` unchanged.

### Rationale

A new value adds capability without breaking anyone. A published, versioned schema lets consumers detect incompatible changes, and validating every format's fixtures against it keeps the parsers honest about what they produce.

### Alternatives Considered

- **Change `-f json` to the shared representation**: One JSON mode - Rejected: breaks existing users of the raw output.
- **A Go package API only**: Consumers import `internal/transcript` - Rejected: `internal` packages cannot be imported from outside the module, and non-Go tools need a CLI contract.
- **Unversioned JSON**: Simpler output - Rejected: consumers could not tell an incompatible change from an additive one.

### Consequences

**Positive:**
- Any tool can read every supported harness through one documented schema.
- Existing `-f json` users are unaffected.

**Negative:**
- The shared representation becomes a public contract; changing it now needs a version decision.
- Two JSON modes need clear help text to avoid confusion.

---

## Decision 2: Codex Exec Streams as a Separate Source

**Date**: 2026-11-04
**Status**: superseded by Decision 3

### Context

`codex exec --json` writes an event stream (`thread.started`, `turn.*`, `item.*`) that differs from the rollout files Codex keeps in `~/.codex/sessions/`. Runs started with `--ephemeral`, as `orbit local` and `local-next-task` do, leave no rollout file, so the stream is the only record. The stream carries no user prompt and no `session_meta`.

### Decision

Parse exec streams as their own format and list them under the source label `codex-exec`. Render orbit's recorded prompt file as the first user message when present.

### Rationale

The formats differ in structure and in what they record, so a separate label tells the reader what they are looking at, including that some fields, such as the prompt, may be absent. Parsing the stream itself, rather than an orbit-specific wrapper, makes the parser useful for any `codex exec --json` output.

### Alternatives Considered

- **Fold exec streams into the `codex` source**: One Codex label - Rejected: hides a different format with different gaps behind the same label.
- **Read orbit's local-run files only, as orbit data**: Smaller scope - Rejected: ties the parser to orbit when the format comes from Codex.

### Consequences

**Positive:**
- Non-interactive Codex runs become reviewable wherever their output was saved.
- Orbit's local runs appear alongside other sessions.

**Negative:**
- Listing needs to know where streams are kept; only orbit's locations are discoverable automatically.
- Codex may change the event format between versions; unknown events render generically rather than failing.

---

## Decision 3: Exec Streams Integrate Through Generic Conventions, Not Orbit's Files

**Date**: 2026-11-04
**Status**: superseded by Decision 4

### Context

Decision 2 rendered orbit's recorded prompt file and listed the streams orbit records. Only the unmerged `orbit local` and `local-next-task` work writes those files (`.orbit/local/` and `.orbit/local-next-task/`, with `<base>.stdout.jsonl` and `<base>.prompt.txt`). Upstream orbit runs `codex exec` without `--json`, so on upstream main there are no orbit exec streams to find. This feature must merge upstream on its own, and the local-execution work must be able to build on it later without being a prerequisite.

### Decision

Keep exec streams as their own format under the source label `codex-exec`. List streams only from configured directories, searched at any depth, assigning a stream to a project when its path lies inside the project directory. Render `<base>.prompt.txt` beside `<base>.jsonl` or `<base>.stdout.jsonl` as the first user message.

### Rationale

Configured directories and a prompt-file convention work for any tool that saves `codex exec --json` output, so the feature stands on upstream main. Orbit's local runs already follow the convention, so pointing the directory setting at a parent of those projects lists them with no apsis code tied to orbit. Assigning projects by path containment needs no metadata the stream lacks.

### Alternatives Considered

- **Discover orbit's `.orbit/local*` directories**: Built-in knowledge of orbit's layout - Rejected: those files exist only with the unmerged local-execution work, which would become a prerequisite.
- **Render exec streams only when given a path**: No listing - Rejected: `apsis serve` and tools that call `apsis --list` could not find them.
- **Scan every project tree for exec streams**: No setting needed - Rejected: unbounded file reads per listing, and cross-project listing has no list of projects to scan.
- **Use orbit's run registry (`~/.orbit/runs`)**: Orbit already tracks runs - Rejected: ties apsis listing to orbit, and it is not known whether the registry records local runs.

### Consequences

**Positive:**
- The feature merges upstream without the local-execution work.
- Exec streams saved by any tool, including orbit's local runs, list and render the same way.

**Negative:**
- Exec streams are not listed until a directory is configured.
- A configured directory with many files makes listing read the first line of each candidate file.

---

## Decision 4: Exec Streams Come From Configured Directories, With No Prompt File

**Date**: 2026-11-05
**Status**: accepted

### Context

Decision 3 listed exec streams from configured directories and rendered a `<base>.prompt.txt` file beside a stream. Nothing on upstream main writes that file; only unmerged local-execution work in a fork does. Reviews also found that path containment credits worktree streams to the parent repository, that file names collide across directories, and that a run without `--ephemeral` also leaves a rollout.

### Decision

List exec streams from configured directories only. Assign each stream to the git repository or worktree that contains it. Identify it by its `thread_id`. List a run once, under `codex`, when a rollout with the same ID exists. Do not read prompt files; the work that writes them can add that on top.

### Rationale

Every rule uses only what the stream records or where it sits, so the feature stands on upstream main and any tool that saves `codex exec --json` output benefits. Leaving the prompt file out keeps apsis from defining a convention with no upstream writer, and adding it later is purely additive.

### Alternatives Considered

- **Keep the prompt-file convention in apsis**: Orbit's local runs would show their prompt on merge - Rejected: upstream maintainers would be asked to own a file name nothing upstream writes.
- **Assign streams by path containment**: Simpler rule - Rejected: worktree streams would list under the parent project.
- **Identify streams by file path**: Always unique - Rejected: paths are unsafe in `apsis <id>` and URLs, and change when files move.

### Consequences

**Positive:**
- No apsis code depends on orbit's file layout.
- A run recorded both ways appears once, with the richer record.

**Negative:**
- Exec streams render without the prompt that started them.
- Exec streams are not listed until a directory is configured.

---

## Decision 5: LM Studio Server Logs Move to Their Own Spec

**Date**: 2026-11-05
**Status**: superseded by Decision 7

### Context

The LM Studio part of this feature carries most of the review findings: an undocumented free-text format, timestamps in local time without a zone, grouping across parallel requests, about 130 MB of logs a month, and full prompts in the logs. It shares only the JSON layout and the all-projects listing with the rest.

### Decision

Move LM Studio server logs to `specs/apsis-lmstudio-logs/`, which builds on this spec and extends the JSON layout additively.

### Rationale

This spec's JSON output, exec streams and listing can be designed, reviewed and merged as small upstream PRs without waiting on LM Studio's open questions. Codex requests in the logs carry `client_metadata.thread_id`, which the sibling spec can use to group requests and link them to exec streams and rollouts listed here.

### Alternatives Considered

- **Keep LM Studio here as the last PR**: One spec - Rejected: approval of everything would wait on the LM Studio questions.
- **Drop LM Studio support**: Smaller scope - Rejected: it is the only record of local-model traffic from clients that keep no transcript.

### Consequences

**Positive:**
- This spec is smaller and can be approved sooner.
- LM Studio gets its own review of its format and privacy questions.

**Negative:**
- Two specs must keep the shared JSON layout consistent.
- The consumer that needs LM Studio data waits for the second spec.

---

## Decision 6: Report the Provider Only, Without a Local Flag

**Date**: 2026-11-05
**Status**: accepted

### Context

The requirements marked a session local when its provider was in a configurable list (Q8). No source records whether a server is local: Codex provider IDs are user-defined, `lmstudio` can point at another host, and Ollama serves cloud models.

### Decision

Record and display the provider exactly as the source records it. Do not mark sessions local or cloud, and add no provider-list setting.

### Rationale

Q3 already limits apsis to what sources record. A local flag would be a guess that the JSON contract then commits to; consumers can apply their own rule to the provider.

### Alternatives Considered

- **Local flag from a configurable provider list**: Ready-made for consumers - Rejected: wrong whenever a provider name does not match where it runs.
- **Infer locality from the base URL**: More accurate - Rejected: Codex rollouts and exec streams do not record the base URL.

### Consequences

**Positive:**
- Every field apsis emits is something a source recorded.
- One fewer setting.

**Negative:**
- Each consumer decides what counts as local.

---

## Decision 7: Retain the Separate LM Studio Spec

**Date**: 2026-11-05
**Status**: accepted

### Context

Decision 5's log-volume evidence described aggregate disk usage as a monthly rate. Aggregate file-size checks establish that the sample spans nine months. The parser still has separate grouping, concurrency, timestamp and exposure questions.

### Decision

Keep the human-approved split: LM Studio server logs belong to `apsis-lmstudio-logs`, depending on this feature's shared JSON and all-projects listing. This supersedes Decision 5 solely to correct its supporting evidence.

### Rationale

The split does not depend on a claimed monthly log volume and keeps upstream delivery independent of the server-log parser.

### Alternatives Considered

- **Recombine the specs:** rejected because unresolved server-log policy would delay shared JSON and exec-stream support.
- **Remove LM Studio work:** rejected because the user requested that input and approved a separate spec.

### Consequences

**Positive:**
- The human's scope decision remains unchanged.
- Active evidence no longer asserts a monthly rate.

**Negative:**
- Both specs must maintain the additive shared contract.
- Consumers needing server logs must wait for the second spec.

---

## Decision 8: Preserve Parsed Evidence and Relationships

**Date**: 2026-11-05
**Status**: accepted

### Context

The common entry model already carries tool linking identifiers, per-entry metadata and some session cost data. A JSON field list omitting these, incomplete states or parse warnings would lose information or hide partial translation.

### Decision

Publish those parsed fields, generic payloads and structured warnings in shared JSON. Preserve turn/item relationships and last known state; retain first-appearance ordering. Summarise model/provider using the first known value while retaining entry-level changes.

### Rationale

Consumers need to distinguish missing evidence from complete output and associate concurrent tool activity. This extends translation without extracting unrelated new token-usage data.

### Alternatives Considered

- **Serialize existing wire-shaped entries unchanged:** rejected because ignored model fields and source-specific tags are not the shared contract.
- **Publish only rendered text and event labels:** rejected because relationships, unknown payloads and completeness information disappear.

### Consequences

**Positive:**
- Consumers can recognise partial parses and recover tool relationships.
- Existing cost information and model changes remain available.

**Negative:**
- The schema and renderers must represent more states.
- Payload retention can increase output size.

---

## Decision 9: Keep Stream Identity Stable Across Listings

**Date**: 2026-11-05
**Status**: accepted

### Context

Copied files and resumed runs can share a thread ID. A rollout records its original project while a copied stream belongs to its containing repository, so filtering before de-duplication can change which source a run appears under.

### Decision

Give each canonical stream file a stable bounded identifier derived from the thread ID, retaining the original thread ID separately. Add explicit CLI source selection. Propose global rollout preference and the rollout's recorded project before project filtering; direct stream conversion remains available.

### Rationale

Adding a file, overlapping roots or changing traversal order must not rename existing artifacts. Global rollout preference makes the richer representation canonical consistently; the scope consequence is Q1 in the approval questions.

### Alternatives Considered

- **Thread ID alone:** rejected because copied and resumed files collide.
- **Traversal-order suffixes:** rejected because adding another file can rename sessions.
- **Content hashes alone:** rejected because appending events changes identifiers.
- **Scope-local rollout preference:** retained as the human's Q1 alternative because it preserves copied-project visibility but changes canonical source between views.

### Consequences

**Positive:**
- Listed source/identifier pairs open unambiguously.
- Duplicate discovery roots do not create extra listings.

**Negative:**
- Moving a stream can change its artifact identifier.
- Under the proposed global policy, a copied project's listing can omit a run owned by a rollout elsewhere.

---

## Decision 10: Separate Source Time From Listing Time

**Date**: 2026-11-05
**Status**: accepted

### Context

Exec events record no timestamps. Existing listers use modification time as a fallback, but copies and appends change that value without establishing session creation time.

### Decision

Omit unknown source creation and entry times. Propose an explicitly marked filesystem modification-time fallback solely for listing order and `latest`; order unknown times first and use source/identifier ties. Stdin has no filesystem fallback.

### Rationale

The fallback remains useful without presenting filesystem evidence as a recorded event time. Q2 asks the human whether this ordering policy is acceptable.

### Alternatives Considered

- **Label modification time as creation:** rejected because it asserts an unsupported source fact.
- **Omit any fallback:** retained as Q2's alternative; undated streams then cannot outrank dated sessions.
- **Use the current clock:** rejected because repeated conversion would invent different times.

### Consequences

**Positive:**
- Missing source metadata remains distinguishable.
- Ordering and latest selection are reproducible for unchanged files.

**Negative:**
- Copies and appends can change fallback ordering.
- Listings need to distinguish source time from filesystem time.

---

## Decision 11: Constrain Discovery and Reuse Unchanged Results

**Date**: 2026-11-05
**Status**: accepted

### Context

Configured roots can overlap or contain links, dependencies and parser fixtures. Web listings refresh every 15 seconds; reading every unchanged transcript body on each refresh is unnecessary.

### Decision

Check resolved-root containment before candidate reads, ignore directory-symlink descent and list canonical files once. Propose skipping `.git`, `node_modules` and `testdata` subdirectories, while scanning explicitly configured roots themselves. Require zero transcript-body reads on unchanged in-process refreshes and updates after inputs change.

### Rationale

These are testable boundaries and caching outcomes without a machine-specific cold-time threshold. Q3 leaves the directory-skip tradeoff to the human; concrete cache mechanics and setting names belong to design.

### Alternatives Considered

- **Follow all directory links:** rejected because escaping roots and cycles complicate safe discovery.
- **Read every transcript on every refresh:** rejected because it repeats work on unchanged input.
- **Skip only `.git`:** retained as Q3's alternative for users storing logs under dependency/fixture directories.

### Consequences

**Positive:**
- Opening and discovery enforce the same file boundary.
- Unchanged web refreshes avoid transcript-body reads.

**Negative:**
- Cache invalidation must include source-specific backing files.
- Skipped subdirectories require explicit roots when they intentionally hold logs.

---

## Decision 12: Extend the Shared Entry and Project It to JSON Separately

**Date**: 2026-11-05
**Status**: proposed

### Context

Every parser produces `[]transcript.Entry` and every renderer consumes it. `Entry` carries Claude's wire-format JSON tags, used to unmarshal Claude lines, and excludes `Model` from JSON, so it cannot double as the published layout. Exec streams need content the type cannot express today: plans, errors, generic events, usage, exit codes, incomplete state, turns and a provider.

### Decision

Keep `Entry`/`ContentItem` as the single shared representation and add the new fields and content kinds with `json:"-"`. Project the parse result into separate, tagged document types in `internal/sharedjson`.

### Rationale

The user's framing is harness formats in, one representation out, rendered three ways. Extending the existing type gives Markdown and HTML the new content in the same change, and a separate projection keeps the public contract independent of Claude's wire tags. Both independent design drafts reached this decision.

### Alternatives Considered

- **Serialise `Entry` with new tags**: Least code - Rejected: its tags are Claude's input format; changing them breaks Claude parsing, and keeping them publishes one harness's wire format (1.2).
- **A new intermediate representation replacing `Entry`**: Cleaner long term - Rejected: rewrites five parsers and three renderers for no requirement.
- **A separate JSON parsing path**: Isolated - Rejected: JSON could disagree with Markdown and HTML for the same input.

### Consequences

**Positive:**
- New content renders in Markdown, standalone HTML and the web view through existing functions.
- The JSON contract changes only through `internal/sharedjson` and the schema.

**Negative:**
- `ContentItem` grows fields that only exec streams use today.
- The projection must change whenever a parser starts filling a new field.

---

## Decision 13: Long-Lived Lister Cache and All-Projects Resolver Branches

**Date**: 2026-11-05
**Status**: superseded by Decision 23

### Context

Listed sessions must open from any directory (4.2), and unchanged serve refreshes must read zero transcript-body bytes (5.9). The CLI lists and opens in separate processes. Kiro CLI and Kiro IDE resolution turn an empty project into the current directory, and Claude, Codex rollout and Kiro IDE identifiers can repeat across directories.

### Decision

Keep `Lister` and `Resolver` as the two entry points. Give the `Lister` a mutex-guarded cache keyed by stat data (size, mtime, ctime, inode and device where available) and by companion files, including their absence; `apsis serve` holds one `Lister` for its lifetime. Give the `Resolver` explicit all-projects branches that call the same candidate-selection functions as the listing. Return fresh slices from every pass.

### Rationale

Opening by identifier in a new CLI process then reads only that session, rather than rebuilding a listing of every source first. Stat-keyed caching meets the zero-read rule directly and can be tested with a counting opener. Sharing selection functions makes listing and opening agree without storing locators. The mutex suits a 15-second poll.

### Alternatives Considered

- **A catalogue publishing immutable snapshots with private locators**: Exact storage locations for opening - Rejected: the CLI would build a cold snapshot of every source to open one session, and snapshot publication and stale-locator retries add lifecycle code that the shared selection rule makes unnecessary.
- **Filesystem watchers**: Push updates - Rejected: new dependency, recursive-watch limits on macOS and Linux, and a missed event leaves stale state.
- **Time-based expiry**: Simple - Rejected: re-reads unchanged bodies on expiry, failing 5.9.

### Consequences

**Positive:**
- CLI opening stays as cheap as today.
- Adds, removals, edits and companion changes show on the next pass.

**Negative:**
- Listings are serialised by the mutex.
- Change detection relies on stat fidelity; on platforms without ctime or inode, a same-size rewrite that preserves mtime is missed.
- Opening Kiro IDE sessions in all-projects scope reads every workspace's `.chat` headers.

---

## Decision 14: Exec Identifier From a Thread Prefix and a Full Path Digest

**Date**: 2026-11-05
**Status**: proposed

### Context

Requirement 2.8 needs a bounded, route-safe identifier derived from the thread ID, unique within `codex-exec`, and unchanged by appends, added copies, overlapping roots or root order. Copies and resumed runs share thread IDs (Q18). The web route rejects identifiers over 256 characters. On case-insensitive volumes, two root spellings can reach one file.

### Decision

Identifier = `sanitise(thread_id)` truncated to 191 bytes, `-`, then the full 64-character hex SHA-256 of the canonical path. Canonical paths resolve symlinks and use on-disk spelling for roots and symlink targets. If two distinct canonical paths produce the same identifier, both are omitted with a warning.

### Rationale

The identifier depends only on facts about the stream itself, so no other file can rename it. The readable thread prefix lets people correlate streams by eye, and hashing only the path lets the resolver find the file from a metadata-only walk. A full digest makes a collision impractical, and omitting both sides keeps availability independent of which copy appeared first.

### Alternatives Considered

- **`exec-` plus a digest of thread ID and path**: Opaque and collision-resistant - Rejected: opening would need every candidate's thread ID, so every stream head would be read.
- **Thread prefix plus a 16-hex path digest, omitting the later path on collision**: Shorter - Rejected: which file survives a collision would depend on what else exists.
- **Bare thread ID with a suffix only on collision**: Friendlier when unique - Rejected: adding a copy renames whichever file gains the suffix.
- **Content hash**: Path-independent - Rejected: appends change it.

### Consequences

**Positive:**
- Identifiers stay stable under every change 2.8 lists, including case aliases.
- Opening by identifier reads one stream head.

**Negative:**
- Moving or renaming a stream changes its identifier (accepted in Decision 9).
- Identifiers are long; the web list already truncates display IDs.

---

## Decision 15: Resolve the Exec Lifecycle per Turn and Item at End of Stream

**Date**: 2026-11-05
**Status**: proposed

### Context

The pinned 0.166.0 emitter numbers items `item_N` from zero per process, so appended resumed output can reuse `item_0`. Items interleave, receive updates and may never complete. Requirement 2.3 needs one final state per item within its turn, in first-appearance order, without rendering lifecycle updates. Renderer maps (`BuildToolMeta`, `preprocessEntries`) key tool links by ID across the whole transcript.

### Decision

Key records by `(turn ordinal, item id)`, with turn 0 before the first `turn.started`; keep first-appearance order; replace stored item JSON on each event; translate final states at EOF. Turn-level and stream-level events become their own ordered records. Tool IDs are `t<turn>:<item id>`, with the original kept as `item_id`.

### Rationale

Per-turn keys survive ID reuse in the parser, and turn-qualified tool IDs carry that separation through every renderer without changing the renderers' maps. Translating once at EOF keeps lifecycle handling out of the item translators.

### Alternatives Considered

- **Key by item ID alone**: Simpler - Rejected: collides when resumed output is appended to one file.
- **Turn-scoped keys but bare item IDs as tool IDs**: Readable IDs - Rejected: two turns reusing `item_0` would show the second command beside the first result.
- **Emit content as events arrive**: Streaming - Rejected: needs in-place replacement of multi-block items and cannot know completion before EOF.

### Consequences

**Positive:**
- Interleaved tools keep their links and order in JSON and in every renderer.
- Incomplete items are known exactly.

**Negative:**
- The whole stream is held in memory before output.
- Shared-JSON tool IDs differ from the source's item IDs; consumers use `item_id` for the original.

---

## Decision 16: Show the Provider Wherever the Model Is Shown

**Date**: 2026-11-05
**Status**: proposed

### Context

Requirement 3.1 displays the Codex provider beside the model following the existing metadata pattern, and 5.1 allows no other output change. Read/Edit group headers, user headers and deferred tool-result sections pass an empty model today, and `preprocessEntries` groups across model changes. The two design cross-reviews disagreed on whether groups and deferred results should carry the provider.

### Decision

Add the provider to the assistant header meta through `modelLabel`, the only place the model renders. Leave group, user and tool-result headers and the grouping rules unchanged. Exec turns never form Read/Edit groups.

### Rationale

Showing the provider exactly where the model appears is the existing pattern, keeps every rollout without a provider byte-identical, and keeps rollouts with one unchanged apart from the header label. Adding provider-aware group boundaries would change grouping for existing Copilot, Kiro and Codex output.

### Alternatives Considered

- **Carry model and provider into group and deferred-result headers, and split groups at provider changes**: More places show the provider - Rejected: changes existing grouping and header output against 5.1, and shows a provider where its model is still hidden.
- **Provider as a separate ` · ` meta part**: Simple - Rejected: detaches the provider from the model it served (Q40).

### Consequences

**Positive:**
- Existing output changes only in Codex assistant headers that record a provider.

**Negative:**
- Tool-only Codex entries absorbed into Read/Edit groups show neither model nor provider, as today.

---

## Decision 17: Suppress Exec Streams by Rollouts Whose Recorded ID Matches Their Filename

**Date**: 2026-11-05
**Status**: superseded by Decision 22

### Context

Requirement 2.7 suppresses an exec stream when a resolvable rollout with the same thread ID exists. `findCodexSession` opens rollouts by the UUID in their filename, while the thread ID a rollout records is `session_meta.payload.id`. A filename-only index can include malformed or unreadable rollouts, and a recorded-ID-only index can name a rollout that `codex/<id>` cannot open.

### Decision

Build the index from rollout heads in every scope, before project filtering: a rollout contributes its recorded `session_meta.payload.id` when that ID equals, ignoring case, the UUID in its filename. If rollout listing fails, keep streams.

### Rationale

Each indexed rollout is both openable by `codex/<id>` and verified to record the same thread, which is the plain reading of "resolvable rollout with the same thread ID". The listing already reads each rollout's `session_meta`, so the check costs no extra reads.

### Alternatives Considered

- **Filename UUIDs only**: Matches what the resolver opens - Rejected: a malformed or unrelated file with a matching name would hide a stream.
- **Recorded IDs only**: Matches the thread - Rejected: a renamed rollout would hide a stream that `codex/<id>` cannot open.

### Consequences

**Positive:**
- A stream is hidden only when the run is reachable under `codex`.

**Negative:**
- A rollout renamed so its filename UUID differs from its recorded ID no longer suppresses its streams, so the run lists twice.

---

## Decision 18: Validate Against the Open Schema and a Structurally Closed Test Copy

**Date**: 2026-11-05
**Status**: superseded by Decision 25

### Context

Requirement 1.3 keeps additive fields compatible with consumers that ignore unknown fields; 1.4 requires fixtures to conform to the published schema. An open schema cannot catch fields the code emits but the schema omits. Closing every object would forbid arbitrary event payloads, tool inputs and usage counters, which must stay open. The module has no JSON Schema engine, and the handoff asks for no heavy dependency.

### Decision

Publish the schema without `additionalProperties: false`. Tests validate every fixture document against the published schema and against a copy that adds `additionalProperties: false` to apsis-owned structural objects only (document roots, session metadata, scope, entry, each block type, warning, plan item), leaving `input`, `payload` and `counters` open. Validation uses `github.com/santhosh-tekuri/jsonschema/v6`, imported only from `_test.go` files, with the schema loaded from disk and no network loading.

### Rationale

The published schema matches the compatibility rule, and the closed copy makes CI fail when code emits an undeclared structural field. A maintained validator avoids hand-written keyword semantics. Its only non-test dependency, `golang.org/x/text`, is already in the module graph. Both independent design drafts chose this library.

### Alternatives Considered

- **Hand-written validator for the keywords used**: No new module - Rejected: validator bugs would pass bad output, and it needs its own tests (offered as question U3).
- **Generate the schema from Go types**: One source - Rejected: needs a generator dependency and does not check emitted values such as `oneOf` discriminators.
- **Golden JSON only**: No schema tooling - Rejected: does not show conformance to the published schema (1.4).

### Consequences

**Positive:**
- Undeclared structural fields and type drift fail tests, while arbitrary payload keys stay valid.
- Binaries gain no dependency.

**Negative:**
- `go.mod` gains a requirement used only by tests.
- The list of closed objects must grow with the schema.

---

## Decision 19: Session Metadata Comes From the Transcript First

**Date**: 2026-11-05
**Status**: superseded by Decision 24

### Context

A session document needs creation time, project, model and provider for file input, stdin and resolved sessions. Some sources record these inside the transcript (Codex `session_meta`, Kiro IDE `startTime`, Claude `cwd`); others only beside it (Copilot `workspace.yaml`, the Kiro CLI database key). Resolvers sometimes substitute file times. `ParseResultMetadata` holds only cost today.

### Decision

Derive document metadata from parsed content first, using the per-source table in the design. Resolver metadata fills only fields still missing, and never supplies a filesystem-flagged time as `created_at`. Exec streams take their project from the containing repository of the input path.

### Rationale

The same file gives the same document whether opened by path or by identifier, wherever the transcript records the fact. Resolver data adds what only the listing knows without presenting file times as source times (Q26).

### Alternatives Considered

- **Resolver metadata first**: Matches listings exactly - Rejected: file and stdin input would produce different documents for the same content.
- **Parsed content only**: Simplest - Rejected: Copilot and Kiro CLI documents would lose a project the listing shows.

### Consequences

**Positive:**
- Documents agree across input modes for transcript-recorded facts.

**Negative:**
- For Copilot and Kiro CLI, listing and session `created_at` can differ slightly, because they come from different records.

---

## Decision 20: One Streaming Probe Decides Exec Detection and Discovery

**Date**: 2026-11-05
**Status**: superseded by Decision 21

### Context

Requirement 2.1 detects an exec stream by its first parseable, non-empty JSON event. `DetectFormat` reads 64 KiB, and the listing's head readers stop at 50 lines and 1 MiB, so a long malformed prefix could make detection and discovery disagree with each other and with the requirement.

### Decision

`ProbeExecStream` reads whole lines until the first one that parses as a JSON object, skipping lines over 10 MiB, and classifies the stream. Discovery uses it directly, with its result cached by stat key. `DetectFormat` applies the same test inside its chunk, before Kiro detection (whose truncated Kiro IDE check matches bare substrings), and, only when the chunk holds no parseable object and is not Kiro, keeps reading for the probe and returns the consumed bytes for replay; that path decides only between exec and the existing result.

### Rationale

One rule gives the same answer for direct conversion and discovery at any prefix length. Restricting the extended read to the exec decision leaves detection of every existing format unchanged (5.1). The cache bounds the cost to one read per file change.

### Alternatives Considered

- **Exec check inside the existing 64 KiB chunk only**: No new reader - Rejected: misses `thread.started` after a long malformed prefix.
- **Extend detection for every format**: More files detected correctly - Rejected: changes existing detection results, against 5.1.

### Consequences

**Positive:**
- Detection and discovery agree on every stream.

**Negative:**
- A JSONL file under a root with no parseable line is read in full once per change.

---

## Decision 21: Complete the Exec Probe Before Trusting Heuristic Kiro Recognition

**ID**: 21
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 20

### Context

Both non-author reviews found that Kiro's substring-only recognition can match malformed prefixes or a partial long exec object. Accepting that heuristic before the complete-line probe makes discovery and direct conversion disagree.

### Decision

Keep complete structurally validated Kiro recognition and the legacy fallback. Continue the exec probe beyond the initial chunk when needed to obtain its first complete parseable object, even after a heuristic Kiro match. Recombine split lines and retain consumed bytes for replay. Only a valid initial thread.started selects the exec format; the other probe outcomes retain the legacy result.

### Rationale

A heuristic is insufficient to override the approved exec signature. Restricting the probe's promotion to exec preserves existing outcomes when the input does not establish that source.

### Alternatives Considered

- **Stop whenever any Kiro heuristic matches:** rejected because arbitrary text can contain all marker keys before the first real event.
- **Use only the first 64 KiB:** rejected because approved detection permits longer malformed prefixes and first objects.
- **Broaden extended detection to other sources:** rejected because this changes existing recognition beyond 5.1.

### Consequences

**Positive:**

- Direct conversion and discovery apply the same complete-event decision.
- Split long first objects cannot acquire another format solely through marker strings.

**Negative:**

- Ambiguous heuristic-only inputs may require reading farther before selecting the fallback.
- The replay path must retain all consumed bytes and physical-line boundaries.

---

## Decision 22: Suppress Only the Verified Rollout Candidate the Resolver Selects

**ID**: 22
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 17

### Context

Several files can share a filename UUID. The existing resolver opens the first match, while indexing any later valid duplicate can hide a stream behind an invalid selected rollout. Both reviews found this mismatch.

### Decision

Share selectCodexCandidate across listing, resolution and suppression, preserving the existing global first-filename-match choice. Only that winner contributes to suppression after containment, readability, parseable Codex head and filename/recorded-ID checks. An invalid winner leaves streams visible; in all-projects listings its row is omitted with a warning and a later duplicate is not automatically promoted. Project filtering follows selection and suppression.

### Rationale

The indexed rollout must be the one that source/identifier opening reaches. Keeping the established winner avoids silently changing legacy bare-ID resolution; explicit file conversion still reaches other copies.

### Alternatives Considered

- **Index every valid duplicate:** rejected because a later file does not prove the selected file is usable.
- **Automatically promote the first valid duplicate everywhere:** rejected because it changes existing bare-ID selection.
- **Index filename UUIDs without reading the selected head:** rejected because unreadable or unrelated content could suppress usable streams.

### Consequences

**Positive:**

- Every suppressing rollout is the resolver's verified candidate.
- Human-approved global precedence and project preference remain intact.

**Negative:**

- A valid duplicate shadowed by an invalid winner requires explicit path conversion.
- Selection, head validation and warning handling must be shared rather than independently approximated.

---

## Decision 23: Gate Cache Admission on Platform Identity and Change Fingerprints

**ID**: 23
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 13

### Context

Requirement 5.9 requires zero body reads on unchanged refreshes and detection of changed inputs. Size/mtime alone can miss equal-size replacements preserving modification time. The existing x/sys module exposes Windows metadata-handle queries; their storage-specific behaviour still needs implementation verification.

### Decision

Keep the long-lived mutex-guarded Lister cache, companion absence keys and fresh result slices. Require Unix identity/change stat fields and Windows volume/file identity plus last-write/change times through a platform adapter. Use metadata-only handles; never silently downgrade to size/mtime. Gate cache consumers on early adapter and native Windows replacement tests. If required metadata is unavailable, block admission and record the platform limitation for a requirements decision.

### Rationale

Platform-specific metadata can preserve the no-body-read outcome without pretending that a weak key detects replacements. Microsoft's [file identity](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info) and [basic file information](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_basic_info) define the Windows fields; [GetFileInformationByHandleEx](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getfileinformationbyhandleex) documents the query and driver dependence. This research establishes the interface, not successful runtime verification.

### Alternatives Considered

- **Use size/mtime on Windows:** rejected because the design would knowingly miss required changes.
- **Hash every transcript on each refresh:** rejected because it violates zero body reads.
- **Assume API support without an early gate:** rejected because filesystem capabilities and errors must not become hidden correctness exceptions.

### Consequences

**Positive:**

- The cache contract has a concrete cross-platform verification path.
- Identity replacement is distinct from content modification and companion changes.

**Negative:**

- Platform adapters and native Windows checks are required before cache admission.
- An unsupported storage environment remains a real dependency rather than an approved exception.

---

## Decision 24: Preserve First-Known Parsed Summaries Without Requiring Assistant Content

**ID**: 24
**Date**: 2026-11-05
**Status**: superseded by Decision 30

### Context

Codex session_meta can record model/provider before any assistant entry. A header-only or user-only session loses that metadata if the projection looks only at entries. Both reviews identified the contradiction between the per-source table and entry-only summary rule.

### Decision

Add FirstModel/FirstProvider to parsed metadata. Each parser retains the first nonempty source values it already recognises in transcript order; entries retain their active values. The projection uses those summaries, falling back to first nonempty entry values, and retains parsed-first precedence for all other metadata. Resolver metadata fills only missing facts and never supplies a filesystem-flagged creation time. U1/U2 still control additional Claude model and Kiro IDE provider extraction.

### Rationale

Summary metadata exists independently of assistant content. Keeping it in the common parse result makes file, stdin and identifier conversion agree without a second JSON-specific parser.

### Alternatives Considered

- **Summarise entries alone:** rejected because recognised header/model-change metadata can precede every assistant entry or be the only content.
- **Use resolver metadata first:** rejected because file/stdin conversion has no resolver and filesystem fallback must not become a recorded time.
- **Read source JSON again inside the projection:** rejected because it creates another parsing path and can disagree with Markdown/HTML.

### Consequences

**Positive:**

- Header-only and user-only sessions retain known model/provider summaries.
- Entries keep recorded changes while the first-known session summary stays stable.

**Negative:**

- Parsers must retain recognised summary values separately from their active model/provider state.
- Additional source-field extraction still needs its own scope decision.

---

## Decision 25: Publish the Complete Known Version 1 Schema Before Staged Producers

**ID**: 25
**Date**: 2026-11-05
**Status**: proposed; local technical admission, supersedes Decision 18

### Context

Both task cross-reviews identify a compatibility hole: adding a new discriminator to a closed block union rejects newer output under an older cached version-1 schema. Unknown-field tolerance does not solve unknown discriminator values. The schema can be complete before every parser/projection producer exists, without introducing unused production Go types.

### Decision

Publish both session/listing kinds and every known v1 field/block in PR 1. Known block types use disjoint branches; an open extension branch excludes those known discriminators and accepts other type strings. Consumers ignore unsupported block types as well as unknown fields. Current unknown source content still projects as `other`. Preserve an exact initial-schema test fixture; later listing and exec output must validate against it. Removing a field or changing its meaning still raises the version.

Retain Decision 18's test-only local validator and dual validation. The closed test copy closes known apsis-owned structural objects, while input, payload, counters and unknown extension objects stay open. U3 remains unanswered; the dependency is admitted as a reversible agent technical choice.

### Rationale

Later slices do not invalidate consumers holding the first schema, and malformed known blocks cannot escape their constraints through the extension branch. Handwritten synthetic documents exercise future known branches before production support arrives. These decisions address both non-author cross-reviews and preserve approved additive compatibility.

### Alternatives Considered

- **Stage a closed enum and tell consumers to reload upstream main:** rejected because pinned or offline validators retain the older rejecting enum and an unchanged $id does not refresh it.
- **Raise the schema version for every additive block:** rejected because known v1 scope can be published once and approved 1.3 treats additive fields as compatible.
- **Generate all representation types and projection code immediately:** rejected because those producers have no consumer yet; schema fixtures give the needed evidence without premature code.

### Consequences

**Positive:** later producers validate against the original contract; known-field mistakes remain detectable; binaries gain no schema-engine dependency.

**Negative:** the initial schema includes branches before their production emitters; the validator adds a test-only module requirement; the extension branch intentionally cannot validate semantics of a future unknown type.

---

## Decision 26: Isolate Fingerprint Adapters for Independent Native Verification

**ID**: 26
**Date**: 2026-11-05
**Status**: proposed; local technical admission

### Context

Decision 23 requires native Windows identity/change evidence before cache admission. The design originally put adapters inside sessions, where parallel list/open work also compiles and edits files. The existing x/sys module exposes Windows queries but lacks the matching result structs.

### Decision

Place the metadata-only interface and Unix/Windows adapters in internal/fileidentity; the sessions cache consumes it. Define the required Windows ABI structs locally, use metadata-only handles and preserve unsupported/error outcomes. Test equal-size replacement and in-place rewrite with restored mtime on native Windows as well as Unix and injected adapters. Cross-compilation cannot satisfy the native gate. Task 15.2 waits for schema task 3.2 before editing go.mod/go.sum, promotes x/sys to direct, and stays incomplete without native evidence.

### Rationale

This reviewed package placement keeps platform ownership disjoint and makes the native prerequisite runnable without the full sessions stack. It changes placement, not Decision 23's fingerprint or cache contract.

### Alternatives Considered

- **Keep the adapters in sessions:** rejected because the independent capability stream then shares a changing package and heavier native test dependencies with listing work.
- **Reuse transcript follow-mode file info:** rejected because its Windows identity is absent and it has different lifecycle needs.
- **Use size/mtime or mocked Windows evidence:** rejected because preserved-mtime replacement/rewrite is the reason for the capability gate.

### Consequences

**Positive:** native verification has a small target; cache consumers share one adapter; sessions and fingerprint implementation can progress separately.

**Negative:** one additional internal package is introduced; module mutation is serialized; cache completion still requires unavailable external native evidence.

---

## Decision 27: Separate Delivery Slices and Preserve Canonical Task Identity

**ID**: 27
**Date**: 2026-11-05
**Status**: proposed; local planning admission, not human approval

### Context

The Codex draft's predecessor chain blocks listing and exec work on unavailable Windows evidence and groups too many behaviours together. The Claude draft has useful PR boundaries and red/green pairs, but its independent module writers conflict. Existing canonical tasks carry stable identities and one live baseline owner.

### Decision

Merge four PR phases: shared JSON/provider, cross-project listing, exec streams, then cache. Keep fifteen adjacent red/green child pairs under sixteen parents. Preserve canonical numbered/stable IDs 1–11 and every status/owner; retain the byte-exact earlier file and scope map. Streams cover transcript/projection, sessions/CLI/web, and isolated fingerprints. Block shared-file consumers explicitly; task 15.2's module writes wait for task 3.2. Native Windows gates only fingerprint completion and cache consumers. Requirement 5.9 stays unfinished until the cache slice passes.

Each PR's last green task includes its README/CHANGELOG changes and Makefile checks. Benchmarks record fixture dimensions, platform/filesystem and cold/warm measurements; any 1.5 s local budget prompts investigation only. Zero unchanged transcript-body bytes remains a hard assertion. No commits, pushes, CI changes or policy relaxations are admitted here.

### Rationale

The task list can advance reviewed local implementation without destroying provenance or making unrelated work depend on absent native evidence. Both non-author task cross-reviews supply the corrective evidence; a fresh critique and peer review must assess this merged revision before implementation admission beyond the separately verified baseline repair.

### Alternatives Considered

- **Keep the single predecessor chain:** rejected because it makes Windows a prerequisite for work the design leaves ungated.
- **Replace canonical tasks with the draft:** rejected because generated draft IDs/pending status would erase original ownership and identity.
- **One stream per PR with independent module writes:** rejected because the CLI/schema/module files overlap across PR boundaries.

### Consequences

**Positive:** first three slices progress independently of native Windows; small paired work units and preserved provenance support review; shared module writes are ordered.

**Negative:** the feature remains incomplete after PR 3 because 5.9 waits for PR 4; cross-stream test runs need compile coordination; four slices require more integration checkpoints.

---

## Decision 28: Preserve Codex Metadata Changes With an Explicit Compatibility Option

**ID**: 28
**Date**: 2026-11-05
**Status**: superseded by Decision 29

### Context

Approved 1.6 preserves later recorded model/provider changes on entries. Approved 5.1 keeps provider-less default output unchanged. The current Codex parser consolidates adjacent assistant content until a user boundary; a later model-only header does not relabel or split the current entry. Splitting every such run would alter protected legacy rendering, while keeping one model-labelled entry cannot express both runs in the new format. Identical repeated headers must not introduce spurious boundaries.

### Decision

Add `ParseOptions.PreserveModelChanges`, enabled only by shared-JSON callers alongside `AllowEmptyCodex`. The common Codex parser starts a new assistant entry at the next content item when the active model changes and that flag is true. Default provider-less Markdown/HTML/web keeps current consolidation; natural entry boundaries use active metadata as before. A recorded provider transition may create a new entry under the explicit provider-output exception in 3.1. Identical metadata never splits, earlier content is never relabelled, and first-known summaries remain fixed. This does not change renderer Read/Edit grouping or the metadata-free group headers retained by Decision 16.

Forward both flags through `Parse`, forced parsing and `ParseResolved`; the CLI retains explicit forced-format precedence and the web uses defaults. Regress model changes, identical headers and tool links at the parser and ID/latest helper integration layers. This is a technical reconciliation of already approved behaviours, not a human answer or approval.

### Rationale

One parser and the existing common Entry model can express the new-format fidelity contract while protecting legacy output. A boolean option is local and reversible; it requires neither a second parser nor new public schema fields. The follow-up reviewer must verify this precision before implementation admission.

### Alternatives Considered

- **Split every model change in default parsing:** rejected because provider-less Markdown/HTML grouping changes against 5.1.
- **Wait for natural user boundaries for every output:** rejected because adjacent changed model runs cannot both appear with their recorded values in shared JSON under 1.6.
- **Add a second parsing path or metadata-history schema:** rejected because it duplicates source handling or expands the contract unnecessarily; the existing option and entry mechanisms suffice.

### Consequences

**Positive:** shared JSON retains both metadata-bearing runs; default provider-less grouping stays unchanged; identical headers and first-known summaries remain stable.

**Negative:** callers must retain two parse flags during refactoring; a provider transition can add an assistant section as permitted by 3.1; parser and CLI/helper tests must cover both modes and tool links.

---

## Decision 29: Source the Codex Model From Turn Context

**ID**: 29
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 28; reversible local technical choice awaiting verification by an agent other than its author

### Context

Decision 28 split Codex assistant entries when `session_meta.payload.model` changed. The pinned Codex rust-v0.166.0 `protocol.rs` shows `SessionMeta` records `model_provider` but no `model`; the model is `TurnContextItem.model`, written as `turn_context` rollout lines, which the parser skips. A key-presence count over 421 local interactive rollouts found no `session_meta.model`, `turn_context.model` in every rollout (first occurrence at physical lines 5 to 11), five rollouts recording more than one model, and no `session_meta` after content. Real rollouts therefore produced no Codex model, lost recorded model changes (1.1, 1.6, 4.3), and never exercised the session_meta split.

### Decision

The active Codex model is the latest nonempty `turn_context.payload.model`, else `session_meta.payload.model` for older files. `FirstModel` and the Codex listing model take the first nonempty value in transcript order; the listing reads it within the existing `codexMetaScanLimit`. With `ParseOptions.PreserveModelChanges`, set only by shared-JSON callers, assistant entries take the active model when created, and a change without an intervening user boundary starts a new entry at the next model-produced item (Q82). Without the option, `Entry.Model` keeps its legacy session_meta-only value, so default Markdown, HTML and web output change only by the provider label (5.1). Provider handling, identical-metadata consolidation, first-known summaries and `AllowEmptyCodex` stay as Decision 28 described. Whether headers also show the Codex model is user question U4 (Q85).

### Rationale

`turn_context` is the record Codex writes for the model, so reading it is preservation of recorded evidence rather than inference. Gating entry models behind the existing shared-JSON option meets approved 1.1 and 1.6 without changing protected 5.1 output, and keeps one parser path.

### Alternatives Considered

- **Keep session_meta as the only model source:** rejected because Codex 0.166.0 never writes it there, so shared JSON and listings would never carry a Codex model.
- **Apply turn_context models to every entry by default:** rejected for now because Codex Markdown/HTML headers would gain a model label, beyond 5.1's provider exception; offered to the user as U4.
- **Add a separate model-history field to the schema:** rejected because entry-level `model` already expresses changes.

### Consequences

**Positive:**
- Shared JSON and listings report the recorded Codex model and its changes.
- Default rendering stays unchanged apart from the approved provider label.

**Negative:**
- Until U4 is answered, shared JSON shows a Codex model that Markdown/HTML headers do not.
- The completed task 2 is reopened for a follow-up pair.

---

## Decision 30: Prefer Recorded Creation Records Over First-Message Times

**ID**: 30
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 24; reversible local technical choice awaiting verification by an agent other than its author

### Context

Decision 24 kept first-known model and provider summaries and gave parsed metadata precedence over resolver metadata. The completed Copilot parser took `StartedAt` from its first converted entry, the first prompt, although `session.start` records `data.startTime`; the listing uses `workspace.yaml` `created_at`. The Kiro CLI parser takes its first message time while the listing uses the database's recorded `created_at`. One session could therefore show two different creation times.

### Decision

Retain Decision 24's first-known `FirstModel`/`FirstProvider` summaries and entry-level active values (with the Codex model per Decision 29). `created_at` uses, in order: a parsed session-start record (Claude first timestamped record, Codex first valid `session_meta` outer timestamp per Q81, Copilot `session.start` `data.startTime` else its first parseable event timestamp, Kiro IDE `metadata.startTime`); then a resolver-recorded creation time that is not a filesystem fallback (Kiro CLI database `created_at`, Copilot `workspace.yaml` `created_at`); then the first entry timestamp. Parsers set `StartedAt` only from start records, so the Kiro CLI parser leaves it empty and the projection applies the fallback. A filesystem time never becomes `created_at`. Project and other metadata keep parsed-first precedence.

### Rationale

A creation time should come from a record of creation when one exists. This order matches each listing's time source where the listing reads a recorded field, and still gives file and stdin input a time from the transcript.

### Alternatives Considered

- **Keep first converted entry times:** rejected because they record the first prompt, not creation, and disagree with listings.
- **Resolver metadata first for every field:** rejected because file and stdin input have no resolver, and parsed projects and models are the authority.
- **Per-source special cases in the projection:** rejected in favour of one ordered rule plus parsers that report only start records.

### Consequences

**Positive:**
- Session documents and listing rows agree on creation time for Copilot and Kiro CLI.
- One precedence rule covers every source.

**Negative:**
- The completed Copilot and Kiro CLI parser metadata changes in the reopened task 2.
- Kiro CLI file or stdin input without a resolver uses its first entry time, as before.

---

## Decision 31: Preserve Recorded Session Origin and Entry Identifiers

**Date**: 2026-11-05
**Status**: accepted

### Context

Claude Code records an `entrypoint` on each entry (for example `cli` for an interactive terminal session and `sdk-cli` for a `-p` run), and Codex records `originator` in `session_meta` (for example `codex-tui` or `codex_exec`). Claude Code also records a `uuid` per entry. Revision 3 of the shared JSON contract keeps neither, so a consumer that needs them must parse each harness format again, which the shared representation exists to avoid. orbit itself starts Claude Code with `-p` and Codex with `exec`, so the all-projects listing mixes orbit-started sessions with interactive ones, and origin is the only recorded field that tells them apart.

### Decision

Add requirements 1.7 and 1.8 (revision 4): shared JSON session and listing metadata carry the recorded entry point or originator exactly as recorded, and each entry carries any identifier its source records. Markdown and HTML output stay unchanged.

### Rationale

Both fields are recorded by the source, so they follow Q3 and Decision 6: apsis records and passes on, and consumers apply their own interpretation. Both are additive under 1.3. Keeping them out of Markdown and HTML leaves 5.1 and existing output tests untouched.

### Alternatives Considered

- **Classify sessions as interactive or automated**: Ready-made for consumers - Rejected: a value such as `sdk-cli` does not establish who wrote a prompt, and Decision 6 rejects contract fields that guess.
- **Per-entry source positions**: Lets consumers point back into the source file - Rejected for now: entries are built by merging and splitting source units (exec items folded into turns, Copilot turn ends, model-change splits), so one position per entry is ill-defined, and no apsis feature needs it.
- **Show origin in Markdown and HTML headers**: Follows the pattern for provider - Rejected for now: it changes existing output for every Claude Code and Codex session for limited reading value; it can be added later without a schema change.

### Consequences

**Positive:**
- Consumers can separate interactive from tool-started sessions using recorded values only.
- Entries can be referenced by their source's own identifiers where one exists.

**Negative:**
- Origin values are harness-specific strings; each consumer maps them itself.
- Sources other than Claude Code and Codex may record no origin, so the field is often absent.

---

## Decision 32: Entry Identifiers Come From the Single Record an Entry Is Built From

**Date**: 2026-11-05
**Status**: proposed; reversible local technical choice implementing approved 1.8, awaiting verification by an agent other than its author

### Context

Requirement 1.8 (Decision 31) asks each shared JSON entry to carry any identifier its source records for it, separately from tool-linking identifiers. Parsers build entries in three ways. Most entries come from one source record: a Claude Code line (`uuid`), a Codex rollout user `message` item (optional `payload.id`), a Copilot `user.message` event (`id`), a Kiro CLI assistant message (`message_id`) and a Kiro IDE execution-detail action (`actionId`). A Kiro IDE tool action yields two entries, a tool use and its result. Some entries merge several records: Codex assistant entries consolidate adjacent response items, Copilot assistant entries collect a turn's events and its tool-result entries collect several completion events, and Codex exec entries are whole turns. Records without an identifier exist in every source except Claude.

### Decision

Add `Entry.RecordID` (`json:"-"`). Parsers set it only where an entry is built from exactly one record that records an identifier, and entries split from one record share it. An entry merged from several records has no `RecordID`. Codex exec item identifiers stay on content blocks as `item_id`. The projection emits `RecordID` as the entry `id` and omits it when empty. `Entry.UUID` and every tool-linking field keep their current values and uses.

### Rationale

An identifier from one of several merged records would mislabel the rest of the entry, so a merged entry has no single recorded identifier to carry. Deciding at each creation site keeps the rule fixed per entry kind, needs no bookkeeping as content is appended, and leaves renderers and raw output untouched (5.1, Decision 31).

### Alternatives Considered

- **First record's identifier for merged entries**: Gives more entries an `id` - Rejected because it attributes later records' content to the first record's identifier.
- **An array of every merged record's identifier**: Keeps all identifiers - Rejected because consumers could not tell which content each identifier covers, and it changes the field's shape between sources.
- **Identifiers on every content block for all sources**: Precise for merged entries - Rejected for now as wider than 1.8; it can be added later as an additive block field without a version change.
- **Reuse `Entry.UUID` for all sources**: No new field - Rejected because renderers use `UUID` and `ParentUUID` for Claude skill and slash-command lookups, so setting it for other sources risks changing protected output.

### Consequences

**Positive:**
- Claude, Codex user, Copilot user, Kiro CLI assistant and Kiro IDE action entries can be referenced by the source's own identifier.
- The rule is fixed per creation site, so tests can enumerate it source by source.

**Negative:**
- Codex assistant entries, Copilot assistant and tool-result entries, Kiro chat-file entries and exec turns carry no `id`.
- Entry `id` values are not unique within a document: a Kiro IDE tool action's two entries share one.

---

## Decision 33: Validate Every Selected Codex Rollout in All-Projects Listings

**Date**: 2026-11-06
**Status**: proposed (agent decision during task 7; not human-approved)

### Context

Decision 22 lists a Codex identifier across projects only from the rollout the resolver opens, and omits it with a warning when that winner fails the containment, readability, parseable-head and recorded-ID checks. Its context is duplicated filename UUIDs, so it leaves open whether the checks apply to identifiers held by one file only. Task 7 needed a rule.

### Decision

Apply the checks to every selected rollout in all-projects scope. An unreadable rollout is omitted with `path_unreadable`; one without a decodable `session_meta` within `codexMetaScanLimit` lines, or whose `session_meta.payload.id` differs from its filename UUID (ignoring case), is omitted with `invalid_record`. Rollouts named without a UUID need only the `session_meta`. Later copies of an omitted identifier are not listed and get `duplicate_session`. Project scope keeps its legacy per-file rows (Q62).

### Rationale

One rule for every identifier keeps listing, opening and the later suppression index (task 13) on the same verified candidate, without a special case for groups of one. No existing output changes: project rows already require a `session_meta` `cwd`, and all-projects listings are new in this feature.

### Alternatives Considered

- **Check only duplicated identifiers**: a single rollout lists whatever its head says - Rejected because rows would then carry unverified metadata that task 13 must not use for suppression, so the two would disagree about which rollouts are valid.
- **List invalid winners without metadata**: keep the row and drop its fields - Rejected because Decision 22 omits invalid winners, and a row whose recorded ID differs from its filename opens a session other than the one its metadata describes.

### Consequences

**Positive:**
- Every all-projects Codex row has a readable head that records its identifier.
- Listing, opening and suppression share one validity rule.

**Negative:**
- Rollouts written by Codex versions without a `session_meta` header are omitted across projects, each with a warning, which can be noisy for long-time users; they still convert by path.
- A rollout whose recorded ID differs from its filename appears in its project's listing but not across projects.

---
