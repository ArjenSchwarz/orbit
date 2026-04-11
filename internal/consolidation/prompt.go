package consolidation

import (
	"fmt"
	"sort"
	"strings"
)

// PromptBuilder constructs the agent prompt for consolidation.
type PromptBuilder struct {
	specName      string
	variantID     int
	reportPath    string
	worktreePaths map[int]string
	customPrompt  string
}

// NewPromptBuilder creates a prompt builder with context.
func NewPromptBuilder(specName string, variantID int, reportPath string, worktreePaths map[int]string, customPrompt string) *PromptBuilder {
	return &PromptBuilder{
		specName:      specName,
		variantID:     variantID,
		reportPath:    reportPath,
		worktreePaths: worktreePaths,
		customPrompt:  customPrompt,
	}
}

// Build generates the consolidation prompt.
func (pb *PromptBuilder) Build() string {
	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "You are consolidating improvements into variant %d for the %q feature.\n\n", pb.variantID, pb.specName)

	// Context section
	sb.WriteString("## Context\n")
	fmt.Fprintf(&sb, "- Comparison report: %s\n", pb.reportPath)
	fmt.Fprintf(&sb, "- Chosen variant worktree: %s\n", pb.worktreePaths[pb.variantID])

	// List other variant worktrees
	otherWorktrees := pb.getOtherWorktrees()
	if len(otherWorktrees) > 0 {
		fmt.Fprintf(&sb, "- Other variant worktrees: %s\n", strings.Join(otherWorktrees, ", "))
	}
	sb.WriteString("\n")

	// Custom instructions section (if provided)
	if pb.customPrompt != "" {
		sb.WriteString("## Custom Instructions\n")
		sb.WriteString(pb.customPrompt)
		sb.WriteString("\n\n")
	}

	// Instructions section
	sb.WriteString(`## Instructions
1. Read the comparison report, focusing on the "Cross-Variant Improvements" section
2. For each improvement from non-chosen variants:
   - Examine the source variant's code to understand the implementation
   - Decide if it should be adopted based on feasibility, value, and any custom instructions above
   - If adopting: implement it in the chosen variant, adapting to fit existing patterns
3. Commit all changes as a single commit with EXACTLY this message format:
`)
	fmt.Fprintf(&sb, "   feat(consolidate): Apply improvements from variants X, Y to variant %d for %s\n", pb.variantID, pb.specName)
	sb.WriteString(`   (Replace X, Y with actual source variant numbers)
4. Output a report (see format below)

`)

	// Conflict Resolution Policy
	sb.WriteString(`## Conflict Resolution Policy
If an improvement conflicts with the chosen variant's implementation:
- Prioritize the chosen variant's existing patterns and architecture
- Skip the conflicting improvement rather than forcing it
- Document the conflict clearly in your report

`)

	// Scope Constraints
	sb.WriteString(`## Scope Constraints - DO NOT:
- Add new external dependencies
- Modify build configuration files (Makefile, go.mod, package.json, etc.)
- Make unrelated refactors or "improvements" not listed in the comparison report
- Change public APIs unless explicitly required by an improvement
- Modify files outside the chosen variant's worktree
- Modify binary files (images, compiled assets, etc.)

`)

	// Edge Case Handling
	sb.WriteString(`## Edge Case Handling:
- If a file was renamed/moved in the source variant, search for similar content in the chosen variant
- Before implementing, check if the improvement is already present (avoid duplicate changes)
- If a file path from the report doesn't exist, note in your report and skip that improvement

`)

	// Report Format
	sb.WriteString("## Report Format (output this after committing)\n")
	sb.WriteString("```markdown\n")
	sb.WriteString("## Consolidation Report\n\n")
	sb.WriteString("### Applied\n")
	sb.WriteString("| Source | Files Modified | Description |\n")
	sb.WriteString("|--------|----------------|-------------|\n")
	sb.WriteString("| V{n} | path/to/file.go | Brief description of what was changed |\n\n")
	sb.WriteString("### Skipped\n")
	sb.WriteString("| Source | Reason |\n")
	sb.WriteString("|--------|--------|\n")
	sb.WriteString("| V{n} | Why this improvement was not applied |\n\n")
	sb.WriteString("### Commit\n")
	sb.WriteString("{commit SHA}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// getOtherWorktrees returns formatted list of non-chosen variant worktrees.
func (pb *PromptBuilder) getOtherWorktrees() []string {
	var others []string
	// Sort variant IDs for deterministic output
	var ids []int
	for id := range pb.worktreePaths {
		if id != pb.variantID {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		others = append(others, fmt.Sprintf("V%d: %s", id, pb.worktreePaths[id]))
	}
	return others
}
