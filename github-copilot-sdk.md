# Research Report: `github/copilot-sdk`

## Executive Summary

[github/copilot-sdk](https://github.com/github/copilot-sdk) is a multi-language SDK family for embedding Copilot’s agent runtime into applications, with first-party Node.js/TypeScript, Python, Go, and .NET implementations in one monorepo plus a separate Java implementation in [github/copilot-sdk-java](https://github.com/github/copilot-sdk-java).[^1][^17]

Architecturally, the SDKs are thin-but-opinionated clients around a Copilot CLI server: the app talks to an SDK client, the SDK talks JSON-RPC to the CLI, and sessions expose message sending, event streaming, tool execution, permission mediation, and persistence controls.[^1][^4][^5]

The family is deliberately feature-parallel across languages: `createSession`/`resumeSession`, streaming events, custom tools, hooks, MCP servers, skill directories, custom agents, and infinite-session settings all map down to the same wire payload concepts, even though each language wraps them idiomatically.[^4][^6][^8][^10][^12][^13][^15]

The most important implementation nuance is that the SDKs are not “LLM SDKs” in the direct-HTTP sense; they are Copilot-runtime SDKs. That gives you a production agent loop, but it also means you inherit CLI protocol/version constraints, permission/user-input handlers, and operational choices about where the CLI process runs and how session state is stored.[^1][^4][^5][^11][^14]

## Architecture / System Overview

```text
┌──────────────────────┐
│ Your application     │
│  - UI / API / worker │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│ SDK client           │
│  - start/stop        │
│  - create/resume     │
│  - event handlers    │
│  - tool callbacks    │
└──────────┬───────────┘
           │ JSON-RPC (stdio or TCP)
           ▼
┌──────────────────────┐
│ Copilot CLI server   │
│  - auth              │
│  - session manager   │
│  - planning/runtime  │
│  - tool orchestration│
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│ Models / providers   │
│  - Copilot-backed    │
│  - BYOK providers    │
└──────────────────────┘
```

The repo’s own docs describe the core pattern exactly this way: application → SDK client → Copilot CLI over JSON-RPC, with deployment differences mostly coming from where the CLI runs, how users authenticate, and how session state is managed.[^1][^14]

On the client side, every language implementation has the same high-level shape:

- a client object that either spawns a local CLI process or connects to an external CLI server;[^4][^6][^8][^10]
- a session object that sends prompts, emits events, and brokers tool/permission/user-input callbacks;[^5][^7][^9]
- a shared protocol/version contract, currently pinned at SDK protocol version `3`, with clients rejecting servers below minimum protocol `2`.[^4][^6][^10][^11]

## What the SDK Actually Exposes

At the package boundary, the SDKs export a compact surface: client/session types, tool-definition helpers, config types, session-event types, and model metadata types.[^2][^3]

In TypeScript, `index.ts` exports `CopilotClient`, `CopilotSession`, `defineTool`, `approveAll`, and a large set of typed configs and event interfaces, which is a good proxy for the intended public API shape across the family.[^2]

In Python, `__init__.py` exports the same conceptual pieces—client, session, tool helper, provider/MCP/session/model types—which reinforces that the language bindings are meant to be semantically parallel rather than separate products.[^3]

## Core Runtime Model

### 1. Client lifecycle

All implementations support two transport modes: spawn a CLI server process locally, or connect to an existing external server via a host/port-style URL.[^4][^6][^8][^10]

The TypeScript client validates mutually exclusive config like `cliUrl` vs `useStdio`/`cliPath`, stores connection state, and starts by spawning the CLI only when not connected to an external server, then connects and verifies protocol compatibility.[^4]

The Python client follows the same flow: resolve connection mode, optionally start the CLI process, connect, verify protocol version, and track state transitions through `disconnected` → `connecting` → `connected` or `error`.[^6]

Go and .NET implement the same lifecycle in idiomatic ways: Go’s `Client.Start` holds a start/stop lock, optionally starts the CLI, connects, verifies protocol version, and moves to `StateConnected`; .NET’s `StartAsync` either connects to an external server or starts a child process, then verifies protocol compatibility before exposing the RPC client.[^8][^10]

### 2. Session creation and resumption

A recurring implementation pattern is that sessions are registered locally **before** the `session.create` or `session.resume` RPC is sent, specifically so early CLI-emitted events like `session.start` are not dropped.[^4][^6][^8][^10]

That pattern appears in TypeScript, Python, Go, and .NET, which strongly suggests this is a fundamental property of the runtime rather than an incidental language choice.[^4][^6][^8][^10]

### 3. Message loop and waiting semantics

Each session object exposes a non-blocking send plus a convenience `sendAndWait`/`send_and_wait`/`SendAndWait` helper that waits for the `session.idle` event rather than for a single RPC response, meaning the “done” signal is session lifecycle state, not merely request completion.[^5][^7][^9]

TypeScript and Python both explicitly document that the timeout only controls how long the client waits; it does **not** abort in-flight agent work.[^5][^7]

## Protocol and Compatibility Model

The current SDK protocol version is `3` in TypeScript, Python, Go, and .NET, and each file says that value must match what the `copilot-agent-runtime` server expects.[^11]

At the same time, the main client implementations still define minimum supported protocol version `2`, which means the SDK family is intentionally straddling a compatibility boundary rather than assuming lockstep rollout.[^4][^6][^10]

The clearest visible compatibility shim is in TypeScript session handling: protocol v3 treats tool and permission work as broadcast session events, while `_handlePermissionRequestV2` remains as a back-compat adapter and explicitly rejects `no-result` decisions against v2 servers.[^5]

## Tools, Permissions, Hooks, and User Input

### Permissions are mandatory by design

All four primary SDKs require an `onPermissionRequest` handler (or language-equivalent) at session creation time, and the create-session payload always enables permission requests on the wire.[^4][^6][^8][^10]

That is an important architectural opinion: the runtime assumes tool use can cross trust boundaries, so permission mediation is not an optional afterthought; it is part of the contract.[^4][^6][^8][^10]

### Tool execution is callback-driven

Sessions register tool handlers locally, then resolve tool invocations when the CLI asks for them; TypeScript, Python, and Go all do this by mapping tool names to handlers inside the session object rather than in the client.[^5][^7][^9]

The create-session payloads show the same wire shape across languages: tool name, description, parameters/schema, and flags such as overriding built-in tools or skipping permission checks.[^4][^6][^8][^10]

### Protocol v3 turns tool calls into broadcast events

The TypeScript and Python session implementations explicitly treat `external_tool.requested` and `permission.requested` as broadcast events that the SDK handles internally before forwarding the event stream to the application.[^5][^7]

Go makes the concurrency model especially explicit: broadcast work runs separately so it does not block the JSON-RPC read loop, while user event handlers are serialized through a single consumer goroutine for FIFO delivery.[^9]

### Hooks are first-class lifecycle extension points

The hook docs frame hooks as lifecycle callbacks registered once per session, covering session start/end, user prompt submission, pre/post tool use, and error handling.[^12]

The Go session implementation confirms that those hook names are not just documentation; it dispatches concrete hook types like `preToolUse`, `postToolUse`, `userPromptSubmitted`, `sessionStart`, `sessionEnd`, and `errorOccurred` through a typed hook invocation path.[^9]

### User input is a separate contract from permissioning

The TypeScript session has a dedicated `registerUserInputHandler` path for `ask_user`-style requests, and the Python and Go implementations mirror that separation with dedicated user-input handlers rather than overloading permission callbacks.[^5][^7][^9]

## Event Model and Streaming

The streaming guide says a session can emit both ephemeral events (for real-time deltas and progress) and persisted events (for replayable history), all wrapped in a common event envelope containing `id`, `timestamp`, `parentId`, `type`, and `data`.[^13]

That guide also makes an important operational point: ephemeral events are **not** replayed on resume, while persisted events are, so consumers should treat streaming deltas as UI/live-observability signals and persisted events as the durable record.[^13]

On the API side, TypeScript supports both wildcard and typed-event subscriptions, Python exposes a generic callback, and Go uses ordered handler registration plus a single-consumer event queue to keep delivery deterministic for application code.[^5][^7][^9]

## Session Persistence and Lifecycle

The source-level contract for persistent sessions is visible in every session type: when infinite/resumable sessions are enabled, a workspace path exists and is documented as containing `checkpoints/`, `plan.md`, and `files/`.[^5][^7][^9]

On lifecycle semantics, the Go client is the clearest statement of intent: `Stop` disconnects active sessions and preserves on-disk session state for later resumption, while `DeleteSession` permanently removes conversation history, planning state, and artifacts from disk.[^8]

This distinction matters for product design: “close the session” and “erase the session” are different operations in the SDK family, and applications should model them separately in UI and APIs.[^8]

## Feature Parity: What `createSession` / `resumeSession` Can Configure

Across TypeScript, Python, Go, and .NET, the create/resume payloads consistently forward the same major knobs to the CLI runtime:[^4][^6][^8][^10]

| Capability | Evidence |
|---|---|
| Model + reasoning effort | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| Tool definitions + available/excluded tool filters | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| System message | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| Provider/BYOK config | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| User input + hooks + streaming toggles | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| Working directory + config directory | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| MCP servers | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| Custom agents + selected agent | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |
| Skill directories + disabled skills | Forwarded in TypeScript, Python, Go, and .NET create-session code.[^4][^6][^8][^10] |
| Infinite sessions | Forwarded in all four primary SDKs.[^4][^6][^8][^10] |

The most revealing constant in these payloads is `envValueMode: "direct"`, which shows the SDK is intentionally passing environment-derived values through to the CLI runtime in direct form rather than via some higher-level indirection layer.[^4][^6][^8][^10]

## Authentication and Deployment Nuances

The auth docs describe four supported authentication families: GitHub signed-in user, OAuth GitHub App, environment-variable tokens, and BYOK provider credentials.[^16]

The setup docs describe the deployment problem less as “which SDK do I use?” and more as “where does the CLI live, how do users authenticate, and how is session state managed?”, with explicit personas for local/hobbyist, internal multi-user, SaaS, and platform-style backends.[^14]

One subtle but important implementation detail is that “install the CLI separately” is not the entire story anymore. The monorepo README still presents separate CLI installation as the basic getting-started path, but the language implementations already diverge in how much bundling they can do themselves.[^1][^4][^6]

- TypeScript resolves a bundled CLI path from `@github/copilot/sdk` / `@github/copilot` package layout, implying tight integration with the Node Copilot package.[^4]
- Python looks for a bundled CLI binary inside `copilot/bin` and errors only if no platform-specific bundled binary is available and no explicit `cli_path` is supplied.[^6]
- Go does **not** show the same package-bundled resolution path; its test harness instead expects `COPILOT_CLI_PATH` or a separately available Node-installed CLI artifact.[^8][^18]
- Java’s README is the most explicit about the old-school model: Java 17+, Copilot CLI installed in `PATH` or supplied via `cliPath`, and a separate repository that tracks upstream SDK behavior.[^17]

For integrators, the practical takeaway is: the runtime architecture is uniformly CLI-backed, but packaging/distribution ergonomics vary materially by language.[^4][^6][^17][^18]

## MCP and External Tooling

The MCP docs position MCP as a way to extend Copilot sessions with external tools/data sources, and the SDK supports both local/stdin-stdout subprocess servers and remote HTTP/SSE servers.[^15]

This is not bolted on outside the main session model; MCP server configuration is part of the same session creation payload as tools, provider config, skills, and custom agents.[^4][^6][^8][^10][^15]

That matters architecturally because it means “external capability surface” in Copilot SDK is a layered stack:

1. built-in first-party CLI tools;[^1]
2. in-process application-defined tools via SDK callbacks;[^4][^5][^7][^9]
3. out-of-process MCP tools via local or remote MCP servers.[^15]

## Language-by-Language Assessment

| SDK | Status / shape | Notable implementation detail |
|---|---|---|
| Node.js / TypeScript | Most explicit typed public API in the monorepo, including exported event/config types.[^2] | Strong feature-forwarding surface and a bundled-CLI resolution path via `@github/copilot` package layout.[^4] |
| Python | Very close semantic match to Node, but with Pythonic dict/config and async callback style.[^3][^6][^7] | Can use a bundled CLI binary from the wheel when available.[^6] |
| Go | Thin, readable implementation with the clearest concurrency commentary in session dispatch code.[^8][^9] | Event delivery is FIFO through a single consumer goroutine; broadcast tool/permission work is isolated from the read loop.[^9] |
| .NET | Full-featured client with async disposal and strongly typed request construction.[^10] | Closely mirrors the shared runtime contract and forwards nearly the full feature surface to `CreateSessionRequest`.[^10] |
| Java | Separate repo, explicitly tracking upstream pre-GA SDKs rather than living in the main monorepo.[^1][^17] | Operationally more “bring your own CLI” than Python/Node at present.[^17] |

## Key Repositories Summary

| Repository | Purpose | Key files |
|---|---|---|
| [github/copilot-sdk](https://github.com/github/copilot-sdk) | Primary SDK monorepo for Node.js, Python, Go, and .NET plus shared docs.[^1] | `README.md`, `nodejs/src/client.ts`, `nodejs/src/session.ts`, `python/copilot/client.py`, `go/client.go`, `go/session.go`, `dotnet/src/Client.cs`.[^1][^4][^5][^6][^8][^10] |
| [github/copilot-sdk-java](https://github.com/github/copilot-sdk-java) | Separate Java implementation tracking the upstream SDK family.[^17] | `README.md`, `jbang-example.java`.[^17] |

## Practical Recommendations

If you are evaluating this SDK for real product work, the first architectural decision is not language choice; it is **deployment topology**: local CLI per app instance, external shared CLI, or isolated CLI per user/tenant.[^14]

After that, the next two decisions are:

1. whether your product wants in-process tools, MCP tools, or both;[^5][^9][^15]
2. whether you want resumable/persistent sessions with explicit deletion semantics, because that becomes part of your product’s data model and storage story very quickly.[^5][^7][^8]

My opinionated read: this SDK is strongest when you want **Copilot’s runtime behavior**—planning, tool orchestration, session persistence, and event streams—not merely model access. If you only want direct model calls, this stack is heavier than necessary; if you want agent workflows with a real session model, the CLI-backed architecture is exactly the point.[^1][^4][^5][^14]

## Confidence Assessment

**High confidence** on the core architecture, lifecycle, protocol/versioning, feature parity, and session semantics because those claims are backed directly by implementation files in the main repo and the Java companion repo.[^4][^5][^6][^8][^9][^10][^17]

**Medium confidence** on fast-moving product positioning details like the exact intended packaging story for every language, because the top-level README still emphasizes separate CLI installation while Node and Python source already contain bundled-CLI resolution logic; that looks like an evolving product surface rather than a contradiction I can fully resolve from the inspected files alone.[^1][^4][^6]

## Footnotes

[^1]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `README.md:9-12, 18-22, 40-52` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^2]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `nodejs/src/index.ts:1-50` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^3]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `python/copilot/__init__.py:1-74` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^4]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `nodejs/src/client.ts:55-59, 91-101, 214-239, 248-265, 317-340, 366-430, 524-635, 661-741` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^5]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `nodejs/src/session.ts:34-38, 65-110, 131-218, 269-448, 459-560` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^6]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `python/copilot/client.py:58-65, 67-84, 87-117, 119-181, 198-257, 259-310, 312-375, 427-622, 624-760` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^7]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `python/copilot/session.py:47-117, 119-225, 227-259, 263-430` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^8]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `go/client.go:54-104, 121-208, 244-295, 316-374, 446-454, 485-577, 605-695, 739-780` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^9]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `go/session.go:43-75, 78-84, 121-223, 248-368, 373-446, 449-620` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^10]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `dotnet/src/Client.cs:24-65, 84-154, 179-233, 368-475` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^11]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `nodejs/src/sdkProtocolVersion.ts:5-16`, `python/copilot/sdk_protocol_version.py:1-16`, `go/sdk_protocol_version.go:1-10`, `dotnet/src/SdkProtocolVersion.cs:1-19` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^12]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `docs/features/hooks.md:1-35, 46-55, 68-76, 109-118` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^13]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `docs/features/streaming-events.md:1-8, 18-49, 50-78` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^14]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `docs/setup/index.md:1-8, 24-41, 46-75, 98-129` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^15]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `docs/features/mcp.md:1-5, 7-24, 26-80, 151-177` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^16]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `docs/auth/index.md:1-12` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).

[^17]: [github/copilot-sdk-java](https://github.com/github/copilot-sdk-java) — `README.md:17-36, 44-83, 124-155` and `jbang-example.java:1-31` (commit `407665c956952df22ebe99a43d8a751a0816e922`).

[^18]: [github/copilot-sdk](https://github.com/github/copilot-sdk) — `go/test.sh:1-31` (commit `698b2598e32e0958a5298e6dcd715970e0a94d53`).
