# OpenCode Agent Support

## Overview

Add OpenCode as a supported agent in Orbit, enabling users to orchestrate AI coding sessions using the OpenCode CLI. OpenCode is an open-source AI coding agent supporting multiple LLM providers (Anthropic, OpenAI, Gemini, AWS Bedrock, GitHub Copilot, etc.) through a unified CLI interface. This implementation follows the established agent pattern used by Codex, Kiro, and Copilot agents.

## Requirements

- The system MUST implement the `agents.Agent` interface for OpenCode
- The system MUST register OpenCode in the agent registry with name `opencode`
- The system MUST execute prompts using `opencode run "<prompt>"` for non-interactive operation
- The system MUST support session resumption via `--continue` flag (resumes last session)
- The system MUST support specific session resumption via `--session <id>` flag
- The system MUST support model selection via `--model provider/model` format (e.g., `anthropic/claude-sonnet-4-5`)
- The system MUST detect OpenCode installation via `exec.LookPath("opencode")`
- The system MUST retrieve version via `opencode --version` and parse the version from output (ignoring INFO log lines)
- The system MUST use `--format json` for output and detect errors when no valid JSON is returned (OpenCode may exit with code 0 on errors)
- The system SHOULD support custom CLI path via `AgentConfig.CLIPath`
- The system SHOULD support extra arguments via `AgentConfig.ExtraArgs`
- The system SHOULD implement session discovery from `~/.local/share/opencode/storage/message/ses_*/`
- The system MAY ignore `AutoApprove` config option (OpenCode works non-interactively without explicit config)

## Implementation Approach

**Files to Create:**
- `internal/agents/opencode/agent.go` - Agent interface implementation
- `internal/agents/opencode/errors.go` - Error classifier implementation

**Pattern Reference:**
Follow `internal/agents/codex/agent.go` and `internal/agents/codex/errors.go` as the primary template.

**CLI Mapping:**

| Orbit Concept | OpenCode CLI |
|---------------|--------------|
| Run prompt | `opencode run --format json "<prompt>"` |
| Resume last session | `opencode run --continue --format json` |
| Resume specific session | `opencode run --session <id> --format json` |
| Model selection | `--model provider/model` |
| JSON output | `--format json` (enables structured output, error detection) |
| Version check | `opencode --version` (outputs INFO log then version on last line) |

**Key Implementation Details:**

1. **buildArgs()**: Construct CLI arguments:
   - Base: `["run", "--format", "json"]`
   - Model: `["--model", "provider/model"]` if `Options["model"]` is configured
   - Extra args from config and options
   - Prompt as final argument

2. **Resume behavior**: Use `--continue` for `Resume()` method. The `--session <id>` flag exists but `--continue` is simpler and matches how other agents work (resume most recent session in the working directory).

3. **Version parsing**: Output format is:
   ```
   INFO  2026-01-27T12:16:29 +27ms service=models.dev file={} refreshing
   1.1.36
   ```
   Parse the last non-empty line for the version string.

4. **Error classification**: Use `--format json` flag. OpenCode may exit with code 0 even on errors. Detect errors by:
   - Checking if stdout contains valid JSON (errors produce plaintext stack traces instead)
   - If plaintext, parse for error patterns:
     - `ProviderModelNotFoundError` - Fatal error (invalid model)
     - `AuthenticationError`, `unauthorized`, `api key` - Fatal error
     - `rate limit`, `429`, `too many requests` - Retryable error
     - `connection`, `network`, `timeout` - Retryable error
     - `overloaded`, `503`, `service unavailable` - Retryable error

5. **Session discovery**: Sessions are stored in `~/.local/share/opencode/storage/message/<sessionID>/` where session IDs include the `ses_` prefix (e.g., `ses_400b135b9ffet34bhtIeaAuYKj`). Each directory contains message JSON files with `sessionID`, `role`, `time.created`, `model` fields. For discovery, list session directories and read the first message file to get metadata.

6. **Auto-approve**: OpenCode works non-interactively without explicit configuration. The `AutoApprove` config option can be ignored (no CLI flag needed).

**Session Storage Structure:**
```
~/.local/share/opencode/storage/
├── message/
│   └── <sessionID>/           # e.g., ses_400b135b9ffet34bhtIeaAuYKj
│       ├── msg_<id1>.json     # {id, sessionID, role, time.created, model, ...}
│       └── msg_<id2>.json
├── project/
│   └── <hash>.json            # {id, worktree, vcs, time.created}
└── session/
    └── <hash>                 # (internal use)
```

**Dependencies:**
- `github.com/arjenschwarz/orbit/internal/agents` - Agent interface and registry

**Out of Scope:**
- Apsis transcript parsing for OpenCode sessions (separate feature)
- Provider/model validation (OpenCode handles this internally)
- Mapping session to project via project hash (not needed for basic discovery)

## Risks and Assumptions

- **Risk**: OpenCode exits with code 0 on some errors (e.g., invalid model) | Mitigation: Use `--format json` and detect errors when output is not valid JSON
- **Risk**: Version output includes INFO log line before version | Mitigation: Parse last non-empty line of output
- **Assumption**: OpenCode CLI command is `opencode` (installed via npm, brew, or curl script)
- **Assumption**: Session IDs include the `ses_` prefix (e.g., `ses_400b135b9ffet34bhtIeaAuYKj`)
- **Assumption**: First message file in session directory provides sufficient metadata for discovery
- **Prerequisite**: User must have OpenCode installed and authenticated with at least one provider
