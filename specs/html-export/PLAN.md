# HTML Export Feature Plan

## Overview

Add HTML export capability to the `apsis` CLI tool and Orbit's log manager, allowing session transcripts to be rendered as styled HTML documents in addition to Markdown.

## Current State

- `apsis` CLI converts JSONL session transcripts to Markdown only
- `internal/transcript` package has `RenderMarkdown()` function
- Orbit's log manager generates `.md` transcripts for phases

## Implementation

### 1. Add `--format` flag to apsis CLI

**File:** `cmd/apsis/main.go`

- Add `Format string` field to `Config` struct
- Add `-f, --format` flag accepting values:
  - `md` or `markdown` (default)
  - `html`
- Update `convert()` function to call appropriate renderer
- Update `printUsage()` with new flag documentation

### 2. Create HTML renderer

**File:** `internal/transcript/html.go`

Create `RenderHTML(entries []Entry, opts RenderOptions) string` function that:

- Generates a complete HTML document with embedded CSS
- Maps transcript structure to semantic HTML:
  - Document header with title and session ID
  - User messages in styled sections
  - Assistant messages with:
    - Collapsible thinking blocks (`<details>/<summary>`)
    - Text content
    - Tool use blocks with JSON syntax highlighting
    - Tool results with success/error styling
- Includes responsive CSS styling
- Reuses `truncateString()` for consistency with Markdown output

### 3. Update Orbit's log manager

**File:** `internal/logs/manager.go`

- Update `generateMarkdownTranscript()` to also generate HTML
- Update `generatePostCompletionMarkdownTranscript()` to also generate HTML
- Generate `.html` files alongside `.md` files

### 4. Add tests

**File:** `internal/transcript/html_test.go`

- Test basic HTML structure generation
- Test all content types (user, assistant, thinking, tool_use, tool_result)
- Test empty/edge cases
- Test truncation behavior

## File Changes Summary

| File | Change |
|------|--------|
| `cmd/apsis/main.go` | Add `--format` flag and format selection logic |
| `internal/transcript/html.go` | New file - HTML renderer implementation |
| `internal/transcript/html_test.go` | New file - HTML renderer tests |
| `internal/logs/manager.go` | Add HTML generation alongside Markdown |

## HTML Output Structure

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{Title}</title>
    <style>
        /* Embedded CSS for styling */
    </style>
</head>
<body>
    <header>
        <h1>{Title}</h1>
        <p class="session-id">Session ID: <code>{SessionID}</code></p>
    </header>
    <main>
        <section class="message user">...</section>
        <section class="message assistant">...</section>
        ...
    </main>
</body>
</html>
```

## Testing

Run tests with:
```bash
go test ./internal/transcript -run TestHTML
go test ./cmd/apsis -run TestFormat
```
