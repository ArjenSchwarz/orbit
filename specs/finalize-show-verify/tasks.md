---
references:
    - specs/finalize-show-verify/smolspec.md
---
# Finalize Show and Verify (T-1197)

- [x] 1. Render agent info in finalize preamble <!-- id:pghg6mg -->
  - Implement conditional formatting in cmd/orbit/finalize.go so the preamble shows `Agent: <alias> (<type>, model: <model>)` when all fields are present.
  - Omit any individual missing field cleanly (no empty parens, dangling commas, or `model: ` with no value).
  - Fall back to `Agent: unknown` when all three of Agent, AgentType, Model are empty on the variant.
  - Verify by running orbit finalize against a fixture variant manually or by running the tests added in task 2.
  - Reference: smolspec.md Requirements 1-3, Implementation Approach point 1 and Format the agent info bullet.
  - References: specs/finalize-show-verify/smolspec.md

- [x] 2. Verify agent info display behaviour with table-driven tests <!-- id:pghg6mh -->
  - Add cmd/orbit/finalize_test.go with table-driven tests.
  - Cases: all three fields populated; only Agent populated; Agent + AgentType (no Model); only Model populated; all empty (Agent: unknown).
  - Use the temp repo + spec dir pattern from cmd/orbit/subdir_test.go and the inline os.Pipe / os.Stdout = w stdout-capture pattern from cmd/orbit/status_test.go:508-517.
  - All tests must pass.
  - Reference: smolspec.md Implementation Approach For tests bullet.
  - Blocked-by: pghg6mg (Render agent info in finalize preamble)
  - References: specs/finalize-show-verify/smolspec.md

- [x] 3. Verify chosen variant against consolidation log <!-- id:pghg6mi -->
  - In cmd/orbit/finalize.go, read consolidation-log.json via consolidation.NewLogger(filepath.Join(specDir, ".orbit")) and Read() (mirroring cmd/orbit/consolidate.go:240).
  - When the last entrys ChosenVariantID differs from the requested --variant, print a `Warning:` line to stdout naming both IDs and the prior consolidation timestamp formatted as RFC3339.
  - Place the lookup BEFORE the `if !*force { ... }` block so the warning prints in --force mode.
  - Treat missing file, JSON parse failure, or empty Entries slice as no-verification: print nothing and continue.
  - Verify manually or via task 4.
  - Reference: smolspec.md Requirements 4-7, Implementation Approach point 1 (placement note) and Format the mismatch warning / Treat any error from Read() bullets.
  - Blocked-by: pghg6mh (Verify agent info display behaviour with table-driven tests)
  - References: specs/finalize-show-verify/smolspec.md

- [x] 4. Verify mismatch warning behaviour with table-driven tests <!-- id:pghg6mj -->
  - Extend cmd/orbit/finalize_test.go with table-driven cases.
  - Cases: mismatch fires warning naming both IDs and RFC3339 timestamp; matching entry produces no warning; missing log file produces no warning; corrupt JSON log produces no warning; empty Entries slice produces no warning; warning prints when --force is passed.
  - Write fixture consolidation-log.json files into a temp .orbit directory.
  - All tests must pass.
  - Reference: smolspec.md Implementation Approach point 2 (test scope) and For tests bullet.
  - Blocked-by: pghg6mi (Verify chosen variant against consolidation log)
  - References: specs/finalize-show-verify/smolspec.md

- [x] 5. Lint and full package test suite pass <!-- id:pghg6mk -->
  - Run `make lint` and `make test` from the repo root; both must exit zero.
  - Fix any issues introduced by tasks 1-4 (for example modernize warnings or formatting).
  - No production code changes outside what tasks 1-4 require.
  - Reference: project Makefile.
  - Blocked-by: pghg6mj (Verify mismatch warning behaviour with table-driven tests)
