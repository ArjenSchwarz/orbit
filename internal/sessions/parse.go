package sessions

import (
	"github.com/arjenschwarz/orbit/internal/transcript"
)

// ParseResolved parses a resolved session for the CLI and the web. It copies
// the caller's options and overlays the resolved Kiro IDE cost path, so the
// Codex flags survive. Kiro IDE and codex-exec sessions use their own parser;
// other sources are detected from content. Callers that force a format (apsis
// -a) parse directly instead.
func ParseResolved(rs *ResolvedSession, opts ...transcript.ParseOptions) (*transcript.ParseResult, error) {
	var o transcript.ParseOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if rs.Metadata.CostPath != "" {
		o.KiroIDECostPath = rs.Metadata.CostPath
	}
	switch rs.Metadata.Source {
	case SourceKiroIDE:
		return transcript.ParseJSONLWithFormat(rs.Reader, transcript.FormatKiroIDE, o)
	case SourceCodexExec:
		return transcript.ParseJSONLWithFormat(rs.Reader, transcript.FormatCodexExec, o)
	}
	return transcript.Parse(rs.Reader, o)
}
