---
references:
    - specs/opencode-agent/smolspec.md
---
# OpenCode Agent Support

## Implementation

- [x] 1. Create OpenCode agent package structure

- [x] 2. Implement Agent interface methods (Name, CLICommand, IsInstalled, Version)

- [x] 3. Implement buildArgs() with --format json and model support

- [x] 4. Implement Run() and Resume() execution methods

- [x] 5. Implement session discovery from ~/.local/share/opencode/storage/message/

- [x] 6. Create error classifier with JSON validation and pattern matching

- [x] 7. Register agent in registry via init()

## Testing

- [x] 8. Add unit tests for buildArgs() covering model and resume flags

- [x] 9. Add unit tests for version parsing (handling INFO log prefix)

- [x] 10. Add unit tests for error classifier (JSON vs plaintext detection)
