---
references:
    - specs/opencode-agent/smolspec.md
---
# OpenCode Agent Support

## Implementation

- [ ] 1. Create OpenCode agent package structure

- [ ] 2. Implement Agent interface methods (Name, CLICommand, IsInstalled, Version)

- [ ] 3. Implement buildArgs() with --format json and model support

- [ ] 4. Implement Run() and Resume() execution methods

- [ ] 5. Implement session discovery from ~/.local/share/opencode/storage/message/

- [ ] 6. Create error classifier with JSON validation and pattern matching

- [ ] 7. Register agent in registry via init()

## Testing

- [ ] 8. Add unit tests for buildArgs() covering model and resume flags

- [ ] 9. Add unit tests for version parsing (handling INFO log prefix)

- [ ] 10. Add unit tests for error classifier (JSON vs plaintext detection)
