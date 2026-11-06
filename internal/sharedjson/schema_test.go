package sharedjson

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	schemaPath = "../../docs/schemas/apsis-shared-json.v1.schema.json"
	// initialSchemaPath is a byte-exact copy of the first published schema.
	// Never update it: output must stay valid for consumers holding that file (Decision 25).
	initialSchemaPath = "testdata/apsis-shared-json.v1.initial.schema.json"
)

// closedDefinitions are the apsis-owned structures the closed test copy
// closes, as JSON pointer segments. unevaluatedProperties, unlike
// additionalProperties, sees properties declared through $ref, so composed
// definitions close correctly. Tool input, event payloads, usage counters and
// extension blocks stay open.
var closedDefinitions = [][]string{
	{"$defs", "sessionDocument"},
	{"$defs", "sessionDocument", "properties", "session"},
	{"$defs", "listingDocument"},
	{"$defs", "listingRow"},
	{"$defs", "cost"},
	{"$defs", "scope"},
	{"$defs", "entry"},
	{"$defs", "textBlock"},
	{"$defs", "thinkingBlock"},
	{"$defs", "toolUseBlock"},
	{"$defs", "toolResultBlock"},
	{"$defs", "planBlock"},
	{"$defs", "planItem"},
	{"$defs", "errorBlock"},
	{"$defs", "eventBlock"},
	{"$defs", "usageBlock"},
	{"$defs", "otherBlock"},
	{"$defs", "warning"},
}

type testSchemas struct {
	published, initial, closed *jsonschema.Schema
}

func (s testSchemas) all() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{"published": s.published, "initial": s.initial, "closed": s.closed}
}

func loadSchemas(t *testing.T) testSchemas {
	t.Helper()
	published := readJSON(t, schemaPath)
	closed := readJSON(t, schemaPath)
	for _, pointer := range closedDefinitions {
		node := closed
		for _, segment := range pointer {
			next, ok := node.(map[string]any)[segment]
			require.True(t, ok, "closed definition %v not found", pointer)
			node = next
		}
		node.(map[string]any)["unevaluatedProperties"] = false
	}
	return testSchemas{
		published: compileSchema(t, schemaPath, published),
		initial:   compileSchema(t, initialSchemaPath, readJSON(t, initialSchemaPath)),
		closed:    compileSchema(t, "testdata/closed.schema.json", closed),
	}
}

// compileSchema compiles one schema document in its own compiler, so copies
// sharing an $id do not conflict. No loader is registered: nothing is read
// from disk or the network beyond the document itself.
func compileSchema(t *testing.T, location string, doc any) *jsonschema.Schema {
	t.Helper()
	abs, err := filepath.Abs(location)
	require.NoError(t, err)
	c := jsonschema.NewCompiler()
	c.UseLoader(jsonschema.SchemeURLLoader{})
	c.AssertFormat()
	require.NoError(t, c.AddResource(abs, doc))
	schema, err := c.Compile(abs)
	require.NoError(t, err)
	return schema
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	return doc
}

func decodeJSON(t *testing.T, data string) any {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(data))
	require.NoError(t, err)
	return doc
}

// requireValid validates a document against the published, initial and closed schemas.
func requireValid(t *testing.T, schemas testSchemas, doc any) {
	t.Helper()
	for name, schema := range schemas.all() {
		assert.NoError(t, schema.Validate(doc), "%s schema", name)
	}
}

func TestSchema_HandwrittenDocumentsForLaterProducers(t *testing.T) {
	schemas := loadSchemas(t)
	paths, err := filepath.Glob(filepath.Join("testdata", "handwritten", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			requireValid(t, schemas, readJSON(t, path))
		})
	}
}

// validSession is a minimal valid session document that the cases below change.
const validSession = `{
  "schema_version": 1,
  "kind": "session",
  "session": {"source": "codex", "created_at": "2026-10-01T09:00:00Z"},
  "entries": [{"role": "assistant", "content": [
    {"type": "text", "text": "Hi"},
    {"type": "tool_result", "tool_use_id": "call-1", "content": "ok", "is_error": false}
  ]}],
  "warnings": [{"code": "malformed_line", "message": "bad line", "line": 2}]
}`

func changed(t *testing.T, change func(doc, session, entry, text, result, warning map[string]any)) any {
	t.Helper()
	doc := decodeJSON(t, validSession).(map[string]any)
	entry := doc["entries"].([]any)[0].(map[string]any)
	content := entry["content"].([]any)
	change(doc, doc["session"].(map[string]any), entry, content[0].(map[string]any), content[1].(map[string]any), doc["warnings"].([]any)[0].(map[string]any))
	return doc
}

func TestSchema_PublishedAcceptsAdditions(t *testing.T) {
	schemas := loadSchemas(t)
	large := json.Number("123456789012345678901234567890")
	for name, change := range map[string]func(doc, session, entry, text, result, warning map[string]any){
		"unknown root field":    func(doc, _, _, _, _, _ map[string]any) { doc["future"] = "value" },
		"unknown session field": func(_, session, _, _, _, _ map[string]any) { session["future"] = large },
		"unknown entry field":   func(_, _, entry, _, _, _ map[string]any) { entry["future"] = []any{} },
		"unknown block field":   func(_, _, _, text, _, _ map[string]any) { text["future"] = true },
		"unknown warning field": func(_, _, _, _, _, warning map[string]any) { warning["future"] = large },
		"large integer field":   func(doc, _, _, _, _, _ map[string]any) { doc["future_count"] = large },
		"unknown block type": func(_, _, entry, _, _, _ map[string]any) {
			entry["content"] = append(entry["content"].([]any), map[string]any{"type": "future_block", "text": json.Number("1")})
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := changed(t, change)
			assert.NoError(t, schemas.published.Validate(doc))
			assert.NoError(t, schemas.initial.Validate(doc))
		})
	}
}

func TestSchema_RejectsInvalidKnownFields(t *testing.T) {
	schemas := loadSchemas(t)
	for name, change := range map[string]func(doc, session, entry, text, result, warning map[string]any){
		"schema_version 2":          func(doc, _, _, _, _, _ map[string]any) { doc["schema_version"] = json.Number("2") },
		"unknown document kind":     func(doc, _, _, _, _, _ map[string]any) { doc["kind"] = "future" },
		"missing entries":           func(doc, _, _, _, _, _ map[string]any) { delete(doc, "entries") },
		"numeric created_at":        func(_, session, _, _, _, _ map[string]any) { session["created_at"] = json.Number("1") },
		"non-RFC3339 created_at":    func(_, session, _, _, _, _ map[string]any) { session["created_at"] = "yesterday" },
		"numeric origin":            func(_, session, _, _, _, _ map[string]any) { session["origin"] = json.Number("1") },
		"numeric entry id":          func(_, _, entry, _, _, _ map[string]any) { entry["id"] = json.Number("1") },
		"string turn":               func(_, _, entry, _, _, _ map[string]any) { entry["turn"] = "1" },
		"text block without text":   func(_, _, _, text, _, _ map[string]any) { delete(text, "text") },
		"numeric tool result":       func(_, _, _, _, result, _ map[string]any) { result["content"] = json.Number("1") },
		"tool result without error": func(_, _, _, _, result, _ map[string]any) { delete(result, "is_error") },
		"string is_error":           func(_, _, _, _, result, _ map[string]any) { result["is_error"] = "false" },
		"fractional exit_code":      func(_, _, _, _, result, _ map[string]any) { result["exit_code"] = json.Number("1.5") },
		"warning without code":      func(_, _, _, _, _, warning map[string]any) { delete(warning, "code") },
		"empty warning code":        func(_, _, _, _, _, warning map[string]any) { warning["code"] = "" },
		"string line":               func(_, _, _, _, _, warning map[string]any) { warning["line"] = "2" },
		"string chat_message":       func(_, _, _, _, _, warning map[string]any) { warning["chat_message"] = "2" },
		"zero execution_action":     func(_, _, _, _, _, warning map[string]any) { warning["execution_action"] = json.Number("0") },
		"fractional execution_action": func(_, _, _, _, _, warning map[string]any) {
			warning["execution_action"] = json.Number("1.5")
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := changed(t, change)
			assert.Error(t, schemas.published.Validate(doc))
			assert.Error(t, schemas.initial.Validate(doc))
		})
	}
}

// The extension branch must not accept a malformed block of a known type, so
// its exclusion list has to name every known discriminator.
func TestSchema_ExtensionBranchExcludesKnownTypes(t *testing.T) {
	schema := readJSON(t, schemaPath).(map[string]any)
	defs := schema["$defs"].(map[string]any)
	var known []string
	for _, branch := range defs["block"].(map[string]any)["oneOf"].([]any) {
		name := strings.TrimPrefix(branch.(map[string]any)["$ref"].(string), "#/$defs/")
		if name == "extensionBlock" {
			continue
		}
		typeSchema := defs[name].(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)
		known = append(known, typeSchema["const"].(string))
	}
	var excluded []string
	extensionType := defs["extensionBlock"].(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)
	for _, value := range extensionType["not"].(map[string]any)["enum"].([]any) {
		excluded = append(excluded, value.(string))
	}
	slices.Sort(known)
	slices.Sort(excluded)
	assert.Equal(t, known, excluded)

	schemas := loadSchemas(t)
	for _, blockType := range known {
		doc := changed(t, func(_, _, entry, _, _, _ map[string]any) {
			entry["content"] = []any{map[string]any{"type": blockType, "items": "wrong", "counters": "wrong", "message": json.Number("1"), "text": json.Number("1"), "event_type": json.Number("1"), "name": json.Number("1")}}
		})
		assert.Error(t, schemas.published.Validate(doc), "malformed %s block", blockType)
	}
}

func TestSchema_ClosedCopyRejectsUndeclaredStructuralFields(t *testing.T) {
	schemas := loadSchemas(t)
	for name, change := range map[string]func(doc, session, entry, text, result, warning map[string]any){
		"document": func(doc, _, _, _, _, _ map[string]any) { doc["undeclared"] = true },
		"session":  func(_, session, _, _, _, _ map[string]any) { session["undeclared"] = true },
		"entry":    func(_, _, entry, _, _, _ map[string]any) { entry["undeclared"] = true },
		"block":    func(_, _, _, _, result, _ map[string]any) { result["undeclared"] = true },
		"warning":  func(_, _, _, _, _, warning map[string]any) { warning["undeclared"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			doc := changed(t, change)
			assert.NoError(t, schemas.published.Validate(doc))
			assert.Error(t, schemas.closed.Validate(doc))
		})
	}

	listing := readJSON(t, filepath.Join("testdata", "handwritten", "listing-all-projects.json")).(map[string]any)
	listing["sessions"].([]any)[0].(map[string]any)["undeclared"] = true
	assert.NoError(t, schemas.published.Validate(listing))
	assert.Error(t, schemas.closed.Validate(listing), "listing rows close over the shared session metadata")

	open := changed(t, func(_, _, entry, _, _, _ map[string]any) {
		entry["content"] = []any{
			map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{"any": "key"}},
			map[string]any{"type": "event", "event_type": "future", "payload": map[string]any{"any": "key"}},
			map[string]any{"type": "usage", "basis": "example", "counters": map[string]any{"any_counter": json.Number("1")}},
			map[string]any{"type": "future_block", "any": "key"},
		}
	})
	assert.NoError(t, schemas.closed.Validate(open), "input, payload, counters and extension blocks stay open")
}

// publishedWarningCodes returns the warning codes the schema lists.
func publishedWarningCodes(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	var schema struct {
		Defs struct {
			Warning struct {
				Properties struct {
					Code struct {
						Examples []string `json:"examples"`
					} `json:"code"`
				} `json:"properties"`
			} `json:"warning"`
		} `json:"$defs"`
	}
	require.NoError(t, json.NewDecoder(bytes.NewReader(data)).Decode(&schema))
	require.NotEmpty(t, schema.Defs.Warning.Properties.Code.Examples)
	return schema.Defs.Warning.Properties.Code.Examples
}
