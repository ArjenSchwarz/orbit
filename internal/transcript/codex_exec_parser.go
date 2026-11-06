package transcript

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// execMaxLineBytes is the longest exec stream line apsis decodes; longer lines
// are skipped with an oversized_line warning, matching the other scanners' limit.
const execMaxLineBytes = 10 * 1024 * 1024

// execRecord is one translated unit of a turn: an item's final state, or a
// turn-level event.
type execRecord struct {
	event     string          // source event type of the final state
	fields    json.RawMessage // compact item JSON for items; compact event JSON otherwise
	itemType  string          // items only: the item's type
	itemID    string          // items only: the valid source item ID, else ""
	isItem    bool            // the record came from an item.* event with an item object
	completed bool            // items only: an item.completed event was seen
	line      int             // physical line of the final state
}

// execItemKey keys an item within its turn. Items without a valid ID key by
// their physical line, so unrelated invalid items never merge.
type execItemKey struct {
	id   string
	line int
}

type execTurn struct {
	number  int
	records []*execRecord
	items   map[execItemKey]*execRecord
}

type execParser struct {
	turns    []*execTurn
	warnings []ParseWarning
	threadID string
}

// ExecProbeOutcome classifies a stream by its first JSON object (Q56).
type ExecProbeOutcome string

const (
	// ExecProbeExec: the first object is thread.started with a nonempty string thread_id.
	ExecProbeExec ExecProbeOutcome = "exec"
	// ExecProbeWithoutThread: the first object is another exec event; parsing it must be forced.
	ExecProbeWithoutThread ExecProbeOutcome = "exec-without-thread"
	// ExecProbeOther: the stream is not an exec stream, or holds no JSON object.
	ExecProbeOther ExecProbeOutcome = "other"
)

// ExecProbe is the result of ProbeExecStream.
type ExecProbe struct {
	Outcome  ExecProbeOutcome
	ThreadID string // set for ExecProbeExec only
}

// execEventTypes are the pinned 0.166.0 ThreadEvent types.
var execEventTypes = map[string]bool{
	"thread.started": true, "turn.started": true, "turn.completed": true, "turn.failed": true,
	"item.started": true, "item.updated": true, "item.completed": true, "error": true,
}

// ProbeExecStream reads whole lines until the first line that parses as a
// JSON object, skipping a leading BOM and blank, malformed and over-10-MiB
// lines, and classifies the stream by that object (requirement 2.1). It
// returns an error only when reading fails.
func ProbeExecStream(r io.Reader) (ExecProbe, error) {
	// A small buffer suffices: readExecLine joins lines longer than it, and
	// discovery probes many files per pass.
	br := bufio.NewReader(r)
	for lineNum := 1; ; lineNum++ {
		line, oversized, err := readExecLine(br)
		if errors.Is(err, io.EOF) {
			return ExecProbe{Outcome: ExecProbeOther}, nil
		}
		if err != nil {
			return ExecProbe{}, fmt.Errorf("read exec stream: %w", err)
		}
		if lineNum == 1 {
			line = stripBOM(line)
		}
		var fields map[string]json.RawMessage
		if oversized || json.Unmarshal(bytes.TrimSpace(line), &fields) != nil || fields == nil {
			continue
		}
		event, _ := jsonString(fields["type"])
		if !execEventTypes[event] {
			return ExecProbe{Outcome: ExecProbeOther}, nil
		}
		if threadID, ok := jsonString(fields["thread_id"]); event == "thread.started" && ok && threadID != "" {
			return ExecProbe{Outcome: ExecProbeExec, ThreadID: threadID}, nil
		}
		return ExecProbe{Outcome: ExecProbeWithoutThread}, nil
	}
}

// ParseCodexExec parses a Codex CLI `codex exec --json` event stream. Each turn
// becomes one assistant entry holding the final state of its items in first
// appearance order. It returns an error only when reading fails; malformed or
// oversized lines become warnings and parsing continues.
func ParseCodexExec(r io.Reader) (*ParseResult, error) {
	p := &execParser{turns: []*execTurn{newExecTurn(0)}}
	br := bufio.NewReaderSize(r, 64*1024)
	for lineNum := 1; ; lineNum++ {
		line, oversized, err := readExecLine(br)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read exec stream: %w", err)
		}
		if lineNum == 1 {
			line = stripBOM(line)
		}
		if oversized {
			p.warn(lineNum, WarningOversizedLine, "line exceeds 10 MiB; skipped")
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		p.parseLine(line, lineNum)
	}

	result := &ParseResult{
		Entries:  []Entry{},
		Metadata: &ParseResultMetadata{ThreadID: p.threadID},
		Format:   FormatCodexExec,
	}
	for _, turn := range p.turns {
		if len(turn.records) == 0 {
			continue
		}
		var content []ContentItem
		for _, rec := range turn.records {
			content = append(content, p.translate(rec, turn.number)...)
		}
		result.Entries = append(result.Entries, Entry{
			Type:    "assistant",
			Message: &Message{Role: "assistant", Content: content},
			Turn:    turn.number,
		})
	}
	// Translation adds item warnings after the scan's line warnings.
	slices.SortStableFunc(p.warnings, func(a, b ParseWarning) int { return a.Line - b.Line })
	result.Warnings = append([]ParseWarning{}, p.warnings...)
	return result, nil
}

func newExecTurn(number int) *execTurn {
	return &execTurn{number: number, items: make(map[execItemKey]*execRecord)}
}

// readExecLine returns the next physical line, including its newline. A line
// longer than execMaxLineBytes is discarded through its newline and reported
// as oversized. It returns io.EOF only when no bytes remain.
func readExecLine(br *bufio.Reader) ([]byte, bool, error) {
	var line []byte
	oversized := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !oversized {
			line = append(line, chunk...)
			if len(bytes.TrimRight(line, "\r\n")) > execMaxLineBytes {
				oversized, line = true, nil
			}
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) > 0 || oversized {
				return line, oversized, nil
			}
			return nil, false, io.EOF
		default:
			return line, oversized, err
		}
	}
}

func (p *execParser) warn(line int, code, message string) {
	p.warnings = append(p.warnings, ParseWarning{Line: line, Code: code, Message: message})
}

func (p *execParser) currentTurn() *execTurn {
	return p.turns[len(p.turns)-1]
}

// addEvent records a turn-level event in the current turn.
func (p *execParser) addEvent(event string, line []byte, lineNum int) {
	turn := p.currentTurn()
	turn.records = append(turn.records, &execRecord{event: event, fields: compactJSON(line), line: lineNum})
}

func (p *execParser) parseLine(line []byte, lineNum int) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		// Q45: fixed text and the syntax error's offset; never the line itself.
		message := "malformed JSON event"
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			message += fmt.Sprintf(" (offset %d)", syntaxErr.Offset)
		}
		p.warn(lineNum, WarningMalformedLine, message)
		return
	}
	event, ok := jsonString(fields["type"])
	if !ok || event == "" {
		p.warn(lineNum, WarningInvalidRecord, "event has no type")
		return
	}

	switch event {
	case "thread.started":
		threadID, ok := jsonString(fields["thread_id"])
		switch {
		case !ok || threadID == "":
			p.addEvent(event, line, lineNum)
			p.warn(lineNum, WarningInvalidRecord, "thread.started has no thread ID")
		case p.threadID == "":
			p.threadID = threadID
		case threadID != p.threadID:
			// Q44: keep the event without changing the stream's identity.
			p.addEvent(event, line, lineNum)
			p.warn(lineNum, WarningThreadMismatch, "thread.started records a different thread ID")
		}
	case "turn.started":
		p.turns = append(p.turns, newExecTurn(p.currentTurn().number+1))
	case "item.started", "item.updated", "item.completed":
		p.addItem(event, fields["item"], line, lineNum)
	default:
		p.addEvent(event, line, lineNum)
	}
}

// addItem applies one lifecycle event: the first sighting of an item appends
// a record, later events replace its state, and item.completed marks it done.
func (p *execParser) addItem(event string, raw json.RawMessage, line []byte, lineNum int) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil || item == nil {
		p.addEvent(event, line, lineNum)
		p.warn(lineNum, WarningItemDecodeFailed, event+" has no item object")
		return
	}
	turn := p.currentTurn()
	id, ok := jsonString(item["id"])
	if !ok {
		id = ""
	}
	key := execItemKey{id: id}
	if id == "" {
		key.line = lineNum
	}
	rec, seen := turn.items[key]
	if !seen {
		rec = &execRecord{isItem: true, itemID: id}
		turn.items[key] = rec
		turn.records = append(turn.records, rec)
	}
	rec.event = event
	rec.fields = compactJSON(raw)
	rec.itemType, _ = jsonString(item["type"])
	rec.line = lineNum
	if event == "item.completed" {
		rec.completed = true
	}
}

// translate turns a record into content blocks for the given turn.
func (p *execParser) translate(rec *execRecord, turn int) []ContentItem {
	if !rec.isItem {
		return []ContentItem{p.translateEvent(rec)}
	}
	items, ok := translateItem(rec, turn)
	if !ok {
		if rec.itemType != "" && rec.itemID != "" && !execKnownItem(rec.itemType) {
			// No specialised translation: a generic event without a warning (2.4).
			items = []ContentItem{{Type: "event", ItemID: rec.itemID, EventType: rec.itemType, Payload: rec.fields}}
		} else {
			p.warn(rec.line, WarningItemDecodeFailed, "item has no valid ID or fields for its type")
			items = []ContentItem{{Type: "event", ItemID: rec.itemID, EventType: cmp.Or(rec.itemType, rec.event), Payload: rec.fields}}
		}
	}
	if !rec.completed {
		for i := range items {
			items[i].Incomplete = true
		}
	}
	return items
}

func (p *execParser) translateEvent(rec *execRecord) ContentItem {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(rec.fields, &fields)
	switch rec.event {
	case "turn.completed":
		if usage, ok := orderedCounters(fields["usage"]); ok {
			return ContentItem{Type: "usage", Usage: &Usage{Basis: UsageBasisThreadTotal, Counters: usage}}
		}
		p.warn(rec.line, WarningInvalidRecord, "turn.completed has no usage object")
	case "error":
		if message, ok := jsonString(fields["message"]); ok {
			return ContentItem{Type: "error", Text: message, Payload: rec.fields}
		}
		p.warn(rec.line, WarningInvalidRecord, "error event has no message")
	case "turn.failed":
		var failed struct {
			Error *execErrorItem `json:"error"`
		}
		if json.Unmarshal(rec.fields, &failed) == nil && failed.Error != nil && failed.Error.Message != nil {
			return ContentItem{Type: "error", Text: *failed.Error.Message, Payload: rec.fields}
		}
		p.warn(rec.line, WarningInvalidRecord, "turn.failed has no error message")
	}
	return ContentItem{Type: "event", EventType: rec.event, Payload: rec.fields}
}

// execKnownItem reports whether an item type has a specialised translation.
func execKnownItem(itemType string) bool {
	switch itemType {
	case "agent_message", "reasoning", "command_execution", "file_change",
		"mcp_tool_call", "web_search", "todo_list", "error":
		return true
	}
	return false
}

// translateItem gives an item's specialised content. It reports false for
// unknown types and for known types without a valid ID or typed fields.
func translateItem(rec *execRecord, turn int) ([]ContentItem, bool) {
	if rec.itemID == "" || !execKnownItem(rec.itemType) {
		return nil, false
	}
	id := rec.itemID
	toolID := fmt.Sprintf("t%d:%s", turn, id)
	toolUse := func(input map[string]any) ContentItem {
		return ContentItem{Type: "tool_use", ID: toolID, ItemID: id, Name: rec.itemType, Input: input}
	}
	toolResult := func(content, status string, isError bool) ContentItem {
		return ContentItem{Type: "tool_result", ToolUseID: toolID, ItemID: id, Content: content, Status: status, IsError: isError}
	}

	switch rec.itemType {
	case "agent_message", "reasoning":
		var item execTextItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Text == nil {
			return nil, false
		}
		if rec.itemType == "reasoning" {
			return []ContentItem{{Type: "thinking", ItemID: id, Thinking: *item.Text}}, true
		}
		return []ContentItem{{Type: "text", ItemID: id, Text: *item.Text}}, true

	case "command_execution":
		var item execCommandItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Command == nil {
			return nil, false
		}
		failed := item.Status == "failed" || item.Status == "declined" || (item.ExitCode != nil && *item.ExitCode != 0)
		result := toolResult(item.AggregatedOutput, item.Status, failed)
		result.ExitCode = item.ExitCode
		return []ContentItem{toolUse(map[string]any{"command": *item.Command}), result}, true

	case "file_change":
		var item execFileChangeItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Changes == nil {
			return nil, false
		}
		lines := make([]string, 0, len(item.Changes))
		for _, change := range item.Changes {
			if change.Path == nil || change.Kind == nil {
				return nil, false
			}
			lines = append(lines, *change.Kind+" "+*change.Path)
		}
		var raw struct {
			Changes json.RawMessage `json:"changes"`
		}
		_ = json.Unmarshal(rec.fields, &raw)
		return []ContentItem{
			toolUse(map[string]any{"changes": decodeJSONValue(raw.Changes)}),
			toolResult(strings.Join(lines, "\n"), item.Status, item.Status == "failed"),
		}, true

	case "mcp_tool_call":
		var item execMCPToolCallItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Server == nil || item.Tool == nil {
			return nil, false
		}
		content := ""
		switch {
		case item.Error != nil:
			content = item.Error.Message
		case len(item.Result) > 0 && string(item.Result) != "null":
			content = string(item.Result)
		}
		return []ContentItem{
			toolUse(map[string]any{"server": *item.Server, "tool": *item.Tool, "arguments": decodeJSONValue(item.Arguments)}),
			toolResult(content, item.Status, item.Status == "failed" || item.Error != nil),
		}, true

	case "web_search":
		var item execWebSearchItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Query == nil {
			return nil, false
		}
		content := ""
		if len(item.Results) > 0 && string(item.Results) != "null" {
			content = string(item.Results)
		}
		return []ContentItem{
			toolUse(map[string]any{"query": *item.Query, "action": decodeJSONValue(item.Action)}),
			toolResult(content, "", false),
		}, true

	case "todo_list":
		var item execTodoListItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Items == nil {
			return nil, false
		}
		plan := make([]PlanItem, 0, len(item.Items))
		for _, todo := range item.Items {
			if todo.Text == nil || todo.Completed == nil {
				return nil, false
			}
			plan = append(plan, PlanItem{Text: *todo.Text, Completed: *todo.Completed})
		}
		return []ContentItem{{Type: "plan", ItemID: id, Plan: plan}}, true

	case "error":
		var item execErrorItem
		if json.Unmarshal(rec.fields, &item) != nil || item.Message == nil {
			return nil, false
		}
		return []ContentItem{{Type: "error", ItemID: id, Text: *item.Message}}, true
	}
	return nil, false
}

// orderedCounters reads a usage object's counters in source key order, keeping
// each value as raw JSON. A repeated key keeps its first position and last value.
func orderedCounters(raw json.RawMessage) ([]UsageCounter, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	counters := []UsageCounter{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		name, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		value = compactJSON(value)
		if i := slices.IndexFunc(counters, func(c UsageCounter) bool { return c.Name == name }); i >= 0 {
			counters[i].Value = value
			continue
		}
		counters = append(counters, UsageCounter{Name: name, Value: value})
	}
	return counters, true
}

// jsonString decodes raw as a JSON string.
func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// decodeJSONValue decodes raw keeping numbers as json.Number, so values keep
// their precision when they are written again. Absent values decode to nil.
func decodeJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	return v
}

// compactJSON returns raw without insignificant whitespace.
func compactJSON(raw []byte) json.RawMessage {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return json.RawMessage(slices.Clone(raw))
	}
	return buf.Bytes()
}
