package transcript

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// parseTimestamp parses an RFC3339 or RFC3339Nano timestamp string.
func parseTimestamp(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}

// formatUnixMilliTimestamp converts epoch milliseconds to an RFC3339 UTC string.
// Returns empty string for zero or negative values.
func formatUnixMilliTimestamp(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// FormatTimestampMarkdown parses an RFC3339 timestamp and re-formats it
// in the system's local timezone as RFC3339.
// Returns empty string if the timestamp is empty or unparseable.
func FormatTimestampMarkdown(ts string) string {
	t, ok := parseTimestamp(ts)
	if !ok {
		return ""
	}
	return t.In(time.Local).Format(time.RFC3339)
}

// FormatTimestampHTML returns a <time> element with ISO datetime attribute
// and a server-rendered RFC3339 fallback.
// Returns empty string if the timestamp is empty or unparseable.
func FormatTimestampHTML(ts string) string {
	t, ok := parseTimestamp(ts)
	if !ok {
		return ""
	}
	utc := t.UTC().Format(time.RFC3339)
	local := t.In(time.Local).Format(time.RFC3339)
	return fmt.Sprintf(`<time datetime="%s">%s</time>`, html.EscapeString(utc), html.EscapeString(local))
}

// FormatMessageMetaMarkdown builds the metadata suffix for a Markdown header.
// Example: " · 2026-03-12T14:32:05+11:00 · claude-opus"
// Returns empty string if no metadata is available.
func FormatMessageMetaMarkdown(timestamp, model string) string {
	var parts []string
	if formatted := FormatTimestampMarkdown(timestamp); formatted != "" {
		parts = append(parts, formatted)
	}
	if model != "" {
		parts = append(parts, model)
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

// FormatMessageMetaHTML builds the metadata span for an HTML header.
// Returns empty string if no metadata is available.
func FormatMessageMetaHTML(timestamp, model string) string {
	var parts []string
	if formatted := FormatTimestampHTML(timestamp); formatted != "" {
		parts = append(parts, formatted)
	}
	if model != "" {
		parts = append(parts, fmt.Sprintf("<span>%s</span>", html.EscapeString(model)))
	}
	if len(parts) == 0 {
		return ""
	}
	return `<span class="message-meta">` + strings.Join(parts, `<span class="meta-separator">·</span>`) + `</span>`
}
