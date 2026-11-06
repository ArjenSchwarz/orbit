package transcript

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"strings"
)

// Rendering of the content kinds Codex exec streams add: plans, errors,
// generic events, token usage, exit codes and incomplete items.

// incompleteSuffix marks an item that started but never completed.
func incompleteSuffix(item *ContentItem) string {
	if item.Incomplete {
		return " (incomplete)"
	}
	return ""
}

// resultLabel labels a tool result, including a recorded exit code.
func resultLabel(item *ContentItem) string {
	if item.ExitCode != nil {
		return fmt.Sprintf("Result (exit code %d):", *item.ExitCode)
	}
	return "Result:"
}

// usageLabel names what a usage block's counters measure.
func usageLabel(basis string) string {
	switch basis {
	case UsageBasisThreadTotal:
		return "Token usage (thread totals at turn completion)"
	case "":
		return "Token usage"
	}
	return "Token usage (" + basis + ")"
}

// usageValues lists counters in source order as "name value" pairs.
func usageValues(usage *Usage) string {
	parts := make([]string, 0, len(usage.Counters))
	for _, c := range usage.Counters {
		parts = append(parts, c.Name+" "+string(c.Value))
	}
	return strings.Join(parts, " · ")
}

// indentedPayload pretty-prints a payload without changing its values.
func indentedPayload(payload json.RawMessage) string {
	var buf bytes.Buffer
	if json.Indent(&buf, payload, "", "  ") != nil {
		return string(payload)
	}
	return buf.String()
}

type fileChange struct{ kind, path string }

// fileChanges reads a file_change tool input's changes.
func fileChanges(input map[string]any) []fileChange {
	list, _ := input["changes"].([]any)
	changes := make([]fileChange, 0, len(list))
	for _, c := range list {
		m, _ := c.(map[string]any)
		kind, _ := m["kind"].(string)
		path, _ := m["path"].(string)
		changes = append(changes, fileChange{kind: kind, path: path})
	}
	return changes
}

func planCheckbox(p PlanItem) string {
	if p.Completed {
		return "[x]"
	}
	return "[ ]"
}

// formatExecContentMarkdown renders a new content kind; ok is false for other kinds.
func formatExecContentMarkdown(item *ContentItem) (string, bool) {
	var sb strings.Builder
	icon := ""
	if item.Incomplete {
		icon = "⏳ "
	}
	switch item.Type {
	case "plan":
		fmt.Fprintf(&sb, "<details open>\n<summary>%s📋 Plan%s</summary>\n\n", icon, incompleteSuffix(item))
		for _, p := range item.Plan {
			fmt.Fprintf(&sb, "- %s %s\n", planCheckbox(p), p.Text)
		}
		sb.WriteString("\n</details>\n\n")
	case "error":
		fmt.Fprintf(&sb, "❌ **Error:** %s%s\n\n", item.Text, incompleteSuffix(item))
	case "event":
		fmt.Fprintf(&sb, "<details>\n<summary>%s📦 Event: %s%s</summary>\n\n", icon, escapeSummary(item.EventType), incompleteSuffix(item))
		if len(item.Payload) > 0 {
			sb.WriteString("```json\n")
			sb.WriteString(indentedPayload(item.Payload))
			sb.WriteString("\n```\n\n")
		}
		sb.WriteString("</details>\n\n")
	case "usage":
		if item.Usage != nil {
			fmt.Fprintf(&sb, "**%s:** %s\n\n", usageLabel(item.Usage.Basis), usageValues(item.Usage))
		}
	default:
		return "", false
	}
	return sb.String(), true
}

// formatExecContentHTML renders a new content kind; ok is false for other kinds.
func formatExecContentHTML(item *ContentItem) (string, bool) {
	var sb strings.Builder
	switch item.Type {
	case "plan":
		sb.WriteString("                <details class=\"tool-collapsible\" open>\n")
		fmt.Fprintf(&sb, "                    <summary>%s</summary>\n", summaryHTML(item, "📋", "Plan"))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		sb.WriteString("                        <div class=\"tool-input-section\">\n")
		sb.WriteString("                            <ul class=\"todo-list\">\n")
		for _, p := range item.Plan {
			fmt.Fprintf(&sb, "                                <li>%s %s</li>\n", planCheckbox(p), stdhtml.EscapeString(p.Text))
		}
		sb.WriteString("                            </ul>\n")
		sb.WriteString("                        </div>\n")
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	case "error":
		fmt.Fprintf(&sb, "                <div class=\"tool-use error\"><span class=\"icon\">❌</span> <strong>Error:</strong> %s%s</div>\n",
			stdhtml.EscapeString(item.Text), incompleteSuffix(item))
	case "event":
		sb.WriteString("                <details class=\"tool-collapsible\">\n")
		fmt.Fprintf(&sb, "                    <summary>%s</summary>\n", summaryHTML(item, "📦", "Event: "+item.EventType))
		sb.WriteString("                    <div class=\"tool-content\">\n")
		if len(item.Payload) > 0 {
			sb.WriteString("                        <pre><code>")
			sb.WriteString(stdhtml.EscapeString(indentedPayload(item.Payload)))
			sb.WriteString("</code></pre>\n")
		}
		sb.WriteString("                    </div>\n")
		sb.WriteString("                </details>\n")
	case "usage":
		if item.Usage != nil {
			fmt.Fprintf(&sb, "                <div class=\"usage\">%s: %s</div>\n",
				stdhtml.EscapeString(usageLabel(item.Usage.Basis)), stdhtml.EscapeString(usageValues(item.Usage)))
		}
	default:
		return "", false
	}
	return sb.String(), true
}

// summaryHTML is a block summary: the kind's icon and label, or the
// incomplete marker before them.
func summaryHTML(item *ContentItem, icon, label string) string {
	if item.Incomplete {
		return fmt.Sprintf("<span class=\"icon\">⏳</span> %s %s (incomplete)", icon, stdhtml.EscapeString(label))
	}
	return fmt.Sprintf("<span class=\"icon\">%s</span> %s", icon, stdhtml.EscapeString(label))
}
