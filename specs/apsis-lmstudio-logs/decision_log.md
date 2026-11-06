# Decision Log: Apsis — LM Studio Server Logs

## Quick Decisions

| ID | Date | Decision | Rationale |
|----|------|----------|-----------|
| Q1 | 2026-11-05 | Preserve the user-directed separate LM Studio spec, common representation, renderer parity and upstream independence | The sibling's accepted Decisions 5/7 and both handoffs establish this direction |
| Q2 | 2026-11-05 | Agent proposal: use source, forced format and provider `lmstudio`, without a local/cloud flag | Names the mechanism while preserving the sibling's accepted provider-only policy |
| Q3 | 2026-11-05 | Agent proposal: include the seven exact inference endpoints in requirements 2.1 | Public LM Studio documentation confirms both v0 completion endpoints; exact matching excludes token counting and management |
| Q4 | 2026-11-05 | Agent proposal: use the existing 65,536-byte detection bound, require an included request header, and retain forced parsing | A generic log header does not distinguish LM Studio; startup-heavy excerpts remain explicitly convertible |
| Q5 | 2026-11-05 | Agent proposal: reject raw JSON and follow mode for LM Studio, directing raw JSON users to shared JSON | The text disk log is neither the existing raw JSONL format nor the follow parser's line format |
| Q6 | 2026-11-05 | Agent proposal: retain physical line ranges, partial/oversized states and body-free warnings in CLI, shared JSON and web | Recovery must preserve later requests without presenting unreadable bodies as complete or leaking content through diagnostics |
| Q7 | 2026-11-05 | Agent proposal: never concatenate an incomplete record across files | Supplied rotation reports establish no continuation grammar; unknown bytes must not become another request's body |
| Q8 | 2026-11-05 | Agent proposal: count only session-owned records toward logical-session size | The existing browser limit must not count other sessions merely sharing the log file |
| Q9 | 2026-11-05 | Agent proposal: synthetic fixtures only, with separate provenance for log grammar and public payload documentation | The repository must contain no private log content, identifiers, paths or served model names |
| Q10 | 2026-11-05 | Correct the handoff's monthly-volume and project-inference claims; qualify unverified grammar assertions | Aggregate-only file sizes disprove a monthly rate; prompt content is not authoritative project metadata |
| Q11 | 2026-11-05 | The merge records agent proposals and unanswered user choices; no LM Studio requirements approval is claimed | Continuous-work authorisation permits useful local progress, not invented human answers |
| Q12 | 2026-11-04 | Recover the stale 15:00Z merge after recording release and preserving all three populated outputs byte-for-byte | No completion or progress followed the 15:06Z writes; requirements, decisions and questions are retained under `drafts/requirements/*codex-gpt-6-lmstudio-merge-unlogged.md` and remain the merge's base |
| Q13 | 2026-11-04 | Keep earlier private-log findings labelled as reported observations; use public API documentation and repository code for recovery checks | The 13:39Z note directly checked only aggregate volume, so other disk-grammar claims must not become newly verified facts through repetition |
| Q14 | 2026-11-04 | Agent proposal: scope tool links and rendering groups to request occurrences or individual logged-response events, preserving original identifiers for prefix equality | Both fresh-context reviews found that global ID lookup can overwrite results across clients and repeated snapshots; requirements 2.8/8.3 make isolation observable |
| Q15 | 2026-11-04 | Agent proposal: direct multi-session log inspection uses CLI Markdown/HTML/shared JSON; browser session pages carry body-free references and explicit CLI instructions | The existing server has session routes, not direct-file routes; this satisfies renderer parity within each supported view without introducing another browser exposure path |
| Q16 | 2026-11-04 | Agent proposal: retain WARN/ERROR before applying routine-category filtering, including excluded-endpoint diagnostics | Both reviews found conflicting retention/filter clauses; all other routine levels can still be ignored when they carry no included content |
| Q17 | 2026-11-04 | Agent proposal: map native chat's string input and typed input items to supplied user context, retaining system_prompt separately | Public native-chat documentation establishes both forms; endpoint coverage must test both rather than treating the string as an unsupported body |
| Q18 | 2026-11-04 | Fresh-context critique and peer review are same-harness/model evidence, with no independent-model or human-approval claim | Both non-author reviews were scheduled without reading each other and completed at 18:16Z; continuous-work rules permit this evidence for routine refinement |
| Q19 | 2026-11-05 | Keep U2's singleton recommendation unapproved and disclose one-row-per-request growth, with per-file listing and direct-input-only alternatives | The independent Claude review establishes a real listing-volume concern; proposed Decision 1's existing negative consequence remains, and the question now makes that consequence explicit without claiming conversations are shared |
| Q20 | 2026-11-05 | Preserve inclusive all-projects latest and disclose its unsupported-mode outcome; do not add a source-selector requirement for latest | The suggested filter conflicts with the sibling's approved 4.4 and current source-selector contract; Decision 11 records the disposition and U1 retains discovery opt-in as the human choice |
| Q21 | 2026-11-05 | Preserve otherwise untranslated request fields as labelled parameters in every renderer, without ad-hoc credential redaction | Decision 12 preserves source fidelity and makes raw-JSON redirection useful; U1's exposure explanation includes logged parameter/header values |
| Q22 | 2026-11-05 | Keep specialised translation limited to seven conversation endpoints; name /v1/decisions as unsupported and aggregate body-free warnings for other unknown POST paths | Public documentation establishes a distinct typed-decision API, not this slice's disk translation; silent loss is avoided without pretending its payload is a conversation |
| Q23 | 2026-11-05 | Retain complete client metadata, parent-thread identifiers and fully recorded request kind; truncated blobs stay raw/marked | Pinned Codex responses_metadata.rs establishes field provenance and side-request sharing; thread membership is not proof of a dialogue turn or response ownership |
| Q24 | 2026-11-05 | A valid LM Studio header signature wins over other formats' body-marker heuristics; missing bounded signatures suggest forced parsing, while discovered matching files still parse as logs | The independent review found Kiro marker collisions and notice-only/late-request files; this clarifies input outcomes without broadening automatic recognition |
| Q25 | 2026-11-05 | Browser session commands include all-projects scope/source, while direct-log commands identify the file and forced format | Oversized-session instructions must not suggest a bare identifier that cannot reopen this source |
| Q26 | 2026-11-05 | Direct-file request/membership IDs match discovery inside the configured root; outer log-document and out-of-root/stdin IDs remain input-scoped | Stable membership does not turn a containing multi-session file into one listed conversation, or require reading other log bodies |
| Q27 | 2026-11-05 | The 18:32Z Claude peer review adds independent-model evidence beyond the earlier same-model six-issue checks | This revision weighs all ten findings using current code and primary public documentation; it reads no private logs and supplies no human approval |
| Q28 | 2026-11-05 | Explain common unknown response attribution once per human transcript while keeping per-request shared-JSON state | The state is universal for this grammar; repeating an absence-sounding sentence on every request adds noise and can misdescribe existing unattributed evidence |

All non-superseded full decisions below remain **proposed**. The merge weighs both drafts and their non-author cross-reviews; it does not promote draft proposals to human-approved decisions. Routine technical choices can be admitted for local work under `SPEC-DRIVE.md` after the coordinator records their review evidence. The repository has no `.agentic.json`, so decision revisions use superseding entries.

---

## Decision 1: Group Only Explicit Client Threads

**Date**: 2026-11-05
**Status**: proposed

### Context

Reported Codex request bodies contain `client_metadata.thread_id`, while other requests lack identifiers. Context can shrink, unrelated clients can send the same prefixes, and truncated strings can make different original inputs appear equal.

### Decision

Group exact nonempty string thread identifiers within one configured root, including across files, models and endpoints. Keep requests without that identifier as singleton sessions, and preserve other identifiers only as evidence. User question U2 covers broader grouping.

### Rationale

Recorded client identity survives compaction without treating text similarity as proof of a shared conversation. Stable identifiers must derive from recorded identity and stable file locations, not traversal order or the earliest currently retained request.

### Alternatives Considered

- **Prefix, time or model grouping:** rejected for the baseline because it can merge independent clients, particularly after log truncation.
- **Cache keys, session fields or response-chain grouping:** rejected for the baseline because the inspected reports do not establish general conversation-identity semantics or complete response links.

### Consequences

**Positive:**

- Thread sessions survive context changes and rotation.
- Adding older files or deleting other files does not rename surviving sessions.

**Negative:**

- Unidentified multi-turn clients produce one listing per request.
- Additional client identity conventions need separately justified support.

---

## Decision 2: Keep Request Snapshots and History References

**Date**: 2026-11-05
**Status**: proposed

### Context

Repeated history can make a log appear to contain the same conversation many times. Global deduplication or suppressing assistant context equal to a logged response can also erase intentional repetitions or obscure provenance.

### Decision

Retain request boundaries and complete parsed input sequences in shared JSON. In human-readable views, reference only an identical leading sequence of logged items from the previous request in the same session, and mark changed or shortened history as a context discontinuity. User question U5 retains full visible snapshots as an alternative.

### Rationale

The reader can distinguish resent context from recorded generation while reconstructing any request. Truncated text equality means only that logged representations match; it establishes neither original content equality nor identity.

### Alternatives Considered

- **Render every snapshot as ordinary new turns:** rejected for the baseline because replayed context looks like new conversation activity; labelled full snapshots remain U5's alternative.
- **Globally deduplicate messages or suppress equal responses:** rejected because equal text at a different position or with different provenance is separate evidence.

### Consequences

**Positive:**

- Routine history repetition is reduced without losing request reconstruction.
- Compaction and repeated requests remain visible.

**Negative:**

- Renderers need request boundaries and history references.
- Human-readable requests can depend on earlier sections for their repeated prefix.

---

## Decision 3: Require Evidence for Request Attribution

**Date**: 2026-11-05
**Status**: superseded by Decision 9

### Context

Model-tagged notices and responses can interleave, and requests can outlive a rotated file. The supplied observations of adjacent validation warnings establish a pattern, not a guarantee that a later record belongs to the immediately preceding request.

### Decision

Attribute a response or diagnostic only through a unique explicit recorded link. Keep all other included log-level content in direct log transcripts; logical-session views expose body-free counts and source references to contributing files and their successors without suggesting ownership. U3 and U4 ask about visibility and any separately labelled inferred-attribution mode.

### Rationale

No response should become a claimed answer or failure for the wrong request. References preserve access to evidence without copying unrelated clients' bodies into a logical session; file scope is navigation context, not causal evidence.

### Alternatives Considered

- **Attach by adjacency or active-request counters:** rejected because concurrency, missing completion lines and rotation provide counterexamples even when sampled logs look serial.
- **Copy unmatched bodies into every file-sharing session:** rejected because it exposes unrelated content and multiplies output.
- **Drop unmatched records:** rejected because errors and responses would disappear without disclosure.

### Consequences

**Positive:**

- The baseline makes no unsupported request-response claim.
- Direct log inspection retains unattributed evidence across every renderer.

**Negative:**

- Most reported response and warning records may remain unattributed.
- A session reader must inspect referenced files for full diagnostics, including later rotated files.

---

## Decision 4: Separate Wall Time From Assumed Instants

**Date**: 2026-11-05
**Status**: superseded by Decision 10

### Context

The reported disk grammar carries local wall time without a zone. During daylight-saving transitions, ordering or monotonicity cannot establish which offset an isolated timestamp had.

### Decision

Preserve wall time and source order, use the process zone (`TZ`, otherwise host local) as an explicit assumption, and emit an offset-bearing instant only when that interpretation is unique. Leave invalid, ambiguous or nonexistent instants and corresponding creation times absent. U8 asks whether an explicit zone should instead be mandatory.

### Rationale

This supports local inspection using the existing process setting while avoiding fabricated precision. File and line order remains reproducible regardless of timestamp interpretation.

### Alternatives Considered

- **Silently assume UTC or select a daylight-saving offset:** rejected because the source does not establish that instant.
- **Infer offset from monotonicity:** rejected because a log excerpt or missing records can contain only the second occurrence of a repeated hour.
- **Require a new dedicated zone option:** rejected for the baseline because `TZ` already provides a process-scoped override; mandatory explicit selection remains U8's alternative.

### Consequences

**Positive:**

- Wall-time evidence is preserved and the conversion assumption is visible.
- Repeated hours do not reorder requests or create invented timestamps.

**Negative:**

- Copied logs require a correct zone override for useful interpreted times.
- Some sessions lack an absolute creation time and sort under the shared unknown-time policy.

---

## Decision 5: Gate Standard Discovery Behind All Projects

**Date**: 2026-11-05
**Status**: proposed

### Context

LM Studio has a reported standard server-log directory, and logs may include full prompts from every server client. Existing single-project listings should not parse unrelated server traffic.

### Decision

Discover the standard root only in explicit all-projects scope, with CLI/environment override and empty-value disable. Gate identifier opening with the same scope, keep listings metadata-only, and disclose server-wide traffic in help and the existing network warning. U1 retains explicit LM Studio opt-in as the alternative.

### Rationale

This keeps standard-location discovery consistent with existing sources while making the expanded listing scope deliberate. Root confinement applies before candidate reads; explicit file conversion remains an independent user choice.

### Alternatives Considered

- **Require a configured root for all discovery:** rejected for the baseline because it adds setup despite a standard location; retained as U1's user choice.
- **Read logs in ordinary project listings:** rejected because the source records no project and this would add unrelated traffic and parsing work.
- **Refuse non-loopback service:** rejected because it would override an explicitly selected bind policy beyond this parser's scope.

### Consequences

**Positive:**

- Default project listings retain their existing behaviour and avoid log-body reads.
- All-projects users can discover sessions without an extra root setting.

**Negative:**

- All-projects users may discover traffic they did not expect.
- The existing `0.0.0.0` warning does not cover every possible externally reachable bind; general bind-policy changes remain outside this feature.

---

## Decision 6: Preserve Correlation Without Borrowing Projects

**Date**: 2026-11-05
**Status**: proposed

### Context

Reported thread identifiers permit consumers to compare server and client evidence. A request's prompt can quote a working directory, and several saved harness artifacts can share the same thread identifier.

### Decision

Expose the original thread identifier in transcript and listing metadata, keep server and harness sessions separately listed, and leave LM Studio's project unknown. Defer automatic web links and borrowed project attribution; U6 asks whether explicit-match links are wanted.

### Rationale

Correlation metadata does not establish an authoritative server-side project. Leaving linking separate also avoids an implicit selection rule between copied streams, resumed streams and rollouts.

### Alternatives Considered

- **Extract projects from environment-context prompts:** rejected because those are client content and may be truncated or quoted examples.
- **Borrow a matching harness session's project:** rejected for the baseline because correlation and project ownership are different assertions and multiple matches need a policy.
- **Merge or suppress the server transcript:** rejected because server-received context and client events are distinct evidence.

### Consequences

**Positive:**

- Consumers retain exact recorded correlation evidence.
- Project-scoped behaviour remains independent of client-specific prompt parsing.

**Negative:**

- Users cannot navigate to matching harness pages automatically in the baseline.
- Server work remains outside project-specific listings even when a likely client record is available.

---

## Decision 7: Keep One Direct Log Document Shape

**Date**: 2026-11-05
**Status**: proposed

### Context

One disk file can contain several interleaved logical sessions and unattributed events. The approved sibling requires one shared session-document structure for all sources.

### Decision

Convert a supplied file or stdin into one source-ordered log transcript using the common outer document, with logical-session membership on requests and log-level events where applicable. Identifier-based opening selects a logical session. U7 retains grouped sections within that same structure as a presentation alternative.

### Rationale

Direct inspection preserves physical evidence and handles stdin without another lookup. Multiple logical sessions do not require an input-dependent object/array switch or a second LM Studio JSON contract.

### Alternatives Considered

- **Emit an array for files and one object for identifiers:** rejected because it violates the common outer-shape contract.
- **Require a session identifier for direct input:** rejected because excerpts and stdin must remain inspectable before discovery.
- **Treat every request as the same conversation:** rejected because input containment does not establish session identity.

### Consequences

**Positive:**

- Consumers handle one outer session shape for every input.
- Unattributed events have a truthful home in direct views.

**Negative:**

- Direct views can interleave logical sessions.
- Additive schema fields must distinguish log-transcript identity from logical-session membership.

---

## Decision 8: Cache Unchanged Logs Within the Process

**Date**: 2026-11-05
**Status**: proposed

### Context

Grouping needs fields inside request bodies, and the existing web list refreshes every 15 seconds. The drafts' cold-time target relied on one machine and a mistaken monthly-volume claim, so it cannot serve as a portable pass/fail threshold.

### Decision

Require zero log-body reads on unchanged in-process refreshes and reflect added, changed, replaced, truncated or removed inputs on the next successful refresh. Write no persistent index; record generated workload and environment in cold and refresh benchmarks instead of imposing an uncalibrated time limit.

### Rationale

The refresh outcome is measurable and directly addresses repeated work. Cache mechanics, record limits and identifiers belong to design and must be tested together before local implementation admission.

### Alternatives Considered

- **Read every log body on each refresh:** rejected because unchanged inputs provide no new evidence.
- **Persist an index on disk:** rejected for this feature because it introduces stored identifiers and cache-format lifecycle without demonstrated need.
- **Require three seconds for a fixed byte volume everywhere:** rejected because hardware, record count and payload shape affect runtime and no reference environment was agreed.

### Consequences

**Positive:**

- Repeated web refreshes avoid unchanged body reads.
- No additional copy of log metadata is persisted by apsis.

**Negative:**

- Each new CLI process still pays cold parsing cost.
- Replacement and removal invalidation must update affected history and references while preserving surviving identities.

---

## Decision 9: Keep Unsupported Request Attribution Unknown and Inspect Full Logs Through the CLI

**ID**: 9
**Date**: 2026-11-04
**Status**: proposed; supersedes Decision 3

### Context

The supported disk grammar has no established request-scoped correlation field. Thread identifiers name conversations; response identifiers and previous-response references name context points. Neither establishes ownership of a newly logged response or diagnostic. Both fresh-context reviews found that the earlier positive matching predicate was undefined. The critique also found no existing browser route for the direct-file evidence promised by the parity tests.

### Decision

Keep all logged response and WARN/ERROR request attribution unknown for this grammar. Translate response content as log-level events in direct CLI views. Session views retain only body-free counts, source locations and CLI inspection instructions, without suggesting candidate ownership. Direct file/stdin conversion supports Markdown, standalone HTML and shared JSON; web parity applies to logical-session views. U3/U4 remain unanswered user choices.

### Rationale

An empty set of evidenced correlation fields cannot justify a positive matcher. Explicit CLI inspection preserves full evidence without adding a browser route that opens unrelated clients' log content. Any later specialised matcher needs independently established field semantics and collision rules; synthetic fields alone are insufficient evidence.

### Alternatives Considered

- **Match a thread or response identifier that is unique in the current excerpt:** rejected because uniqueness in a partial input does not make a conversation/context identifier request-scoped.
- **Infer ownership from position, model tags or apparent serial execution:** rejected because concurrency and rotation can assign another request's evidence.
- **Add a root-confined browser file viewer:** rejected for this proposal because it adds routing and full-file exposure beyond existing session views; CLI instructions provide an explicit inspection path.
- **Leave file references without an inspection contract:** rejected because browser readers would not know how to reach the retained evidence.

### Consequences

**Positive:**

- No undocumented matching field becomes invented request ownership.
- Response content remains inspectable in all supported direct-output formats.
- Session pages disclose the CLI access needed for full log inspection.

**Negative:**

- Requests have no individually attributed responses or server diagnostics in this grammar.
- Browser-only readers cannot inspect full unattributed bodies without CLI access to the log host.

---

## Decision 10: Validate Explicit Time-Zone Overrides Before Interpreting Wall Time

**ID**: 10
**Date**: 2026-11-04
**Status**: superseded by Decision 13; supersedes Decision 4

### Context

Zone-free wall times need a declared assumption. Go's Unix local-zone initialization can silently return UTC after a failed `TZ` load, so a usable zone object does not prove that an explicit override succeeded. Both fresh-context reviews identified this gap in the proposed uncertainty contract.

### Decision

Use the host zone when `TZ` is unset and UTC when it is explicitly empty. Accept UTC, loadable IANA names and absolute TZif-file paths, with an optional leading colon. An unsupported or unloadable nonempty override produces a visible warning and wall-time-only output. Preserve source order and omit instants for ambiguous, nonexistent or invalid wall times. U8 retains mandatory explicit selection as an unanswered alternative.

### Rationale

Validation prevents a failed override from acquiring a plausible but unintended offset. Existing `TZ` forms supply the override without another setting; wall times remain useful even when interpretation fails.

### Alternatives Considered

- **Use `time.Local` without validating the override:** rejected because UTC fallback hides invalid configuration.
- **Abort the whole log conversion on a zone error:** rejected because recorded content and physical ordering remain valid evidence.
- **Introduce a dedicated zone flag or silently choose UTC:** rejected for this proposal because the existing environment setting suffices, while silent substitution invents a successful assumption.

### Consequences

**Positive:**

- Invalid explicit settings cannot masquerade as successfully interpreted instants.
- Logs remain readable with source ordering and visible time uncertainty.

**Negative:**

- Parsing must validate explicit settings separately from Go's implicit local-zone initialization.
- Missing zone data can leave listing creation times absent.

---

## Decision 11: Keep All-Projects Latest Inclusive and Make Its Consequences Visible

**ID**: 11
**Date**: 2026-11-05
**Status**: proposed

### Context

Enabled LM Studio discovery can add many recent requests to an all-projects list. Its selected session may be newer than the client transcript, and follow/raw-JSON modes do not support this source. The independent review recommended selecting LM Studio through latest only when a source selector is given.

### Decision

Retain the approved sibling's inclusive all-projects latest ordering. Explain that enabled LM Studio sessions participate, report the selected source on an unsupported-mode error, and offer discovery disable or explicit opening of another session. Do not silently fall back to an older session or introduce source-filtered latest here. U1 still asks whether discovery should require additional opt-in; U2 asks how unthreaded traffic should list.

### Rationale

This spec must build on the sibling's approved scope and cannot silently change what latest means. Explicit discovery controls provide the existing way to exclude LM Studio without another selection rule.

### Alternatives Considered

- **Exclude LM Studio from latest unless --source is supplied:** rejected because it changes inclusive all-projects ordering and the sibling design currently rejects --source with latest.
- **Silently select the newest followable source:** rejected because a mode flag would change session selection without disclosing the skipped newest evidence.
- **Change the discovery recommendation automatically:** rejected because U1 is a material unanswered user choice, not an agent-owned answer.

### Consequences

**Positive:**

- Listing and latest use the same approved ordering.
- Errors explain the source and a usable remedy.

**Negative:**

- Default all-projects discovery can make latest select server traffic frequently.
- Users may need to disable LM Studio discovery or open another session explicitly before following.

---

## Decision 12: Preserve Request Parameters as Evidence Outside Conversation Messages

**ID**: 12
**Date**: 2026-11-05
**Status**: proposed

### Context

Conversation extraction alone loses tools, stream/store settings, client metadata and integration options that explain what the server received. Directing raw-JSON users to shared JSON would otherwise hide those parsed fields. Some logged parameter values, including integration headers, can be sensitive.

### Decision

Keep remaining parsed top-level request fields as parameters in shared JSON and labelled non-conversation details in Markdown/HTML, with stream/tool-name summaries where available. Preserve values and truncation markers without vendor-specific redaction. Disclose parameter/header exposure with the existing server-log help and U1 discovery choice; use synthetic fixtures only.

### Rationale

A translator should preserve logged evidence and distinguish configuration from conversation turns. Ad-hoc field redaction would silently change source content and requires a separately defined policy; it is not inferred from the source format.

### Alternatives Considered

- **Discard fields outside conversation content:** rejected because stream and tool settings explain missing output and other request behaviour.
- **Redact selected vendor header keys automatically:** rejected for this proposal because arbitrary client parameters can contain equally sensitive values, and selective removal creates unrecorded content changes.
- **Render the complete body as new conversation messages:** rejected because definitions/settings are not dialogue turns and would defeat context-prefix presentation.

### Consequences

**Positive:**

- Shared JSON remains useful for inspecting otherwise untranslated request evidence.
- All renderers expose the same parameters without inventing conversation roles.

**Negative:**

- Parameter details can be large and retain sensitive logged values.
- Discovery/exposure choices remain important and unanswered; source fixtures cannot use real values.

---

## Decision 13: Interpret Explicit Time Zones Portably Without Requiring System Zone Data

**ID**: 13
**Date**: 2026-11-05
**Status**: proposed; supersedes Decision 10

### Context

The project releases Windows binaries, whose host-local Go zone handling does not establish Unix TZ semantics. Standard IANA lookup also cannot depend on a separately installed zone database. The independent review identified that gap beyond the earlier invalid-setting check.

### Decision

Require explicit TZ interpretation on every supported platform, with valid IANA zones available without system data. Preserve unset-host and explicitly-empty-UTC semantics, accept optional colon prefixes and native absolute TZif paths, and retain the visible wall-time-only failure outcome for invalid/unloadable overrides. Preserve source order and ambiguous/nonexistent-time handling. U8 still asks whether an explicit zone should instead be mandatory.

### Rationale

The same explicit setting must not silently acquire a different interpretation on another release platform. Go's [tzdata package](https://pkg.go.dev/time/tzdata) provides a standard-library path to bundling IANA data; exact integration and platform tests belong to design.

### Alternatives Considered

- **Use each platform's time.Local behaviour directly:** rejected because it does not consistently honour the specified override.
- **Require users to install zone files separately:** rejected because standard IANA settings should work with the distributed binary.
- **Substitute UTC on load failure:** rejected because it presents failed interpretation as a successful assumption.

### Consequences

**Positive:**

- Valid explicit IANA settings work across release platforms without extra setup.
- Failed or ambiguous interpretation remains visibly distinct from recorded wall time.

**Negative:**

- Bundled zone information adds binary data and needs a defined update path.
- Native path validation and platform-specific tests are required before implementation can claim portability.
