---
references:
    - specs/auto-consolidate/smolspec.md
---
# Auto-Consolidate Feature

## Configuration

- [x] 1. Config struct supports auto-consolidate and post-consolidate-command settings

- [x] 2. CLI accepts --auto-consolidate, --no-auto-consolidate, and --allow-dirty flags with proper validation

- [x] 3. Orbit config receives auto-consolidate settings from CLI and config file

## Implementation

- [x] 4. Auto-consolidation runs on recommended variant after successful comparison

- [x] 5. Auto-consolidation skips gracefully when preconditions not met (single variant, dirty worktree, no improvements)

- [x] 6. Post-consolidate-command executes in variant worktree after consolidation completes

- [x] 7. Auto-consolidation failures are non-fatal and variant run continues to report generation

## Verification

- [x] 8. Unit tests verify flag parsing, validation, and config resolution

- [x] 9. Integration test confirms end-to-end variant run with auto-consolidate produces expected outcome
