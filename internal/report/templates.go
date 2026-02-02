package report

import (
	_ "embed"
	"html/template"

	"github.com/arjenschwarz/orbit/internal/cost"
)

//go:embed templates/index.html
var indexTemplateContent string

//go:embed templates/diff.html
var diffTemplateContent string

//go:embed templates/style.css
var styleCSS string

// templates holds the parsed HTML templates.
var templates *template.Template

func init() {
	// Parse embedded templates. Panics are acceptable here because:
	// 1. Templates are embedded at compile time via //go:embed
	// 2. Parse errors indicate programming bugs, not runtime issues
	// 3. These would be caught during development/testing, not production
	var err error
	templates = template.New("").Funcs(templateFuncs)

	templates, err = templates.New("index.html").Parse(indexTemplateContent)
	if err != nil {
		panic("failed to parse index template: " + err.Error())
	}

	templates, err = templates.New("diff.html").Parse(diffTemplateContent)
	if err != nil {
		panic("failed to parse diff template: " + err.Error())
	}

	templates, err = templates.New("style.css").Parse(styleCSS)
	if err != nil {
		panic("failed to parse style template: " + err.Error())
	}
}

// templateFuncs provides helper functions for templates.
var templateFuncs = template.FuncMap{
	"formatCost":       formatCost,
	"formatCostTotals": formatCostTotals,
	"add":              add,
	"sub":              sub,
}

// formatCost formats a cost value according to its unit type.
// Uses the centralized cost.Format function for consistent formatting.
func formatCost(value float64, unit string) string {
	if unit == "" {
		unit = cost.UnitUSD
	}
	return cost.Format(value, unit)
}

// formatCostTotals formats cost totals using the centralized cost.FormatTotals function.
// Returns "-" if totals is nil or empty.
func formatCostTotals(totals *cost.Totals) string {
	if totals == nil {
		return "-"
	}
	return cost.FormatTotals(*totals)
}

// add returns a + b.
func add(a, b int) int {
	return a + b
}

// sub returns a - b.
func sub(a, b int) int {
	return a - b
}
