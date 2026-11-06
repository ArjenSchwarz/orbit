package sharedjson

import (
	"cmp"
	"time"

	"github.com/arjenschwarz/orbit/internal/sessions"
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// SessionContext carries what the input, rather than the transcript, knows
// about a session.
type SessionContext struct {
	Source   string                    // source name from the resolver; else derived from the parsed format
	ID       string                    // listing identifier or filename stem; "" for stdin
	Project  string                    // input's project, such as an exec stream file's containing repository
	Resolved *sessions.SessionMetadata // resolver metadata; nil for file and stdin input
}

// formatSources names the source each parsed format belongs to. The Claude
// fallback for unrecognised input (FormatUnknown) has no source.
var formatSources = map[transcript.Format]string{
	transcript.FormatClaude:    sessions.SourceClaude,
	transcript.FormatCodex:     sessions.SourceCodex,
	transcript.FormatCopilot:   sessions.SourceCopilot,
	transcript.FormatKiro:      sessions.SourceKiroCLI,
	transcript.FormatKiroIDE:   sessions.SourceKiroIDE,
	transcript.FormatCodexExec: sessions.SourceCodexExec,
}

// Session projects a parse result into a session document. It reads parsed
// data only and never calls renderer helpers, so content keeps its full length.
func Session(res *transcript.ParseResult, ctx SessionContext) SessionDocument {
	doc := SessionDocument{
		SchemaVersion: SchemaVersion,
		Kind:          "session",
		Session:       sessionMetadata(res, ctx),
		Entries:       make([]Entry, 0, len(res.Entries)),
		Warnings:      make([]Warning, 0, len(res.Warnings)),
	}
	for _, e := range res.Entries {
		doc.Entries = append(doc.Entries, entry(e))
	}
	for _, w := range res.Warnings {
		doc.Warnings = append(doc.Warnings, warning(w))
	}
	return doc
}

// sessionMetadata prefers parsed values; resolver metadata fills only what the
// transcript does not record.
func sessionMetadata(res *transcript.ParseResult, ctx SessionContext) Metadata {
	var parsed transcript.ParseResultMetadata
	if res.Metadata != nil {
		parsed = *res.Metadata
	}
	var resolved sessions.SessionMetadata
	if ctx.Resolved != nil {
		resolved = *ctx.Resolved
	}

	m := Metadata{
		Source:    cmp.Or(ctx.Source, resolved.Source, formatSources[res.Format]),
		ID:        cmp.Or(ctx.ID, resolved.ID),
		ThreadID:  parsed.ThreadID,
		CreatedAt: createdAt(res.Entries, parsed.StartedAt, resolved),
		Project:   cmp.Or(parsed.Project, ctx.Project, resolved.Project),
		Model:     cmp.Or(parsed.FirstModel, firstEntryValue(res.Entries, func(e transcript.Entry) string { return e.Model })),
		Provider:  cmp.Or(parsed.FirstProvider, firstEntryValue(res.Entries, func(e transcript.Entry) string { return e.Provider })),
		Origin:    parsed.Origin,
	}
	if parsed.TotalCost != nil {
		// Renderers show "credits" when a parser records no unit.
		m.Cost = &Cost{Amount: *parsed.TotalCost, Unit: cmp.Or(parsed.CostUnit, "credits")}
	}
	return m
}

// createdAt follows Decision 30: a parsed session-start record, then a
// recorded resolver creation time, then the first entry timestamp. A
// filesystem modification time never qualifies.
func createdAt(entries []transcript.Entry, startedAt string, resolved sessions.SessionMetadata) string {
	if validTime(startedAt) {
		return startedAt
	}
	if !resolved.DatabaseCreatedAt.IsZero() {
		return resolved.DatabaseCreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !resolved.TimeFromFilesystem && !resolved.CreatedAt.IsZero() {
		return resolved.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return firstEntryValue(entries, func(e transcript.Entry) string {
		if validTime(e.Timestamp) {
			return e.Timestamp
		}
		return ""
	})
}

func firstEntryValue(entries []transcript.Entry, value func(transcript.Entry) string) string {
	for _, e := range entries {
		if v := value(e); v != "" {
			return v
		}
	}
	return ""
}

// validTime reports whether ts is an RFC3339 timestamp, with or without fractional seconds.
func validTime(ts string) bool {
	_, err := time.Parse(time.RFC3339Nano, ts)
	return err == nil
}

func entry(e transcript.Entry) Entry {
	out := Entry{
		ID:       e.RecordID,
		Role:     e.Type,
		Model:    e.Model,
		Provider: e.Provider,
		Turn:     e.Turn,
		Content:  []any{},
	}
	if validTime(e.Timestamp) {
		out.Timestamp = e.Timestamp
	}
	if e.Message != nil {
		out.Role = cmp.Or(e.Message.Role, e.Type)
		for _, item := range e.Message.Content {
			out.Content = append(out.Content, block(item))
		}
	}
	return out
}

func block(item transcript.ContentItem) any {
	common := BlockCommon{ItemID: item.ItemID, Status: item.Status, Incomplete: item.Incomplete}
	switch item.Type {
	case "text":
		return TextBlock{Type: "text", BlockCommon: common, Text: item.Text}
	case "thinking":
		return TextBlock{Type: "thinking", BlockCommon: common, Text: item.Thinking}
	case "tool_use":
		return ToolUseBlock{Type: "tool_use", BlockCommon: common, ID: item.ID, Name: item.Name, Input: item.Input}
	case "tool_result":
		return ToolResultBlock{Type: "tool_result", BlockCommon: common, ToolUseID: item.ToolUseID,
			Content: item.Content, IsError: item.IsError, ExitCode: item.ExitCode}
	case "plan":
		items := make([]PlanItem, 0, len(item.Plan))
		for _, p := range item.Plan {
			items = append(items, PlanItem{Text: p.Text, Completed: p.Completed})
		}
		return PlanBlock{Type: "plan", BlockCommon: common, Items: items}
	case "error":
		return ErrorBlock{Type: "error", BlockCommon: common, Message: item.Text}
	case "event":
		return EventBlock{Type: "event", BlockCommon: common, EventType: item.EventType, Payload: item.Payload}
	case "usage":
		if item.Usage != nil {
			return UsageBlock{Type: "usage", BlockCommon: common, Basis: item.Usage.Basis, Counters: item.Usage.Counters}
		}
	}
	return OtherBlock{Type: "other", BlockCommon: common, SourceType: item.Type, Text: item.Text}
}

// warning projects a parse warning, naming its position by unit (Q75).
func warning(w transcript.ParseWarning) Warning {
	out := Warning{Code: w.Code, Message: w.Message}
	switch w.Unit {
	case transcript.WarningUnitChatMessage:
		out.ChatMessage = w.Line
	case transcript.WarningUnitExecutionAction:
		out.ExecutionAction = w.Line
	default:
		out.Line = w.Line
	}
	return out
}
