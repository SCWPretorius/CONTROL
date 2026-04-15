# Go Telegram Copilot Assistant Plan

## Problem

Build a personal assistant in Go that uses the GitHub Copilot SDK as its runtime and is reachable through Telegram.

## Current state

- The working tree is effectively a blank slate right now: only `github-copilot-sdk.md` is present in the filesystem.
- This should be treated as a clean rewrite, not a migration plan.
- The research note in `github-copilot-sdk.md` confirms the Go SDK is a thin client around the Copilot CLI runtime, with sessions, tool callbacks, permission handling, hooks, MCP, and resumable session support.

## Proposed approach

Create a fresh Go service with clear boundaries between:

1. Telegram ingress adapter
2. Message router
3. Copilot SDK client/session manager
4. Session persistence and lightweight memory
5. Config/env loading
6. Operational entrypoint for local running and deployment

The first milestone should stay narrow: a single-user or small-allowlist Telegram bot that can receive messages, route them into a Copilot SDK Go session, and send replies back reliably. Extra channels, complex memory systems, schedulers, and broad tool surfaces should be deferred until the Telegram loop is solid.

## Architecture draft

### Runtime

- Go application process
- Copilot CLI runtime started or connected via the Go SDK
- Telegram bot transport using a Go Telegram library
- Local persistent storage for session metadata/transcripts/artifacts

### Suggested package boundaries

- `cmd\assistant\main.go` - process entrypoint
- `internal\config` - env/config loading
- `internal\telegram` - Telegram adapter and update ingestion
- `internal\copilot` - Copilot SDK client/session lifecycle
- `internal\router` - message normalization and dispatch
- `internal\store` - session/chat persistence
- `internal\tools` - app-defined Copilot tools and permission policy

### Recommended initial behavior

- Restrict access to exactly one Telegram user/chat for the first version.
- Start with long polling before webhooks unless deployment constraints require webhook delivery.
- Keep one Copilot session per Telegram chat or per allowed user to preserve context.
- Use explicit permission handling in the SDK instead of blanket approval.
- Keep the initial tool surface minimal: filesystem-safe utilities, structured notes/tasks, and maybe shell access only if intentionally gated.

## Key implementation decisions to confirm

1. Whether Telegram should use long polling or webhooks.
2. Whether session persistence should be simple local files first or a database-backed design from day one.
3. Whether the assistant needs custom tools/MCP in the first milestone or just plain conversational Copilot sessions.

## Execution plan

### Phase 1 - Project foundation

- Initialize a Go module and directory layout.
- Add configuration loading for Telegram token, allowed chat/user IDs, Copilot CLI path/URL, runtime dirs, and model/session settings.
- Add a local development entrypoint and minimal README/run instructions.

### Phase 2 - Telegram transport

- Integrate a Go Telegram bot library.
- Implement inbound update handling, outbound replies, and basic access control.
- Normalize Telegram messages into an internal message shape.

### Phase 3 - Copilot runtime integration

- Create a Go Copilot SDK client wrapper for startup/shutdown.
- Implement session creation/resume keyed by Telegram chat/user.
- Add mandatory permission handling and basic event logging.

### Phase 4 - Routing and persistence

- Build a router that maps Telegram messages into Copilot session sends and returns assistant output.
- Persist session identifiers and conversation metadata locally.
- Decide whether to lean on Copilot session persistence, app-managed transcripts, or both.

### Phase 5 - Tools and safeguards

- Define a minimal set of app tools, if needed.
- Add guardrails around tool execution and permissions.
- Add admin/debug commands for reset, health, and session inspection.

### Phase 6 - Verification and deployment readiness

- Add tests for config loading, session keying, and Telegram message normalization.
- Validate end-to-end local message flow with a real Telegram bot token.
- Package the service for the target host and document required environment variables.

## Risks and considerations

- Copilot SDK Go relies on the Copilot CLI runtime, so local/dev/deploy environments need a clear CLI installation or path strategy.
- A public Telegram bot without allowlisting is the wrong default for a "personal assistant" because it exposes your runtime and tools.
- Tool approval policy is a product decision, not just a code detail.

## Draft assumptions for planning

- Use Go for all new application code.
- Start with Telegram only.
- Treat the repository as a clean Go rewrite.
- Lock the first version to a single Telegram user/chat.
- Prefer long polling, local file persistence, and a minimal initial tool surface unless the user says otherwise.
