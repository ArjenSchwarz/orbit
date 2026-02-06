package transcript

import (
	"fmt"
	"io"
)

// ParseKiroIDE parses a Kiro IDE .chat JSON file and returns the result.
func ParseKiroIDE(r io.Reader) (*ParseResult, error) {
	return nil, fmt.Errorf("kiro IDE parser not yet implemented")
}

// ParseKiroIDEWithCostPath parses a .chat file and extracts cost from the given
// execution detail file path.
func ParseKiroIDEWithCostPath(r io.Reader, executionDetailPath string) (*ParseResult, error) {
	return nil, fmt.Errorf("kiro IDE parser not yet implemented")
}
