# Decision Log: Custom Commands

## Decision 1: Post-command can be disabled

**Date**: 2024-12-19
**Status**: accepted

### Context

The design-critic and peer-review-validator agents both identified that there's no way to disable the default post-command. If a user doesn't want any post-completion step, they're stuck with the default behavior. Setting `post-command: ""` is treated as "not set" per requirement 4.2, causing fallback to the default.

### Decision

Add a `--no-post-command` flag and treat `post-command: ""` in config as explicitly disabled (not "not set").

### Rationale

Users need control over their workflow. Some may have CI-based verification and don't want Orbit running an additional Claude session. The distinction between "not set" (use default) and "explicitly empty" (disabled) is important.

### Alternatives Considered

- **Only `--no-post-command` flag**: Requires flag every time, no config file option - Rejected because config file support is a core requirement
- **Special sentinel value like `post-command: none`**: Non-standard YAML pattern - Rejected for being non-intuitive
- **No default post-command (opt-in only)**: Would change the user experience - Rejected to maintain the "batteries included" approach

### Consequences

**Positive:**
- Users can disable post-command via config or flag
- Clear distinction between "not set" and "disabled"

**Negative:**
- Slightly more complex config parsing logic
- Need to document the empty string behavior

---

## Decision 2: Post-command failure affects exit code

**Date**: 2024-12-19
**Status**: accepted

### Context

The requirements don't specify what happens when the post-command fails. Does Orbit exit with success (tasks completed) or failure (post-command failed)?

### Decision

If post-command fails, Orbit exits with a non-zero exit code. Logs clearly indicate that orchestration succeeded but post-command failed.

### Rationale

Safety first. If the post-command is verification (like the default pre-push-code-reviewer), a failure indicates problems that should block further actions. Users who want different behavior can disable the post-command.

### Alternatives Considered

- **Exit success with warning**: Could mask real issues - Rejected for safety reasons
- **Make failure behavior configurable**: Adds complexity - Rejected as premature optimization

### Consequences

**Positive:**
- Safe default behavior
- Clear signal to CI/CD pipelines

**Negative:**
- Post-command failures block "successful" orchestration
- May require users to disable post-command if they want tasks-only success

---

## Decision 3: Project-level config is working directory

**Date**: 2024-12-19
**Status**: accepted

### Context

"Project-level" config location is ambiguous. When running `orbit --tasks-file /some/other/path/tasks.md` from `/home/user/myproject`, which `.orbit.yaml` is project-level?

### Decision

Project-level config is `.orbit.yaml` in the current working directory where Orbit is invoked.

### Rationale

This matches user expectations and common CLI behavior. The working directory is where the user is running the command. If they want a specific project's config, they should `cd` to that directory first.

### Alternatives Considered

- **Tasks file directory**: Would require looking up the file system - Rejected as less intuitive
- **Git repository root**: Would require git integration - Rejected as adding unnecessary complexity

### Consequences

**Positive:**
- Simple, predictable behavior
- Matches common CLI patterns

**Negative:**
- Users with multiple projects must change directories

---

## Decision 4: Config loaded once at startup

**Date**: 2024-12-19
**Status**: accepted

### Context

Should config be re-read between phases, or loaded once at startup?

### Decision

Configuration is loaded once at startup. Changes to config files during orchestration are not picked up.

### Rationale

Consistency during a run. Changing config mid-run could cause unexpected behavior. This matches how most CLI tools work.

### Alternatives Considered

- **Re-read per phase**: Dynamic but unpredictable - Rejected for consistency

### Consequences

**Positive:**
- Predictable behavior
- Simpler implementation

**Negative:**
- Users must restart Orbit to pick up config changes

---

## Decision 5: Post-command log filename pattern

**Date**: 2024-12-19
**Status**: accepted

### Context

The requirements say post-command should be logged distinctly but don't specify the filename pattern.

### Decision

Use `post-completion-session.json` and `post-completion-session.txt` for post-command logs. Use phase 0 internally for compatibility with existing log manager.

### Rationale

Clear naming that doesn't conflict with phase numbers. Using phase 0 internally keeps the code simple while the filename clearly indicates what it is.

### Alternatives Considered

- **`phase-0-session.*`**: Confusing since phases start at 1 - Rejected
- **`final-session.*`**: Less descriptive - Rejected

### Consequences

**Positive:**
- Clear, unambiguous filenames
- Easy to find in log directory

**Negative:**
- Minor code change to handle special filename

---

## Decision 6: Generic default post-command

**Date**: 2024-12-19
**Status**: accepted

### Context

The original proposed default post-command referenced `pre-push-code-reviewer agent`, which may not exist on all Claude Code installations. Review agents identified this as a potential issue.

### Decision

Use a generic default: `"Review the implementation to verify it meets the requirements and all tests pass. If issues are found, fix them."`

### Rationale

A generic prompt works in any Claude Code environment. It achieves the same goal (verification before completion) without assuming specific tools exist.

### Alternatives Considered

- **Keep specific agent reference**: Would fail on systems without the agent - Rejected for portability
- **No default (opt-in only)**: Loses "batteries included" experience - Rejected to maintain value out of the box

### Consequences

**Positive:**
- Works in all Claude Code environments
- Still provides verification by default

**Negative:**
- Less powerful than a dedicated review agent

---

## Decision 7: Use OptionalString type for YAML presence tracking

**Date**: 2024-12-19
**Status**: accepted

### Context

Design review identified that the original `PostCommandSet bool` pattern would not work because YAML unmarshalling does not set this flag. Standard Go YAML unmarshalling cannot distinguish between "field omitted" and "field set to empty string".

### Decision

Use a custom `OptionalString` type that implements `yaml.Unmarshaler`. When the field is present in YAML (including empty string), `Set` is `true`. When omitted, `Set` remains `false`.

### Rationale

This pattern is idiomatic Go and works with standard YAML unmarshalling. It keeps the config struct typed and avoids pointer sprawl. The custom unmarshaller is simple and self-documenting.

### Alternatives Considered

- **Pointer fields (`*string`)**: `nil` = omitted, `""` = empty - Works but requires nil checks throughout the codebase
- **Parallel boolean fields**: Would not work without custom unmarshaller anyway - Rejected as less clean
- **Map-based unmarshalling**: Parse to `map[string]any` first - Rejected as more complex and loses type safety

### Consequences

**Positive:**
- Correctly distinguishes omitted from empty
- Single custom type handles all optional string fields
- Clean API with `IsPostCommandDisabled()` helper

**Negative:**
- Requires understanding the OptionalString pattern
- Slightly more verbose than plain strings

---

## Decision 8: Separate method for post-completion logging

**Date**: 2024-12-19
**Status**: accepted

### Context

The original design used `const PostCompletionPhase = 0` as a magic number passed to `SaveSession()`. Design review identified this as a code smell: 0 could collide with "not found" return values and spreads special-casing throughout the codebase.

### Decision

Add a separate `SavePostCompletionSession()` method to the log manager instead of overloading `SaveSession()` with a magic phase number.

### Rationale

Explicit is better than implicit. A separate method clearly communicates intent, avoids magic numbers, and keeps the `SaveSession()` API clean for regular phases.

### Alternatives Considered

- **Phase 0 constant**: Magic number is a code smell - Rejected
- **Negative phase number (-1)**: Still a magic number - Rejected
- **Typed enum**: More complex for little benefit - Rejected as overkill

### Consequences

**Positive:**
- No magic numbers
- Clear separation of concerns
- `SaveSession()` remains clean

**Negative:**
- Some code duplication between SaveSession and SavePostCompletionSession
- Two methods to maintain

---

## Decision 9: Use Viper for configuration management

**Date**: 2024-12-19
**Status**: accepted

### Context

The original design used custom code with `yaml.v3` and a custom `OptionalString` type for config loading and merging. This required ~100 lines of custom code. The user noted that most of their Go CLI apps use Cobra/Viper.

### Decision

Use Viper (without Cobra) for configuration management. Viper handles YAML loading, file discovery, merging, and environment variable support.

### Rationale

Viper provides all needed functionality out of the box:
- YAML parsing and file discovery
- Automatic merging with correct priority
- Environment variable support (`ORBIT_COMMAND`, `ORBIT_POST_COMMAND`)
- Built-in `IsSet()` for presence detection

This eliminates the need for custom `OptionalString` type and reduces code complexity.

### Alternatives Considered

- **Custom code with yaml.v3**: More control, fewer dependencies - Rejected because Viper is simpler
- **Cobra + Viper**: Full CLI framework - Rejected as overkill for single-command tool
- **koanf**: Alternative config library - Rejected because user already uses Viper in other projects

### Consequences

**Positive:**
- Simpler code, less to maintain
- Environment variable support for free
- Consistent with user's other projects

**Negative:**
- Additional dependency (Viper + its transitive deps)
- Less explicit control over merging logic

---
