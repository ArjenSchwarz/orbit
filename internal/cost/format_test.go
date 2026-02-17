package cost

import "testing"

func TestFormat(t *testing.T) {
	tests := map[string]struct {
		value    float64
		unit     string
		expected string
	}{
		"USD":              {1.23, UnitUSD, "$1.23"},
		"credits":          {0.45, UnitCredits, "0.45 credits"},
		"premium_requests": {0.33, UnitPremiumRequests, "0.33 premium requests"},
		"zero value":       {0, UnitUSD, "-"},
		"negative value":   {-1, UnitUSD, "-"},
		"unknown unit":     {1.5, "unknown", "1.50"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Format(tc.value, tc.unit)
			if got != tc.expected {
				t.Errorf("Format(%v, %q) = %q, want %q", tc.value, tc.unit, got, tc.expected)
			}
		})
	}
}

func TestFormatWithPrecision(t *testing.T) {
	tests := map[string]struct {
		value     float64
		unit      string
		precision int
		expected  string
	}{
		"USD 4 decimals":              {1.2345, UnitUSD, 4, "$1.2345"},
		"credits 3 decimals":          {0.456, UnitCredits, 3, "0.456 credits"},
		"premium_requests 1 decimal":  {0.3, UnitPremiumRequests, 1, "0.3 premium requests"},
		"zero value":                  {0, UnitUSD, 2, "-"},
		"negative value":              {-1, UnitCredits, 2, "-"},
		"unknown unit with precision": {1.5678, "unknown", 3, "1.568"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatWithPrecision(tc.value, tc.unit, tc.precision)
			if got != tc.expected {
				t.Errorf("FormatWithPrecision(%v, %q, %d) = %q, want %q",
					tc.value, tc.unit, tc.precision, got, tc.expected)
			}
		})
	}
}

func TestFormatCodeChanges(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	tests := map[string]struct {
		added    *int
		removed  *int
		expected string
	}{
		"both nil":            {nil, nil, "-"},
		"both zero":           {intPtr(0), intPtr(0), "+0/-0 lines"},
		"positive values":     {intPtr(10), intPtr(5), "+10/-5 lines"},
		"added only":          {intPtr(10), nil, "+10/-0 lines"},
		"removed only":        {nil, intPtr(5), "+0/-5 lines"},
		"large numbers":       {intPtr(1000), intPtr(500), "+1000/-500 lines"},
		"added nil removed 0": {nil, intPtr(0), "+0/-0 lines"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatCodeChanges(tc.added, tc.removed)
			if got != tc.expected {
				t.Errorf("FormatCodeChanges(%v, %v) = %q, want %q",
					tc.added, tc.removed, got, tc.expected)
			}
		})
	}
}

func TestFormatTotals(t *testing.T) {
	tests := map[string]struct {
		totals   Totals
		expected string
	}{
		"all units": {
			Totals{USD: 1.23, Credits: 0.45, PremiumRequests: 0.33},
			"$1.23, 0.45 credits, 0.33 premium requests",
		},
		"USD only": {
			Totals{USD: 1.23},
			"$1.23",
		},
		"credits only": {
			Totals{Credits: 0.45},
			"0.45 credits",
		},
		"premium_requests only": {
			Totals{PremiumRequests: 0.33},
			"0.33 premium requests",
		},
		"USD and credits": {
			Totals{USD: 1.23, Credits: 0.45},
			"$1.23, 0.45 credits",
		},
		"USD and premium_requests": {
			Totals{USD: 1.23, PremiumRequests: 0.33},
			"$1.23, 0.33 premium requests",
		},
		"credits and premium_requests": {
			Totals{Credits: 0.45, PremiumRequests: 0.33},
			"0.45 credits, 0.33 premium requests",
		},
		"all zero": {
			Totals{},
			"-",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatTotals(tc.totals)
			if got != tc.expected {
				t.Errorf("FormatTotals(%+v) = %q, want %q", tc.totals, got, tc.expected)
			}
		})
	}
}

func TestTotalsFromValue(t *testing.T) {
	tests := map[string]struct {
		existing Totals
		value    float64
		unit     string
		expected Totals
	}{
		"existing totals returned as-is": {
			existing: Totals{USD: 1.0, Credits: 2.0},
			value:    5.0,
			unit:     UnitUSD,
			expected: Totals{USD: 1.0, Credits: 2.0},
		},
		"existing with only credits": {
			existing: Totals{Credits: 3.0},
			value:    5.0,
			unit:     UnitUSD,
			expected: Totals{Credits: 3.0},
		},
		"empty existing USD": {
			existing: Totals{},
			value:    1.50,
			unit:     UnitUSD,
			expected: Totals{USD: 1.50},
		},
		"empty existing credits": {
			existing: Totals{},
			value:    2.50,
			unit:     UnitCredits,
			expected: Totals{Credits: 2.50},
		},
		"empty existing premium requests": {
			existing: Totals{},
			value:    3.0,
			unit:     UnitPremiumRequests,
			expected: Totals{PremiumRequests: 3.0},
		},
		"empty existing unknown unit defaults to USD": {
			existing: Totals{},
			value:    4.0,
			unit:     "unknown",
			expected: Totals{USD: 4.0},
		},
		"zero existing zero value": {
			existing: Totals{},
			value:    0,
			unit:     UnitUSD,
			expected: Totals{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := TotalsFromValue(tc.existing, tc.value, tc.unit)
			if got != tc.expected {
				t.Errorf("TotalsFromValue(%+v, %v, %q) = %+v, want %+v",
					tc.existing, tc.value, tc.unit, got, tc.expected)
			}
		})
	}
}

func TestInferUnitFromAgent(t *testing.T) {
	tests := map[string]struct {
		agentType string
		expected  string
	}{
		"kiro":        {"kiro", UnitCredits},
		"copilot":     {"copilot", UnitPremiumRequests},
		"claude-code": {"claude-code", UnitUSD},
		"codex":       {"codex", UnitUSD},
		"opencode":    {"opencode", UnitUSD},
		"unknown":     {"unknown", UnitUSD},
		"empty":       {"", UnitUSD},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := InferUnitFromAgent(tc.agentType)
			if got != tc.expected {
				t.Errorf("InferUnitFromAgent(%q) = %q, want %q",
					tc.agentType, got, tc.expected)
			}
		})
	}
}
