---
references:
    - specs/kiro-transcript-improvements/smolspec.md
---
# Kiro Transcript Improvements

## Core Infrastructure

- [x] 1. ParseResult carries format-specific metadata for cost information

- [x] 2. RenderOptions supports cost display fields without signature changes

## Kiro Parser Enhancements

- [x] 3. Json variant in tool results is parsed alongside Text variant

- [x] 4. Kiro parser populates cost metadata from usage_info

## Renderer Updates

- [x] 5. Markdown renderer displays cost in transcript header

- [x] 6. HTML renderer displays cost in transcript header

## Integration

- [ ] 7. Apsis CLI passes metadata from ParseResult to RenderOptions

- [ ] 8. All existing tests pass and new functionality is covered
