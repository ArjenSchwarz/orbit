package main

import (
	"testing"

	"github.com/arjenschwarz/orbit/internal/config"
)

func TestBuildAgentConfig(t *testing.T) {
	tests := []struct {
		name          string
		resolved      config.ResolvedAgent
		wantModel     string
		wantOptionsNil bool
	}{
		{
			name: "model in Options when set",
			resolved: config.ResolvedAgent{
				Alias: "claude-opus",
				Type:  "claude-code",
				Config: config.AgentAliasConfig{
					Type:        "claude-code",
					Model:       "claude-opus-4-20250514",
					AutoApprove: true,
				},
			},
			wantModel:      "claude-opus-4-20250514",
			wantOptionsNil: false,
		},
		{
			name: "Options nil when no model",
			resolved: config.ResolvedAgent{
				Alias: "claude-code",
				Type:  "claude-code",
				Config: config.AgentAliasConfig{
					Type:        "claude-code",
					AutoApprove: true,
				},
			},
			wantModel:      "",
			wantOptionsNil: true,
		},
		{
			name: "model with extra-args",
			resolved: config.ResolvedAgent{
				Alias: "codex-o3",
				Type:  "codex",
				Config: config.AgentAliasConfig{
					Type:      "codex",
					Model:     "o3",
					ExtraArgs: []string{"--verbose"},
				},
			},
			wantModel:      "o3",
			wantOptionsNil: false,
		},
		{
			name: "numeric model converted to string",
			resolved: config.ResolvedAgent{
				Alias: "test-agent",
				Type:  "codex",
				Config: config.AgentAliasConfig{
					Type:  "codex",
					Model: "4",
				},
			},
			wantModel:      "4",
			wantOptionsNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAgentConfig(tt.resolved)

			if tt.wantOptionsNil {
				if len(result.Options) > 0 {
					t.Errorf("expected Options to be nil or empty, got %v", result.Options)
				}
			} else {
				if result.Options == nil {
					t.Fatal("expected Options to be non-nil")
				}
				got := result.Options["model"]
				if got != tt.wantModel {
					t.Errorf("Options[model] = %q, want %q", got, tt.wantModel)
				}
			}

			// Verify other fields are passed through
			if result.AutoApprove != tt.resolved.Config.AutoApprove {
				t.Errorf("AutoApprove = %v, want %v", result.AutoApprove, tt.resolved.Config.AutoApprove)
			}
			if len(result.ExtraArgs) != len(tt.resolved.Config.ExtraArgs) {
				t.Errorf("ExtraArgs length = %d, want %d", len(result.ExtraArgs), len(tt.resolved.Config.ExtraArgs))
			}
		})
	}
}
