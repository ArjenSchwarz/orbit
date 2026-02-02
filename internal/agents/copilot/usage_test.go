package copilot

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"pgregory.net/rapid"
)

// Helper functions for creating pointers
func durPtr(d time.Duration) *time.Duration { return &d }
func intPtr(i int) *int                     { return &i }

func TestParseUsage(t *testing.T) {
	tests := map[string]struct {
		stdout   string
		stderr   string
		expected *UsageInfo
	}{
		"complete output": {
			stdout: `Total usage est:        0.33 Premium requests
API time spent:         28s
Total session time:     33s
Total code changes:     +10 -5
Breakdown by AI model:
 claude-haiku-4.5        146.4k in, 2.6k out, 88.2k cached`,
			expected: &UsageInfo{
				PremiumRequests: 0.33,
				APIDuration:     durPtr(28 * time.Second),
				SessionDuration: durPtr(33 * time.Second),
				InputTokens:     146400,
				OutputTokens:    2600,
				CachedTokens:    88200,
				LinesAdded:      intPtr(10),
				LinesRemoved:    intPtr(5),
			},
		},
		"minutes and seconds format": {
			stdout: `Total usage est:        1.5 Premium requests
API time spent:         1m 36.11s
Total session time:     1m 48.964s`,
			expected: &UsageInfo{
				PremiumRequests: 1.5,
				APIDuration:     durPtr(96110 * time.Millisecond),
				SessionDuration: durPtr(108964 * time.Millisecond),
			},
		},
		"million token suffix": {
			stdout: `Breakdown by AI model:
 claude-haiku-4.5        1.3m in, 8.7k out, 1.3m cached`,
			expected: &UsageInfo{
				InputTokens:  1300000,
				OutputTokens: 8700,
				CachedTokens: 1300000,
			},
		},
		"multiple model lines aggregated": {
			stdout: `Breakdown by AI model:
 claude-haiku-4.5        100k in, 10k out, 50k cached
 gpt-4                   200k in, 20k out, 100k cached`,
			expected: &UsageInfo{
				InputTokens:  300000,
				OutputTokens: 30000,
				CachedTokens: 150000,
			},
		},
		"case insensitive": {
			stdout: `TOTAL USAGE EST:        0.5 PREMIUM REQUESTS
API TIME SPENT:         10s`,
			expected: &UsageInfo{
				PremiumRequests: 0.5,
				APIDuration:     durPtr(10 * time.Second),
			},
		},
		"no usage summary": {
			stdout:   "Some other output",
			expected: nil,
		},
		"partial output": {
			stdout: `Total usage est:        0.33 Premium requests`,
			expected: &UsageInfo{
				PremiumRequests: 0.33,
			},
		},
		"in stderr": {
			stderr: `Total usage est:        0.33 Premium requests`,
			expected: &UsageInfo{
				PremiumRequests: 0.33,
			},
		},
		"last occurrence used": {
			stdout: `Total usage est:        0.10 Premium requests
Total usage est:        0.33 Premium requests`,
			expected: &UsageInfo{
				PremiumRequests: 0.33,
			},
		},
		"duration without space after minutes": {
			stdout: `API time spent:         1m36s
Total session time:     2m15.5s`,
			expected: &UsageInfo{
				APIDuration:     durPtr(96 * time.Second),
				SessionDuration: durPtr(135500 * time.Millisecond),
			},
		},
		"tokens without cached": {
			stdout: `Breakdown by AI model:
 gpt-4                   100k in, 10k out`,
			expected: &UsageInfo{
				InputTokens:  100000,
				OutputTokens: 10000,
				CachedTokens: 0,
			},
		},
		"malformed number rejected": {
			stdout:   `Total usage est:        1.2.3 Premium requests`,
			expected: nil, // Regex rejects "1.2.3" with strict pattern
		},
		"zero code changes": {
			stdout: `Total code changes:     +0 -0`,
			expected: &UsageInfo{
				LinesAdded:   intPtr(0),
				LinesRemoved: intPtr(0),
			},
		},
		"large code changes": {
			stdout: `Total code changes:     +1234 -5678`,
			expected: &UsageInfo{
				LinesAdded:   intPtr(1234),
				LinesRemoved: intPtr(5678),
			},
		},
		"seconds only durations": {
			stdout: `API time spent:         36.11s
Total session time:     48.964s`,
			expected: &UsageInfo{
				APIDuration:     durPtr(36110 * time.Millisecond),
				SessionDuration: durPtr(48964 * time.Millisecond),
			},
		},
		"mixed case token breakdown": {
			stdout: `Breakdown by AI model:
 claude-haiku-4.5        100K in, 10K out, 50K cached`,
			expected: &UsageInfo{
				InputTokens:  100000,
				OutputTokens: 10000,
				CachedTokens: 50000,
			},
		},
		"whitespace variations": {
			stdout: `Total  usage   est:    0.5   Premium    requests`,
			expected: &UsageInfo{
				PremiumRequests: 0.5,
			},
		},
		"combined stdout and stderr": {
			stdout: `Total usage est:        0.5 Premium requests`,
			stderr: `API time spent:         10s`,
			expected: &UsageInfo{
				PremiumRequests: 0.5,
				APIDuration:     durPtr(10 * time.Second),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseUsage(tc.stdout, tc.stderr)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("ParseUsage() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := map[string]struct {
		minutes  string
		seconds  string
		expected time.Duration
		wantErr  bool
	}{
		"seconds only":              {"", "28", 28 * time.Second, false},
		"seconds with decimal":      {"", "36.11", 36110 * time.Millisecond, false},
		"minutes and seconds":       {"1", "36", 96 * time.Second, false},
		"minutes and decimal secs":  {"1", "36.11", 96110 * time.Millisecond, false},
		"large minutes":             {"10", "30", 630 * time.Second, false},
		"zero seconds":              {"", "0", 0, false},
		"zero minutes and seconds":  {"0", "0", 0, false},
		"invalid seconds":           {"", "abc", 0, true},
		"invalid minutes":           {"abc", "10", 0, true},
		"high precision seconds":    {"", "48.964", 48964 * time.Millisecond, false},
		"very high precision":       {"", "1.123456", 1123456 * time.Microsecond, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseDuration(tc.minutes, tc.seconds)
			if tc.wantErr {
				if err == nil {
					t.Error("parseDuration() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration() unexpected error: %v", err)
				return
			}
			if got != tc.expected {
				t.Errorf("parseDuration() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestParseTokenValue(t *testing.T) {
	tests := map[string]struct {
		value    string
		suffix   string
		expected int
	}{
		"plain number":           {"146", "", 146},
		"decimal number":         {"146.4", "", 146},
		"k suffix":               {"146.4", "k", 146400},
		"K suffix uppercase":     {"146.4", "K", 146400},
		"m suffix":               {"1.3", "m", 1300000},
		"M suffix uppercase":     {"1.3", "M", 1300000},
		"whole number with k":    {"100", "k", 100000},
		"whole number with m":    {"2", "m", 2000000},
		"rounding up":            {"1.999", "k", 1999},
		"rounding with decimals": {"146.4567", "k", 146457},
		"invalid value":          {"abc", "k", 0},
		"empty value":            {"", "", 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseTokenValue(tc.value, tc.suffix)
			if got != tc.expected {
				t.Errorf("parseTokenValue(%q, %q) = %d, want %d", tc.value, tc.suffix, got, tc.expected)
			}
		})
	}
}

// Property-based tests using rapid

// TestPropertyParseUsage_PremiumRequests verifies that valid premium request values parse correctly.
func TestPropertyParseUsage_PremiumRequests(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate valid premium request values (0.0 to 1000.0)
		premiumReqs := rapid.Float64Range(0.01, 1000).Draw(rt, "premiumReqs")

		// Generate output with standard format
		output := fmt.Sprintf("Total usage est:        %.2f Premium requests", premiumReqs)

		result := ParseUsage(output, "")
		if result == nil {
			rt.Fatal("expected result, got nil")
			return // unreachable but satisfies staticcheck
		}

		// Property: parsed value should match generated value (within floating point tolerance)
		if math.Abs(result.PremiumRequests-premiumReqs) > 0.01 {
			rt.Fatalf("PremiumRequests = %v, want %v", result.PremiumRequests, premiumReqs)
		}
	})
}

// TestPropertyParseUsage_WhitespaceTolerance verifies parsing tolerates varying whitespace.
func TestPropertyParseUsage_WhitespaceTolerance(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate varying amounts of whitespace
		spaces1 := rapid.IntRange(1, 10).Draw(rt, "spaces1")
		spaces2 := rapid.IntRange(1, 10).Draw(rt, "spaces2")

		// Build whitespace strings
		ws1 := ""
		for range spaces1 {
			ws1 += " "
		}
		ws2 := ""
		for range spaces2 {
			ws2 += " "
		}

		// Generate output with varying whitespace
		output := fmt.Sprintf("Total%susage%sest:  0.5 Premium requests", ws1, ws2)

		result := ParseUsage(output, "")
		if result == nil {
			rt.Fatal("expected result, got nil - whitespace tolerance failed")
			return // unreachable but satisfies staticcheck
		}

		// Property: parsed value should be correct regardless of whitespace
		if result.PremiumRequests != 0.5 {
			rt.Fatalf("PremiumRequests = %v, want 0.5", result.PremiumRequests)
		}
	})
}

// TestPropertyParseDuration_RoundTrip verifies duration parsing produces valid durations.
func TestPropertyParseDuration_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate valid duration components
		minutes := rapid.IntRange(0, 120).Draw(rt, "minutes")
		seconds := rapid.Float64Range(0, 59.999).Draw(rt, "seconds")

		minStr := ""
		if minutes > 0 {
			minStr = fmt.Sprintf("%d", minutes)
		}
		secStr := fmt.Sprintf("%.3f", seconds)

		dur, err := parseDuration(minStr, secStr)
		if err != nil {
			rt.Fatalf("parseDuration(%q, %q) failed: %v", minStr, secStr, err)
		}

		// Property: duration should be non-negative
		if dur < 0 {
			rt.Fatalf("parsed duration is negative: %v", dur)
		}

		// Property: duration should approximate the input values
		expectedSeconds := float64(minutes)*60 + seconds
		actualSeconds := dur.Seconds()

		if math.Abs(actualSeconds-expectedSeconds) > 0.001 {
			rt.Fatalf("duration mismatch: got %.3f seconds, want %.3f seconds", actualSeconds, expectedSeconds)
		}
	})
}

// TestPropertyParseTokenValue_MultiplierCorrectness verifies k/m suffixes multiply correctly.
func TestPropertyParseTokenValue_MultiplierCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate base values that produce reasonable token counts
		// Use integer tenths to avoid floating point precision issues
		intValue := rapid.IntRange(1, 9999).Draw(rt, "intValue")
		baseValue := float64(intValue) / 10.0

		// Format to 1 decimal place (what we're parsing)
		valueStr := fmt.Sprintf("%.1f", baseValue)

		// Parse the formatted string to get the actual value being parsed
		// This avoids floating point representation issues
		var parsedValue float64
		_, _ = fmt.Sscanf(valueStr, "%f", &parsedValue)

		// Test no suffix
		noSuffix := parseTokenValue(valueStr, "")
		expectedNoSuffix := int(math.Round(parsedValue))
		if noSuffix != expectedNoSuffix {
			rt.Fatalf("no suffix: parseTokenValue(%s, \"\") = %d, want %d", valueStr, noSuffix, expectedNoSuffix)
		}

		// Test k suffix
		withK := parseTokenValue(valueStr, "k")
		expectedK := int(math.Round(parsedValue * 1000))
		if withK != expectedK {
			rt.Fatalf("k suffix: parseTokenValue(%s, \"k\") = %d, want %d", valueStr, withK, expectedK)
		}

		// Test m suffix
		withM := parseTokenValue(valueStr, "m")
		expectedM := int(math.Round(parsedValue * 1000000))
		if withM != expectedM {
			rt.Fatalf("m suffix: parseTokenValue(%s, \"m\") = %d, want %d", valueStr, withM, expectedM)
		}
	})
}

// TestPropertyParseUsage_TokenAggregation verifies that tokens from multiple models are summed.
func TestPropertyParseUsage_TokenAggregation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate token counts for two models
		in1 := rapid.IntRange(1, 500).Draw(rt, "in1")
		out1 := rapid.IntRange(1, 100).Draw(rt, "out1")
		cached1 := rapid.IntRange(1, 200).Draw(rt, "cached1")
		in2 := rapid.IntRange(1, 500).Draw(rt, "in2")
		out2 := rapid.IntRange(1, 100).Draw(rt, "out2")
		cached2 := rapid.IntRange(1, 200).Draw(rt, "cached2")

		// Build multi-model output
		output := fmt.Sprintf(`Breakdown by AI model:
 model-a        %dk in, %dk out, %dk cached
 model-b        %dk in, %dk out, %dk cached`, in1, out1, cached1, in2, out2, cached2)

		result := ParseUsage(output, "")
		if result == nil {
			rt.Fatal("expected result, got nil")
			return // unreachable but satisfies staticcheck
		}

		// Property: tokens should be summed correctly
		expectedIn := (in1 + in2) * 1000
		expectedOut := (out1 + out2) * 1000
		expectedCached := (cached1 + cached2) * 1000

		if result.InputTokens != expectedIn {
			rt.Fatalf("InputTokens = %d, want %d", result.InputTokens, expectedIn)
		}
		if result.OutputTokens != expectedOut {
			rt.Fatalf("OutputTokens = %d, want %d", result.OutputTokens, expectedOut)
		}
		if result.CachedTokens != expectedCached {
			rt.Fatalf("CachedTokens = %d, want %d", result.CachedTokens, expectedCached)
		}
	})
}
