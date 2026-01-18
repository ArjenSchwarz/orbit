package variants

import (
	"testing"
)

func TestAssignVariantAgents(t *testing.T) {
	tests := []struct {
		name          string
		variants      []*Variant
		variantAgents []string
		defaultAgent  string
		expected      []string // Expected Agent values after assignment
	}{
		{
			name:          "no variant agents uses default",
			variants:      []*Variant{{ID: 1}, {ID: 2}, {ID: 3}},
			variantAgents: nil,
			defaultAgent:  "claude-code",
			expected:      []string{"claude-code", "claude-code", "claude-code"},
		},
		{
			name:          "empty variant agents uses default",
			variants:      []*Variant{{ID: 1}, {ID: 2}},
			variantAgents: []string{},
			defaultAgent:  "codex",
			expected:      []string{"codex", "codex"},
		},
		{
			name:          "exact match agent per variant",
			variants:      []*Variant{{ID: 1}, {ID: 2}, {ID: 3}},
			variantAgents: []string{"claude-code", "codex", "kiro"},
			defaultAgent:  "claude-code",
			expected:      []string{"claude-code", "codex", "kiro"},
		},
		{
			name:          "two agents for two variants",
			variants:      []*Variant{{ID: 1}, {ID: 2}},
			variantAgents: []string{"claude-code", "codex"},
			defaultAgent:  "claude-code",
			expected:      []string{"claude-code", "codex"},
		},
		{
			name:          "cycling with fewer agents than variants",
			variants:      []*Variant{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
			variantAgents: []string{"claude-code", "codex"},
			defaultAgent:  "kiro",
			expected:      []string{"claude-code", "codex", "claude-code", "codex"},
		},
		{
			name:          "cycling three agents for five variants",
			variants:      []*Variant{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
			variantAgents: []string{"a", "b", "c"},
			defaultAgent:  "default",
			expected:      []string{"a", "b", "c", "a", "b"},
		},
		{
			name:          "single agent for all variants",
			variants:      []*Variant{{ID: 1}, {ID: 2}, {ID: 3}},
			variantAgents: []string{"codex"},
			defaultAgent:  "claude-code",
			expected:      []string{"codex", "codex", "codex"},
		},
		{
			name:          "more agents than variants",
			variants:      []*Variant{{ID: 1}, {ID: 2}},
			variantAgents: []string{"a", "b", "c", "d"},
			defaultAgent:  "default",
			expected:      []string{"a", "b"},
		},
		{
			name:          "empty variants slice",
			variants:      []*Variant{},
			variantAgents: []string{"a", "b"},
			defaultAgent:  "default",
			expected:      []string{},
		},
		{
			name:          "preserves existing guidance",
			variants:      []*Variant{{ID: 1, Guidance: "hint1"}, {ID: 2, Guidance: "hint2"}},
			variantAgents: []string{"codex", "kiro"},
			defaultAgent:  "claude-code",
			expected:      []string{"codex", "kiro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data between runs
			variants := make([]*Variant, len(tt.variants))
			for i, v := range tt.variants {
				copy := *v
				variants[i] = &copy
			}

			AssignVariantAgents(variants, tt.variantAgents, tt.defaultAgent)

			// Verify agents were assigned correctly
			for i, v := range variants {
				if i >= len(tt.expected) {
					t.Errorf("variant %d: unexpected extra variant", v.ID)
					continue
				}
				if v.Agent != tt.expected[i] {
					t.Errorf("variant %d: Agent = %q, want %q", v.ID, v.Agent, tt.expected[i])
				}
			}

			// Verify guidance was preserved for the test that checks it
			if tt.name == "preserves existing guidance" {
				if variants[0].Guidance != "hint1" {
					t.Errorf("variant 1: Guidance = %q, want %q", variants[0].Guidance, "hint1")
				}
				if variants[1].Guidance != "hint2" {
					t.Errorf("variant 2: Guidance = %q, want %q", variants[1].Guidance, "hint2")
				}
			}
		})
	}
}

func TestVariant_AgentField(t *testing.T) {
	// Test that the Agent field is properly initialized and accessible
	v := Variant{
		ID:     1,
		Branch: "test-branch",
		Agent:  "codex",
	}

	if v.Agent != "codex" {
		t.Errorf("Variant.Agent = %q, want %q", v.Agent, "codex")
	}

	// Test JSON serialization includes agent
	v2 := Variant{
		ID:    2,
		Agent: "kiro",
	}
	if v2.Agent != "kiro" {
		t.Errorf("Variant.Agent = %q, want %q", v2.Agent, "kiro")
	}
}
