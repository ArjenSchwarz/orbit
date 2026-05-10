# Decision Log: Finalize Show and Verify

## Decision 1: Mismatch produces a warning, not a hard error

**Date**: 2026-05-10
**Status**: accepted

### Context

When `orbit finalize --variant N` is invoked after `orbit consolidate` was run on a different variant, the user has likely lost track of which variant they intended to ship. The finalize command currently has no awareness of the consolidation log, so this footgun goes undetected. T-1197 asks for verification.

### Decision

When the variant passed to `--variant` does not match the most recent `chosen_variant_id` in `consolidation-log.json`, finalize prints a `Warning:` line naming both IDs and the consolidation timestamp, then proceeds through the existing `y/N` confirmation. No new flag is introduced and finalize never refuses to run on this basis.

### Rationale

Hard-failing or requiring a `--force-mismatch` flag would block legitimate workflows where the user intentionally finalizes a different variant than the one consolidated (e.g., they reviewed the consolidated diff and decided to ship the un-consolidated original instead). A warning surfaces the discrepancy without removing user agency. The existing `y/N` prompt — and `--force` for CI — already give the user a clear acknowledge-or-abort decision point.

### Alternatives Considered

- **Hard error**: Refuse to finalize on mismatch, requiring the user to consolidate or pick the matching variant - Rejected because it blocks legitimate "I changed my mind" workflows.
- **Warn + require `--force-mismatch`**: Print warning and require a new flag to proceed - Rejected because the existing `y/N` prompt already serves this purpose; adding a flag duplicates UX without adding safety.

### Consequences

**Positive:**
- No new flag surface area to document or maintain.
- Users who intentionally finalize a different variant are not blocked.
- The warning still appears under `--force` (printed before the prompt-skip path), so CI logs surface mismatches.

**Negative:**
- A user who proceeds through the `y/N` prompt without reading the warning could still finalize the wrong variant. Acceptable given the prompt already requires deliberate `y` input.

---

## Decision 2: Skip verification silently when no consolidation log exists

**Date**: 2026-05-10
**Status**: accepted

### Context

Consolidation is optional in the Orbit workflow — a user can run `orbit finalize` without ever running `orbit consolidate`. In that case there is no `consolidation-log.json` to read.

### Decision

When `consolidation-log.json` is missing, unreadable, or has no entries, finalize prints no warning and continues normally. Read errors are not surfaced to the user.

### Rationale

A "no consolidation log found" warning would fire on every finalize that wasn't preceded by consolidation, which is a normal and supported workflow. The verification feature exists to catch user error in the consolidate-then-finalize path; it should be silent when consolidation simply wasn't part of the workflow.

### Alternatives Considered

- **Print "no consolidation log found, skipping verification"**: Always tell the user what happened - Rejected as noise; the absence of a log is the default state for non-consolidation workflows.
- **Surface JSON parse errors**: Help the user notice a corrupt log - Rejected because it conflates "log absent" and "log corrupt" into a verification failure that blocks finalize for an unrelated reason.

### Consequences

**Positive:**
- Workflows that skip consolidation see no behavioural change.
- No false alarms for users who have never run consolidation.

**Negative:**
- A genuinely corrupt `consolidation-log.json` is silently ignored. Acceptable: the consolidate command itself will surface the corruption next time it runs, and finalize is not the right place to diagnose log integrity.

---
