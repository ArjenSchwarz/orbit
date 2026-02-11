package copilot

import (
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UsageInfo contains parsed usage metrics from Copilot CLI output.
type UsageInfo struct {
	PremiumRequests float64
	APIDuration     *time.Duration
	SessionDuration *time.Duration
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	LinesAdded      *int
	LinesRemoved    *int
}

var (
	// Numeric pattern: matches valid floats like "0.33", "146.4", "1"
	// Uses \d+(?:\.\d+)? instead of [\d.]+ to reject malformed "1.2.3"
	numericPattern = `\d+(?:\.\d+)?`

	// Case-insensitive patterns with flexible whitespace
	premiumRequestsRe = regexp.MustCompile(`(?i)total\s+usage\s+est:\s+(` + numericPattern + `)\s+premium\s+requests?`)

	// Duration patterns: allow optional space after minutes (handles "1m 36s" and "1m36s")
	apiTimeRe     = regexp.MustCompile(`(?i)api\s+time\s+spent:\s+(?:(\d+)m\s*)?(` + numericPattern + `)s`)
	sessionTimeRe = regexp.MustCompile(`(?i)total\s+session\s+time:\s+(?:(\d+)m\s*)?(` + numericPattern + `)s`)

	codeChangesRe = regexp.MustCompile(`(?i)total\s+code\s+changes:\s+\+(\d+)\s+-(\d+)`)

	// Token breakdown regex: matches lines with model usage stats.
	// Anchored to start with whitespace + model name to avoid false positives.
	// Handles optional cached tokens (some models may not report caching).
	// Groups: 1=in_value, 2=in_suffix, 3=out_value, 4=out_suffix, 5=cached_value, 6=cached_suffix
	tokenBreakdownRe = regexp.MustCompile(`(?i)^\s+\S+\s+(` + numericPattern + `)([km])?\s+in,\s+(` + numericPattern + `)([km])?\s+out(?:,\s+(` + numericPattern + `)([km])?\s+cached)?`)
)

// ParseUsage extracts usage information from Copilot CLI output.
// It searches both stdout and stderr for the usage summary.
// Returns nil if no usage summary is found.
func ParseUsage(stdout, stderr string) *UsageInfo {
	// Combine and search both streams
	combined := stdout + "\n" + stderr

	info := &UsageInfo{}
	found := false

	// Extract premium requests (use last match if multiple)
	if matches := premiumRequestsRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		if val, err := strconv.ParseFloat(last[1], 64); err != nil {
			debugLog("Failed to parse premium requests value '%s': %v", last[1], err)
		} else if val < 0 {
			debugLog("Invalid negative premium requests value: %v", val)
		} else {
			info.PremiumRequests = val
			found = true
		}
	}

	// Extract API time
	if matches := apiTimeRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		dur, err := parseDuration(last[1], last[2])
		if err != nil {
			debugLog("Failed to parse API time '%s': %v", last[0], err)
		} else {
			info.APIDuration = &dur
			found = true
		}
	}

	// Extract session time
	if matches := sessionTimeRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		dur, err := parseDuration(last[1], last[2])
		if err != nil {
			debugLog("Failed to parse session time '%s': %v", last[0], err)
		} else {
			info.SessionDuration = &dur
			found = true
		}
	}

	// Extract code changes
	if matches := codeChangesRe.FindAllStringSubmatch(combined, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		added, err1 := strconv.Atoi(last[1])
		removed, err2 := strconv.Atoi(last[2])
		if err1 != nil || err2 != nil {
			debugLog("Failed to parse code changes '%s': added=%v, removed=%v", last[0], err1, err2)
		} else {
			info.LinesAdded = &added
			info.LinesRemoved = &removed
			found = true
		}
	}

	// Aggregate tokens from all model breakdown lines
	// Process line-by-line to use the anchored regex correctly
	for line := range strings.SplitSeq(combined, "\n") {
		if match := tokenBreakdownRe.FindStringSubmatch(line); match != nil {
			info.InputTokens += parseTokenValue(match[1], match[2])
			info.OutputTokens += parseTokenValue(match[3], match[4])
			// Cached tokens are optional (groups 5,6 may be empty)
			if match[5] != "" {
				info.CachedTokens += parseTokenValue(match[5], match[6])
			}
			found = true
		}
	}

	if !found {
		debugLog("No usage summary found in Copilot output")
		return nil
	}
	return info
}

// debugLog logs a message if ORBIT_DEBUG is enabled.
func debugLog(format string, args ...any) {
	if env := os.Getenv("ORBIT_DEBUG"); env == "true" || env == "1" {
		log.Printf("[copilot-usage] "+format, args...)
	}
}

// parseDuration parses minutes and seconds strings into a time.Duration.
func parseDuration(minutes, seconds string) (time.Duration, error) {
	var total float64
	if minutes != "" {
		m, err := strconv.Atoi(minutes)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes: %w", err)
		}
		total += float64(m) * 60
	}
	s, err := strconv.ParseFloat(seconds, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %w", err)
	}
	total += s
	return time.Duration(total * float64(time.Second)), nil
}

// parseTokenValue parses a token count with optional k/m suffix.
// Uses math.Round to avoid truncation issues (e.g., "1.999k" → 2000, not 1999).
// Returns 0 on parse error (tokens are non-critical, errors logged elsewhere).
func parseTokenValue(value, suffix string) int {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(suffix) {
	case "k":
		v *= 1000
	case "m":
		v *= 1000000
	}
	return int(math.Round(v))
}
