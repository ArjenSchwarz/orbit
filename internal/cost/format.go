// Package cost provides cost formatting utilities for displaying costs in their native units.
package cost

import (
	"fmt"
	"strings"
)

// Unit constants for cost types.
const (
	UnitUSD             = "USD"
	UnitCredits         = "credits"
	UnitPremiumRequests = "premium_requests"
)

// Format formats a cost value according to its unit type.
// Returns "-" if value is zero or negative.
func Format(value float64, unit string) string {
	if value <= 0 {
		return "-"
	}

	switch unit {
	case UnitUSD:
		return fmt.Sprintf("$%.2f", value)
	case UnitCredits:
		return fmt.Sprintf("%.2f credits", value)
	case UnitPremiumRequests:
		return fmt.Sprintf("%.2f premium requests", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

// FormatWithPrecision formats with specified decimal places.
// Returns "-" if value is zero or negative.
func FormatWithPrecision(value float64, unit string, precision int) string {
	if value <= 0 {
		return "-"
	}

	format := fmt.Sprintf("%%.%df", precision)
	switch unit {
	case UnitUSD:
		return "$" + fmt.Sprintf(format, value)
	case UnitCredits:
		return fmt.Sprintf(format+" credits", value)
	case UnitPremiumRequests:
		return fmt.Sprintf(format+" premium requests", value)
	default:
		return fmt.Sprintf(format, value)
	}
}

// FormatCodeChanges formats lines added/removed as "+N/-M lines".
// Returns "-" if both values are nil (data unavailable).
// Returns "+0/-0 lines" if both are explicitly zero (no changes made).
func FormatCodeChanges(added, removed *int) string {
	if added == nil && removed == nil {
		return "-"
	}
	a, r := 0, 0
	if added != nil {
		a = *added
	}
	if removed != nil {
		r = *removed
	}
	return fmt.Sprintf("+%d/-%d lines", a, r)
}

// Totals represents aggregated costs by unit type.
type Totals struct {
	USD             float64 `json:"usd,omitempty"`
	Credits         float64 `json:"credits,omitempty"`
	PremiumRequests float64 `json:"premium_requests,omitempty"`
}

// FormatTotals formats aggregated cost totals.
// Displays in order: USD, credits, premium requests.
// Omits unit types with zero values.
func FormatTotals(totals Totals) string {
	var parts []string

	if totals.USD > 0 {
		parts = append(parts, Format(totals.USD, UnitUSD))
	}
	if totals.Credits > 0 {
		parts = append(parts, Format(totals.Credits, UnitCredits))
	}
	if totals.PremiumRequests > 0 {
		parts = append(parts, Format(totals.PremiumRequests, UnitPremiumRequests))
	}

	if len(parts) == 0 {
		return "-"
	}

	return strings.Join(parts, ", ")
}

// InferUnitFromAgent returns the cost unit for an agent type.
// Used for backward compatibility with legacy summary.json files.
func InferUnitFromAgent(agentType string) string {
	switch agentType {
	case "kiro":
		return UnitCredits
	case "copilot":
		return UnitPremiumRequests
	default:
		return UnitUSD
	}
}
