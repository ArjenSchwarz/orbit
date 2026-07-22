# Bugfix Report: Claude Session Limit Message

**Date:** 2026-07-22
**Status:** Fixed

## Description of the Issue

Claude Code now reports usage exhaustion as `"You've hit your session limit · resets 4pm (Australia/Melbourne)"`. Orbit only recognizes the older `"You've hit your limit"` prefix, so it does not wait until the supplied reset time.

**Reproduction steps:**
1. Pass the new Claude Code session-limit message to the Claude error classifier.
2. Let the classifier inspect the message and reset time.
3. Observe that it returns `ErrorClassUnknown` instead of `ErrorClassRateLimitWait`.

**Impact:** Orbit can stop an in-progress Claude Code run when the session limit is reached instead of waiting and resuming after the reset.

## Investigation Summary

The failure occurs in the phrase-detection guard in `Classifier.Classify`; the existing reset-time parser already accepts the remainder of the new message.

- **Symptoms examined:** The new message is classified as unknown with a zero retry delay.
- **Code inspected:** `internal/agents/claudecode/errors.go`, its callers in the Claude agent and shared retry path, and `internal/agents/claudecode/errors_test.go`.
- **Hypotheses tested:** A regression test confirmed that `parseUsageLimitReset` can parse the new reset suffix, but `Classify` never calls it because `"hit your limit"` is not present in `"hit your session limit"`.

## Discovered Root Cause

`Classifier.Classify` uses a literal substring guard tied to Claude Code's old wording. Claude inserted the word `session`, causing the guard to fail before the format-independent reset parser runs.

**Defect type:** Missing external-message variant handling

**Why it occurred:** The classifier relied on a specific upstream phrase; test coverage contained only that phrase; Claude changed the phrase while preserving the reset-time syntax; Orbit consequently fell through to unknown-error handling instead of its rate-limit wait path.

**Contributing factors:** Claude Code's human-readable limit message is an external interface that can change independently of Orbit.

## Resolution for the Issue

**Changes made:**
- `internal/agents/claudecode/errors.go` - Recognize `"hit your session limit"` before parsing the reset time, while retaining support for the older phrase.
- `internal/agents/claudecode/errors_test.go` - Add the new Claude Code wording to the classifier regression cases.
- `CLAUDE.md` - Document the current wording and compatibility with the older message.

**Approach rationale:** An explicit second accepted phrase is the smallest safe change. It reuses the existing reset-time parser and avoids treating unrelated messages containing `"resets <time>"` as Claude session limits.

**Alternatives considered:**
- Parse every error containing a reset time - Rejected because unrelated reset messages could be misclassified as session limits.
- Replace the guard with a looser regular expression - Rejected because two known literal variants are clearer and sufficient.

## Regression Test

**Test file:** `internal/agents/claudecode/errors_test.go`
**Test name:** `TestClassifier_Classify_UsageLimit/hit_your_session_limit`

**What it verifies:** The new session-limit message is classified as `ErrorClassRateLimitWait` with a positive retry delay.

**Run command:** `go test ./internal/agents/claudecode -run TestClassifier_Classify_UsageLimit`

## Affected Files

| File | Change |
|------|--------|
| `internal/agents/claudecode/errors.go` | Recognize the new session-limit phrase |
| `internal/agents/claudecode/errors_test.go` | Add the regression case |
| `CLAUDE.md` | Update the documented Claude Code message |

## Verification

**Automated:**
- [x] Regression test passes
- [x] All non-server packages pass
- [x] Linters/validators pass
- [ ] Full `make test` passes in this sandbox; listener tests are blocked by `bind: operation not permitted`

**Manual verification:**
- Confirmed the exact reported message produces `ErrorClassRateLimitWait` with a positive reset delay through the regression test.

## Prevention

**Recommendations to avoid similar bugs:**
- Keep explicit regression cases for each observed Claude Code limit-message variant.

## Related

- Supersedes no existing bugfix; the older message remains supported.
