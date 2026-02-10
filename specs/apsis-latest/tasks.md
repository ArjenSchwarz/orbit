---
references:
    - specs/apsis-latest/smolspec.md
---
# Apsis Latest Session

## Core Implementation

- [ ] 1. Add resolveLatestSession() that uses Lister.ListAll() and picks the newest session

- [ ] 2. Integrate latest keyword into run() — check cfg.Input == "latest" before isFilePath() and route to resolveLatestSession()

- [ ] 3. Print resolved session info (source, ID, timestamp) to stderr

- [ ] 4. Verify latest resolves correctly with all output formats (-f md, html, json) and -o flag

## Follow Mode Support

- [ ] 5. Handle latest in follow mode path — resolve session, validate source is file-backed, error for Kiro CLI

- [ ] 6. Verify apsis latest -F works for file-backed sessions and errors for non-file-backed sessions

## Polish

- [ ] 7. Update printUsage() help text and examples to include apsis latest

- [ ] 8. Add tests for latest keyword resolution, empty session list error, and isFilePath shadowing protection
