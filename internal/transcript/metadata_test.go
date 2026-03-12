package transcript

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMain sets a fixed timezone for deterministic timestamp formatting.
// No existing tests in this package depend on the default local timezone.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("TEST", 0)
	m.Run()
}

func TestFormatTimestampMarkdown(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"valid RFC3339 UTC": {
			input: "2026-03-12T03:32:05Z",
			want:  "2026-03-12T03:32:05Z",
		},
		"valid RFC3339 with offset converts to local": {
			input: "2026-03-12T14:32:05+11:00",
			want:  "2026-03-12T03:32:05Z",
		},
		"valid RFC3339Nano": {
			input: "2026-03-12T03:32:05.123456789Z",
			want:  "2026-03-12T03:32:05Z",
		},
		"empty string": {
			input: "",
			want:  "",
		},
		"invalid timestamp": {
			input: "not-a-date",
			want:  "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatTimestampMarkdown(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFormatTimestampHTML(t *testing.T) {
	tests := map[string]struct {
		input    string
		want     string
		wantEmpty bool
	}{
		"valid RFC3339 produces time element": {
			input: "2026-03-12T03:32:05Z",
			want:  `<time datetime="2026-03-12T03:32:05Z">2026-03-12T03:32:05Z</time>`,
		},
		"valid RFC3339 with offset": {
			input: "2026-03-12T14:32:05+11:00",
			want:  `<time datetime="2026-03-12T03:32:05Z">2026-03-12T03:32:05Z</time>`,
		},
		"empty string": {
			input:    "",
			wantEmpty: true,
		},
		"invalid timestamp": {
			input:    "not-a-date",
			wantEmpty: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatTimestampHTML(tc.input)
			if tc.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFormatMessageMetaMarkdown(t *testing.T) {
	tests := map[string]struct {
		timestamp string
		model     string
		want      string
	}{
		"both timestamp and model": {
			timestamp: "2026-03-12T03:32:05Z",
			model:     "claude-opus",
			want:      " · 2026-03-12T03:32:05Z · claude-opus",
		},
		"timestamp only": {
			timestamp: "2026-03-12T03:32:05Z",
			model:     "",
			want:      " · 2026-03-12T03:32:05Z",
		},
		"model only": {
			timestamp: "",
			model:     "claude-opus",
			want:      " · claude-opus",
		},
		"neither timestamp nor model": {
			timestamp: "",
			model:     "",
			want:      "",
		},
		"invalid timestamp with model": {
			timestamp: "bad-ts",
			model:     "gpt-4",
			want:      " · gpt-4",
		},
		"invalid timestamp without model": {
			timestamp: "bad-ts",
			model:     "",
			want:      "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatMessageMetaMarkdown(tc.timestamp, tc.model)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFormatMessageMetaHTML(t *testing.T) {
	tests := map[string]struct {
		timestamp string
		model     string
		want      string
		wantEmpty bool
	}{
		"both timestamp and model": {
			timestamp: "2026-03-12T03:32:05Z",
			model:     "claude-opus",
			want: `<span class="message-meta">` +
				`<time datetime="2026-03-12T03:32:05Z">2026-03-12T03:32:05Z</time>` +
				`<span class="meta-separator">·</span>` +
				`<span>claude-opus</span>` +
				`</span>`,
		},
		"timestamp only": {
			timestamp: "2026-03-12T03:32:05Z",
			model:     "",
			want: `<span class="message-meta">` +
				`<time datetime="2026-03-12T03:32:05Z">2026-03-12T03:32:05Z</time>` +
				`</span>`,
		},
		"model only": {
			timestamp: "",
			model:     "claude-opus",
			want: `<span class="message-meta">` +
				`<span>claude-opus</span>` +
				`</span>`,
		},
		"neither timestamp nor model": {
			timestamp: "",
			model:     "",
			wantEmpty: true,
		},
		"invalid timestamp with model": {
			timestamp: "bad-ts",
			model:     "gpt-4",
			want: `<span class="message-meta">` +
				`<span>gpt-4</span>` +
				`</span>`,
		},
		"invalid timestamp without model": {
			timestamp: "bad-ts",
			model:     "",
			wantEmpty: true,
		},
		"model with special characters is HTML-escaped": {
			timestamp: "",
			model:     "<script>alert('xss')</script>",
			want: `<span class="message-meta">` +
				`<span>&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</span>` +
				`</span>`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatMessageMetaHTML(tc.timestamp, tc.model)
			if tc.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
