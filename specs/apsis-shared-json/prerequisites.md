# Prerequisites for Apsis Shared JSON

The native Windows capability check is an external verification dependency for the cache slice only. It is not an unanswered product-policy choice, and PRs 1–3 may proceed without it. No native result exists in this worktree.

## During Implementation

- [ ] Before completing task 15.2, run `go test ./internal/fileidentity/... -count=1` on a native Windows host using the implementation revision under review. Run the replacement and in-place-rewrite cases on the supported filesystem being claimed: equal-size content changes with the original modification time restored must change the fingerprint, while untouched files must keep it. Test companion absence/presence and error/unsupported metadata outcomes; opening metadata must read no transcript-body bytes.
- [ ] Record the exact implementation revision or file digests, command, Windows version/architecture, filesystem, case names and pass/fail output in the feature's implementation evidence. Use only generated temporary files and generic environment descriptions; omit usernames, hostnames and private paths. Have the implementing agent check that this evidence applies to its current files before completing task 15.2 or starting task 16.

`GOOS=windows go vet ./internal/fileidentity/...`, cross-compilation and injected adapter tests are useful preceding checks but do not satisfy the native prerequisite. Without a passing native result, leave task 15.2 and task 16 incomplete and report that exact dependency. Do not add a CI job or substitute size/mtime to remove the gate; a platform-contract relaxation requires a requirements decision.
