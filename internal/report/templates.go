package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"strings"
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
	var err error
	templates = template.New("").Funcs(templateFuncs)

	// Parse index template
	templates, err = templates.New("index.html").Parse(indexTemplateContent)
	if err != nil {
		panic("failed to parse index template: " + err.Error())
	}

	// Parse diff template
	templates, err = templates.New("diff.html").Parse(diffTemplateContent)
	if err != nil {
		panic("failed to parse diff template: " + err.Error())
	}

	// Define CSS as a template
	templates, err = templates.New("style.css").Parse(styleCSS)
	if err != nil {
		panic("failed to parse style template: " + err.Error())
	}
}

// templateFuncs provides helper functions for templates.
var templateFuncs = template.FuncMap{
	"formatCost": formatCost,
	"add":        add,
	"sub":        sub,
}

// formatCost formats a cost value as a currency string.
func formatCost(cost float64) string {
	if cost == 0 {
		return "-"
	}
	return "$" + trimTrailingZeros(fmt.Sprintf("%.4f", cost))
}

// trimTrailingZeros removes unnecessary trailing zeros after decimal point.
func trimTrailingZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// add returns a + b.
func add(a, b int) int {
	return a + b
}

// sub returns a - b.
func sub(a, b int) int {
	return a - b
}
