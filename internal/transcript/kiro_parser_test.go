package transcript

import (
	"strings"
	"testing"
)

func TestParseKiroUsageInfo(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    float64
		wantErr bool
	}{
		"valid session with credits": {
			input: `{
				"conversation_id": "test-123",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-1",
					"requests": [],
					"usage_info": [
						{"unit": "credit", "unit_plural": "credits", "value": 0.05},
						{"unit": "credit", "unit_plural": "credits", "value": 0.04}
					]
				}
			}`,
			want:    0.09,
			wantErr: false,
		},
		"session without user_turn_metadata": {
			input: `{
				"conversation_id": "test-456",
				"history": []
			}`,
			want:    0,
			wantErr: false,
		},
		"session with empty usage_info": {
			input: `{
				"conversation_id": "test-789",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-2",
					"requests": [],
					"usage_info": []
				}
			}`,
			want:    0,
			wantErr: false,
		},
		"session with mixed unit types": {
			input: `{
				"conversation_id": "test-mixed",
				"history": [],
				"user_turn_metadata": {
					"continuation_id": "cont-3",
					"requests": [],
					"usage_info": [
						{"unit": "credit", "unit_plural": "credits", "value": 0.10},
						{"unit": "token", "unit_plural": "tokens", "value": 1000},
						{"unit": "credit", "unit_plural": "credits", "value": 0.05}
					]
				}
			}`,
			want:    0.15,
			wantErr: false,
		},
		"invalid JSON": {
			input:   `{invalid json`,
			want:    0,
			wantErr: true,
		},
		"empty input": {
			input:   ``,
			want:    0,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseKiroUsageInfo(strings.NewReader(tc.input))

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Use tolerance for float comparison
			if got < tc.want-0.001 || got > tc.want+0.001 {
				t.Errorf("got %.4f, want %.4f", got, tc.want)
			}
		})
	}
}

func TestExtractKiroCredits(t *testing.T) {
	tests := map[string]struct {
		session *KiroSession
		want    float64
	}{
		"nil metadata": {
			session: &KiroSession{
				ConversationID:   "test-1",
				UserTurnMetadata: nil,
			},
			want: 0,
		},
		"empty usage info": {
			session: &KiroSession{
				ConversationID: "test-2",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{},
				},
			},
			want: 0,
		},
		"single credit entry": {
			session: &KiroSession{
				ConversationID: "test-3",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "credit", UnitPlural: "credits", Value: 0.123},
					},
				},
			},
			want: 0.123,
		},
		"multiple credit entries": {
			session: &KiroSession{
				ConversationID: "test-4",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "credit", UnitPlural: "credits", Value: 0.05},
						{Unit: "credit", UnitPlural: "credits", Value: 0.03},
						{Unit: "credit", UnitPlural: "credits", Value: 0.02},
					},
				},
			},
			want: 0.10,
		},
		"ignores non-credit units": {
			session: &KiroSession{
				ConversationID: "test-5",
				UserTurnMetadata: &KiroUserTurnMetadata{
					UsageInfo: []KiroUsageInfo{
						{Unit: "token", UnitPlural: "tokens", Value: 5000},
						{Unit: "credit", UnitPlural: "credits", Value: 0.08},
						{Unit: "dollar", UnitPlural: "dollars", Value: 1.50},
					},
				},
			},
			want: 0.08,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := extractKiroCredits(tc.session)

			// Use tolerance for float comparison
			if got < tc.want-0.0001 || got > tc.want+0.0001 {
				t.Errorf("got %.4f, want %.4f", got, tc.want)
			}
		})
	}
}
