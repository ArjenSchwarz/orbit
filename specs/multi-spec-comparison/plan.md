# Multi-Spec Comparison Feature Plan

## Overview

Enable Orbit to run multiple implementations of the same spec in parallel (or sequentially), each in isolated git worktrees. After all implementations complete, Orbit compares the results and generates an HTML report with a recommendation on which implementation is best.

This feature leverages the non-deterministic nature of LLM outputs to explore multiple solution paths for the same specification, allowing users to select the best approach.

## Current State

- Orbit runs a single implementation per spec
- Uses the current working directory/branch directly
- No comparison or variant functionality exists
- `claude.Client` is the only agent implementation

---

## Architecture

### Worktree Structure

```
project/                          # Original repository
../orbit-impl-1-<spec-name>/      # Worktree 1 (sibling directory)
../orbit-impl-2-<spec-name>/      # Worktree 2 (sibling directory)
../orbit-impl-N-<spec-name>/      # Worktree N (sibling directory)
```

### Branch Naming

Branches follow the pattern `orbit-impl-N/<spec-name>` to satisfy tooling requirements (e.g., rune expects the spec name as the final path component):

- `orbit-impl-1/my-feature`
- `orbit-impl-2/my-feature`
- `orbit-impl-3/my-feature`

The prefix is configurable via `--branch-prefix` (default: `orbit-impl`).

### Report Output Structure

Reports are stored in the user-configured log directory (defaults to the same directory as `tasks.md`):

```
specs/<spec-name>/
  ├── tasks.md
  ├── comparison-report/
  │   ├── index.html           # Main report with recommendation
  │   ├── assets/
  │   │   └── styles.css       # Embedded or external styles
  │   ├── screenshots/         # UI captures (if available)
  │   └── diffs/
  │       ├── variant-1-2.html # Diff between variants 1 and 2
  │       └── ...
  └── ...
```

---

## Configuration

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--variants N` | Number of parallel implementations | 1 (disabled) |
| `--parallel` | Run variants in parallel vs sequential | false |
| `--branch-prefix PREFIX` | Branch prefix for worktrees | `orbit-impl` |
| `--guidance-file FILE` | YAML file with per-variant guidance | none |
| `--compare-command CMD` | Custom comparison command | built-in |
| `--screenshot-command CMD` | Command to capture screenshots | none |

### Config File (`.orbit.yaml`)

```yaml
variants:
  count: 3
  parallel: true
  branch_prefix: "orbit-impl"
  guidance:
    - "Focus on simplicity and maintainability"
    - "Optimize for performance"
    - "Use idiomatic patterns from the existing codebase"

comparison:
  command: ""  # Empty uses built-in Claude comparison
  screenshot_command: "npm run screenshots"

# Existing config...
command: "claude"
post_command: ""
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ORBIT_VARIANTS` | Number of variants |
| `ORBIT_PARALLEL` | Enable parallel execution |
| `ORBIT_BRANCH_PREFIX` | Branch prefix |
| `ORBIT_COMPARE_COMMAND` | Comparison command |
| `ORBIT_SCREENSHOT_COMMAND` | Screenshot command |

---

## Execution Flow

### Phase 1: Setup

1. Parse variant configuration (count, guidance, parallel mode)
2. For each variant N (1 to count):
   - Create branch `{prefix}-N/{spec-name}` from current HEAD
   - Create worktree at `../{prefix}-N-{spec-name}/`
3. Store worktree metadata for later cleanup

### Phase 2: Implementation

**Sequential Mode (default):**
```
for each variant:
    run all spec phases in worktree
    save logs to variant-specific subdirectory
```

**Parallel Mode:**
```
spawn goroutine for each variant:
    run all spec phases in worktree
    save logs to variant-specific subdirectory
wait for all variants to complete
```

**Rate Limit Handling in Parallel Mode:**
- Share rate limit state across variants
- When one variant hits a rate limit, pause all variants
- Resume all after the retry-after period

**Failure Handling:**
- If a variant fails, continue with remaining variants
- Comparison proceeds with successful variants only
- Report includes failure information for failed variants

### Phase 3: Testing (Optional)

If tests exist or are generated:
1. Run test suite in each worktree
2. Capture pass/fail counts and output
3. Include test results in comparison data

### Phase 4: Screenshots (Optional)

If `screenshot_command` is configured:
1. Run screenshot command in each worktree
2. Collect generated screenshots from convention path
3. Include in comparison report

If no command configured but `screenshots/` directory exists in worktree:
1. Include existing screenshots in report

### Phase 5: Comparison

1. Gather implementation data from all successful variants:
   - List of changed files
   - File diffs between variants
   - Test results (if available)
   - Screenshots (if available)
2. Execute comparison command (or built-in Claude comparison)
3. Generate structured comparison output

**Built-in Comparison Prompt:**

The comparison command receives a structured prompt requesting:
- Executive summary with recommendation
- Confidence level (high/medium/low)
- Per-file analysis
- Architectural observations
- Test result analysis (if available)
- Trade-off discussion

Output format is JSON matching the report template structure.

### Phase 6: Report Generation

Generate HTML report with:
1. **Executive Summary** - Recommendation with reasoning and confidence
2. **Overview Table** - Side-by-side metrics per variant
3. **File Comparison** - Syntax-highlighted diffs with expandable sections
4. **Test Results** - Pass/fail breakdown per variant (if available)
5. **Screenshots** - Side-by-side UI comparisons (if available)
6. **Detailed Analysis** - Claude's observations on each variant

---

## New Commands

### `orbit compare`

Trigger comparison for already-completed variants:

```bash
orbit compare <spec-name>
```

Useful for re-running comparison with different settings or after manual modifications.

### `orbit cleanup`

Remove worktrees and branches without finalizing:

```bash
orbit cleanup <spec-name>           # Clean up all variants
orbit cleanup <spec-name> --keep 2  # Keep variant 2, remove others
```

### `orbit finalize`

Adopt a variant as the final implementation:

```bash
orbit finalize <spec-name> --variant 2
```

Flow:
1. Checkout original branch (before variants were created)
2. Rebase changes from `{prefix}-2/{spec-name}` onto current branch
3. Delete all worktrees for the spec
4. Delete all `{prefix}-N/{spec-name}` branches

---

## Components and Interfaces

### New Package: `internal/variants`

```go
// Config holds variant execution configuration.
type Config struct {
    Count             int
    Parallel          bool
    BranchPrefix      string
    Guidance          []string  // Per-variant guidance (optional)
    CompareCommand    string
    ScreenshotCommand string
}

// Variant represents a single implementation variant.
type Variant struct {
    Number     int
    BranchName string
    WorktreePath string
    Status     Status  // pending, running, completed, failed
    Error      error
}

// Manager handles variant lifecycle.
type Manager struct {
    config    Config
    specName  string
    basePath  string
    variants  []*Variant
}

func NewManager(cfg Config, specName, basePath string) *Manager
func (m *Manager) Setup(ctx context.Context) error
func (m *Manager) RunAll(ctx context.Context, runFunc func(v *Variant) error) error
func (m *Manager) Cleanup(ctx context.Context) error
func (m *Manager) Finalize(ctx context.Context, variantNum int) error
```

### New Package: `internal/comparison`

```go
// Result holds comparison output.
type Result struct {
    Recommendation  int      // Recommended variant number
    Confidence      string   // high, medium, low
    Summary         string
    PerFileAnalysis []FileAnalysis
    TestResults     map[int]TestSummary
    Observations    []string
}

// Comparator generates comparisons between variants.
type Comparator struct {
    command string  // Empty for built-in
}

func NewComparator(command string) *Comparator
func (c *Comparator) Compare(ctx context.Context, variants []*variants.Variant) (*Result, error)
```

### New Package: `internal/report`

```go
// Generator creates HTML comparison reports.
type Generator struct {
    outputDir string
}

type ReportData struct {
    SpecName    string
    GeneratedAt time.Time
    Variants    []VariantData
    Comparison  *comparison.Result
    Diffs       []FileDiff
    Screenshots []Screenshot
}

func NewGenerator(outputDir string) *Generator
func (g *Generator) Generate(data *ReportData) error
```

### Modified: `internal/orbit/orbit.go`

Add variant-aware execution:

```go
type Orbit struct {
    // Existing fields...
    variantManager *variants.Manager  // nil for single-run mode
}

func (o *Orbit) Run(ctx context.Context) error {
    if o.variantManager != nil {
        return o.runWithVariants(ctx)
    }
    return o.runSingle(ctx)
}

func (o *Orbit) runWithVariants(ctx context.Context) error {
    // Setup worktrees
    // Run implementations (parallel or sequential)
    // Run comparison
    // Generate report
}
```

### Future: Agent Interface

Prepare for multi-agent support:

```go
// Agent represents an AI coding agent.
type Agent interface {
    Name() string
    RunPhase(ctx context.Context, phase int, prompt string) (*Result, error)
}

// ClaudeAgent implements Agent for Claude Code CLI.
type ClaudeAgent struct {
    command string
}

// Future: CodexAgent, etc.
```

This allows variant configuration like:

```yaml
variants:
  - agent: claude
    guidance: "Focus on simplicity"
  - agent: codex
    guidance: "Focus on simplicity"
```

---

## HTML Report Template

### Structure

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Comparison Report: {SpecName}</title>
    <style>/* Embedded CSS */</style>
</head>
<body>
    <header>
        <h1>Comparison Report: {SpecName}</h1>
        <p class="generated">Generated: {Timestamp}</p>
    </header>

    <section id="recommendation">
        <h2>Recommendation</h2>
        <div class="recommendation-card variant-{N}">
            <span class="badge">{Confidence} Confidence</span>
            <h3>Variant {N}</h3>
            <p>{Summary}</p>
        </div>
    </section>

    <section id="overview">
        <h2>Overview</h2>
        <table>
            <thead>
                <tr>
                    <th>Metric</th>
                    <th>Variant 1</th>
                    <th>Variant 2</th>
                    <!-- ... -->
                </tr>
            </thead>
            <tbody>
                <tr><td>Files Changed</td><td>12</td><td>8</td></tr>
                <tr><td>Lines Added</td><td>340</td><td>280</td></tr>
                <tr><td>Lines Removed</td><td>45</td><td>62</td></tr>
                <tr><td>Tests Passing</td><td>42/45</td><td>45/45</td></tr>
            </tbody>
        </table>
    </section>

    <section id="files">
        <h2>File Comparison</h2>
        <details>
            <summary>src/feature.go (+120/-15)</summary>
            <div class="diff-view">
                <!-- Syntax-highlighted diff -->
            </div>
        </details>
        <!-- More files... -->
    </section>

    <section id="tests">
        <h2>Test Results</h2>
        <!-- Test breakdown per variant -->
    </section>

    <section id="screenshots">
        <h2>Screenshots</h2>
        <div class="screenshot-grid">
            <!-- Side-by-side comparisons -->
        </div>
    </section>

    <section id="analysis">
        <h2>Detailed Analysis</h2>
        <!-- Claude's observations -->
    </section>
</body>
</html>
```

### Styling

- Clean, professional design
- Color-coded diffs (green for additions, red for deletions)
- Collapsible sections for large diffs
- Responsive layout for various screen sizes
- Print-friendly styles

---

## File Changes Summary

| File | Change |
|------|--------|
| `cmd/orbit/main.go` | Add variant-related flags and subcommands |
| `internal/orbit/orbit.go` | Add variant execution logic |
| `internal/variants/manager.go` | New - worktree lifecycle management |
| `internal/variants/manager_test.go` | New - tests |
| `internal/comparison/comparator.go` | New - comparison logic |
| `internal/comparison/comparator_test.go` | New - tests |
| `internal/report/generator.go` | New - HTML report generation |
| `internal/report/generator_test.go` | New - tests |
| `internal/report/templates/` | New - HTML templates |
| `internal/config/config.go` | Add variant configuration fields |
| `internal/claude/client.go` | Prepare for Agent interface extraction |

---

## Implementation Phases

### Phase 1: Core Variant Infrastructure

1. Add variant configuration to config package
2. Implement `internal/variants` package
   - Worktree creation/cleanup
   - Branch management
   - Metadata tracking
3. Add CLI flags for `--variants`, `--parallel`, `--branch-prefix`
4. Update `orbit.Run()` to support variant mode

### Phase 2: Variant Execution

1. Implement sequential variant execution
2. Implement parallel variant execution with shared rate limiting
3. Add per-variant logging (separate log directories)
4. Handle partial failures gracefully

### Phase 3: Comparison

1. Implement `internal/comparison` package
2. Create built-in comparison prompt for Claude
3. Add `--compare-command` for custom comparison
4. Implement diff generation between variants

### Phase 4: Report Generation

1. Implement `internal/report` package
2. Create HTML template with embedded CSS
3. Add syntax-highlighted diff rendering
4. Implement test result integration

### Phase 5: Screenshots

1. Add `--screenshot-command` configuration
2. Implement screenshot capture flow
3. Add fallback to manual screenshots
4. Include screenshots in HTML report

### Phase 6: Cleanup Commands

1. Implement `orbit cleanup` subcommand
2. Implement `orbit finalize` subcommand
3. Add `orbit compare` for re-running comparisons

### Phase 7: Polish

1. Add comprehensive tests
2. Update documentation
3. Add example configurations
4. Performance optimization for large diffs

---

## Testing Strategy

### Unit Tests

- Variant manager: worktree creation, branch naming, cleanup
- Comparator: diff generation, result parsing
- Report generator: HTML output, template rendering

### Integration Tests

- Full variant run with mock Claude
- Comparison flow with test fixtures
- Cleanup and finalize operations

### Manual Testing

- Real multi-variant runs
- Report visual inspection
- Cross-platform worktree behavior

---

## Future Considerations

### Multi-Agent Support

The Agent interface allows comparing different AI agents:

```yaml
variants:
  - agent: claude
  - agent: codex
  - agent: gemini
```

Each agent implementation handles its specific CLI/API.

### Comparison Metrics

Future versions could add:
- Code complexity metrics (cyclomatic complexity)
- Performance benchmarks
- Security scanning results
- Dependency analysis

### Report Formats

While HTML is the default, future versions could support:
- Markdown (simplified, for embedding in PRs)
- JSON (for programmatic consumption)
- PDF (for formal documentation)
