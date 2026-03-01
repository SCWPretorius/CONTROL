# Plan: CONTROL OpenClaw Replica (All Phases, Telegram + Discord)

**TL;DR**
Build a CLI-driven WebSocket gateway that wraps the existing HTTP/event infrastructure. Repurpose the current skill registry as a tool system with policy enforcement, convert integrations to a channel abstraction, add typed RPC methods/events for multi-client sync, and introduce plugin loading with manifest validation. Phases 1–5 run sequentially; each phase integrates tightly with existing code to avoid duplication.

---

## **Phase 1: CLI Bootstrap + Gateway Server**

### What to build
- CLI entry point with Command routing (gateway run, agent --message, send, status, config, doctor)
- WebSocket gateway wrapping the existing Express app
- Typed request/response/event contracts
- Session/presence tracking and graceful shutdown

### Key Files (new + modified)
1. **[src/entry.ts](src/entry.ts)** — new
   - Parse CLI args (Commander)
   - Route to `runGateway`, `runAgent`, `runSend`, etc.
   - Load config + DI seam (createDefaultDeps)
   
2. **[src/cli/program.ts](src/cli/program.ts)** — new
   - Command definitions and option schemas
   - Factories for each subcommand
   
3. **[src/gateway/server.impl.ts](src/gateway/server.impl.ts)** — new
   - Express + WS server composition
   - Load config at startup, validate, migrate if needed
   - Initialize plugin registry before binding
   - Add TLS/auth rate limiter (if needed)
   - Graceful close: drain active runs, notify clients
   
4. **[src/gateway/server-methods.ts](src/gateway/server-methods.ts)** — new
   - Method dispatch table: `chat`, `agent`, `send`, `status`, `models.*`, `config.*`, `plugins.*`
   - Idempotency cache (in-memory or Redis key dedup)
   - Response streaming for agent runs
   
5. **[src/gateway/runtime-state.ts](src/gateway/runtime-state.ts)** — new
   - In-memory state: connected clients/sessions, active runs, presence/health versions
   - Event bus for client subscriptions + server-push
   - Locking/atomicity for state mutations
   
6. **[src/gateway/device-pairing.ts](src/gateway/device-pairing.ts)** — new
   - Device identity + signed challenge validation (pairing approval workflow)
   - Token auth: bind tokens to devices
   
7. **Modify [src/index.ts](src/index.ts)**
   - Delegate to `entry.ts` (runCli)
   - Remove hardcoded startup
   
8. **[package.json](package.json)** — modify
   - Add `ws` library for WebSocket support
   - Bin entry: `"bin": { "openclaw": "dist/entry.js" }`
   - Update scripts: `start: node dist/entry.js gateway run`

### Handoff checklist
- [ ] CLI parses `gateway run`, `agent --message`, `send`, `status`, `config get/set`
- [ ] WS server starts on port (default 3000), binds to loopback by default
- [ ] Clients can connect, handshake with device identity, receive heartbeats
- [ ] Method dispatch receives typed requests, returns typed responses
- [ ] Graceful close drains pending requests
- [ ] Tests: WS multi-client connect/disconnect, method dispatch, idempotency

---

## **Phase 2: Agent Runtime + Tool System**

### What to build
- Tool registry wrapping the existing skills system
- Model selection + provider/model allowlisting
- Policy evaluator (workspace boundaries, channel/provider/group gating)
- Streaming agent executor (iterate LLM blocks, handle tool calls)

### Key Files (new + modified)
1. **[src/agents/tool-registry.ts](src/agents/tool-registry.ts)** — new
   - Load skills from existing `src/skills/skillLoader.ts`
   - Wrap in standard tool interface: `{ name, description, params schema, execute }`
   - Index core tools: read, write, exec, message (send)
   - Plugin tools added in Phase 4
   
2. **[src/agents/model-selection.ts](src/agents/model-selection.ts)** — new
   - Resolve desired model → provider/model ID
   - Allowlist filtering (global, agent, group, provider constraints)
   - Fallback cascade (primary → fallback 1 → fallback 2 → deterministic)
   - Reuse existing `src/llm/llmDecider.ts` status tracking
   
3. **[src/agents/policy-evaluator.ts](src/agents/policy-evaluator.ts)** — new
   - Check tool is allowed in workspace/channel
   - Check model is in provider allowlist
   - Check group/agent policies apply
   - Enforce workspace boundary (don't allow exec outside workspace root)
   
4. **[src/agents/agent-runner.ts](src/agents/agent-runner.ts)** — new
   - Execute tool calls: dispatch to tool registry, get results
   - Run agent: invoke LLM with tools, stream blocks (text, tool calls, results)
   - Finalize: emit summary block, save session
   - Handle retries, timeouts, context limits
   
5. **[src/agents/types.ts](src/agents/types.ts)** — new
   - Agent, Tool, Block, ToolCall, StreamEvent types
   
6. **Modify [src/gateway/server-methods.ts](src/gateway/server-methods.ts)**
   - Add `chat` method: load session → run agent → yield blocks → save session
   - Add `agent` method: same as chat

### Handoff checklist
- [ ] Tool registry loads all 11 existing skills, indexes by name
- [ ] Model selection resolves Anthropic/OpenAI/Google models with fallback
- [ ] Policy evaluator blocks tools outside workspace root
- [ ] Agent runner streams blocks in real-time, tool calls resolved correctly
- [ ] Session saved after run with conversation history
- [ ] Tests: tool filtering, model selection logic, policy enforcement, streaming blocks

---

## **Phase 3: Channel Adapters + Routing**

### What to build
- Channel abstraction (`connect`, `receive`, `send`, `capabilities`)
- Telegram + Discord adapters (wrap existing integrations)
- Inbound→routing→outbound pipeline (derive agent ID from peer/group, deliver formatted response)

### Key Files (new + modified)
1. **[src/channels/channel-interface.ts](src/channels/channel-interface.ts)** — new
   - Interface: `connect(credentials)`, `receive()` → AsyncIterator, `send(message, context)`, `capabilities()`
   - Normalized message model (content, peer, group, attachments, replyTo)
   
2. **[src/channels/telegram.ts](src/channels/telegram.ts)** — new
   - Wrap existing `src/integrations/telegram.ts` integration
   - Bot token → long-polling (or webhook mode)
   - Normalize Update → InboundMessage (user ID, chat ID, text, attachments)
   - Send: format msg for Telegram, call API, handle chunking (4096 char limit)
   - Pairing: derive pairing key from user ID + bot ID
   
3. **[src/channels/discord.ts](src/channels/discord.ts)** — new
   - Wrap existing `src/integrations/discord.ts` integration
   - Token + Socket Mode intents
   - Normalize Message event → InboundMessage (user ID, guild/DM ID, content, attachments)
   - Send: format msg for Discord, call API, handle chunking (2000 char limit)
   - Pairing: derive pairing key from user ID + bot ID + guild ID (if applicable)
   
4. **[src/channels/router.ts](src/channels/router.ts)** — new
   - Inbound event (peer, group, text) → derive agent ID + session key
   - Load/create session from session store
   - Run agent (Phase 2)
   - Format/chunk response for channel (per channel's chunking rules)
   - Call channel.send() for each chunk
   - Update session post-delivery
   
5. **[src/channels/registry.ts](src/channels/registry.ts)** — new
   - Active channels: list by ID, status, capabilities
   - Lazy-load on demand (credentials from config)
   
6. **[src/session/store.ts](src/session/store.ts)** — new
   - Load/save per-agent sessions: agentId + sessionKey → SessionState
   - Include conversation history, metadata, last-updated TTL
   - Prune expired sessions (via heartbeat or cleanup task)
   - File-based (JSON in `~/.openclaw/sessions/`) or SQLite key-value
   
7. **Modify [src/gateway/server-methods.ts](src/gateway/server-methods.ts)**
   - Add `send` method: explicit channel send (fallback for CLI)
   - Add `status` method: list active channels, connected clients, sessions
   
8. **Modify webhook handlers in [src/server/server.ts](src/server/server.ts)**
   - Remove HTTP-only event processing
   - Rewire Telegram/Discord webhooks to channel registry, then router

### Handoff checklist
- [ ] Telegram adapter: send message via CLI, receive in Telegram, send reply → appears in CLI
- [ ] Discord adapter: send DM to bot, receive in Discord, send reply → appears in CLI
- [ ] Session persists across gateway restart
- [ ] Routing derives correct agent ID from peer metadata
- [ ] Message chunking respects platform limits (Telegram 4096, Discord 2000)
- [ ] Tests: adapter I/O, message normalization, inbound→routing→outbound, TTL pruning

---

## **Phase 4: Plugin System**

### What to build
- Plugin manifest schema + loader (bundled + npm packages)
- Plugin registry with active state + diagnostics
- Config schema binding + validation
- Tool/channel/hook plugin entry points

### Key Files (new + modified)
1. **[src/plugins/manifest.ts](src/plugins/manifest.ts)** — new
   - Manifest schema: metadata, register fn, config schema, capabilities (tools, channels, hooks)
   - Validation: check tools/channels are well-formed, no parent escapes in hooks
   
2. **[src/plugins/loader.ts](src/plugins/loader.ts)** — new
   - Discovery: bundled plugins in `extensions/` + npm packages (require.resolve)
   - Safe dynamic import: use jiti, validate manifest, isolated error handling
   - Load config schema from manifest, apply defaults, reject on mismatch
   - Hook boundary: allow workspace + extension paths only
   
3. **[src/plugins/registry.ts](src/plugins/registry.ts)** — new
   - Active plugins: list by ID, status (ok, error), metadata, tool/channel index
   - Tool registry integration: add plugin tools to core tools
   - Channel registry integration: add plugin channels to core channels
   - Hook registry: store plugin hooks (on startup, on event, etc.)
   
4. **[src/config/schema.ts](src/config/schema.ts)** — new
   - Zod schema for config: port, host, webhookBaseUrl, LLM endpoints, integration tokens (Telegram, Discord), plugin configs
   - Include minimum versions for dependencies
   
5. **[src/config/io.ts](src/config/io.ts)** — modify
   - Load and validate config against schema
   - Apply plugin config schemas at load time
   - Immutable snapshot for runtime
   
6. **[src/config/migration.ts](src/config/migration.ts)** — new
   - Version-aware config migrations (v1 → v2, etc.)
   - Log changes to decision trace
   
7. **Hook system [src/plugins/hooks.ts](src/plugins/hooks.ts)** — new
   - Hook types: `on-startup`, `on-event`, `on-agent-run`, `on-tool-call`
   - Call registry: activate hooks in order, handle errors with isolation

### Handoff checklist
- [ ] Load bundled plugin from `extensions/example-plugin/manifest.json`
- [ ] Load npm package plugin dynamically
- [ ] Manifest validation rejects invalid tool/channel metadata
- [ ] Hook boundary check: reject paths outside workspace
- [ ] Plugin config schema applied at load; defaults injected
- [ ] Plugin tools appear in tool registry; plugin channels appear in channel registry
- [ ] Tests: manifest validation, loader error modes, registry activation, config schema binding

---

## **Phase 5: Security Hardening**

### What to build
- Auth rate limiting for WS handshake
- Tool execution policy enforcement
- Secrets snapshot lifecycle
- Media token + TTL
- Pairing approval workflow

### Key Files (new + modified)
1. **[src/gateway/rate-limiter.ts](src/gateway/rate-limiter.ts)** — new (or extend existing)
   - Rate limit WS handshake by origin/IP
   - Limit tool execution calls (per session, per channel)
   - Active sliding-window slots
   
2. **Modify [src/agents/policy-evaluator.ts](src/agents/policy-evaluator.ts)**
   - Enforce workspace boundary before tool exec
   - Check model in provider allowlist before LLM call
   - Check channel in allowed channels for outbound send
   - Check group policies (if agent is sub-agent)
   
3. **[src/secrets/runtime.ts](src/secrets/runtime.ts)** — modify
   - Load encrypted secrets from config
   - Snapshot at request start (decrypt once)
   - Clear snapshot after tool execution (no lingering in memory)
   - Audit log all secret access
   
4. **[src/media/token.ts](src/media/token.ts)** — new
   - Generate temporary media tokens (UUID + HMAC-SHA256 signed timestamp)
   - Token has TTL (default 1 hour)
   - GET /media/:token → serve file, decrement TTL counter
   - Cleanup on gateway shutdown
   
5. **[src/gateway/device-pairing.ts](src/gateway/device-pairing.ts)** — modify/complete
   - Device registry: store device ID + public key (one-time)
   - Challenge/response: server generates challenge, client signs with private key
   - Approval store: list approved devices per user
   - CLI: `openclaw auth approve` to manually approve new device
   
6. **Modify [src/config/io.ts](src/config/io.ts)**
   - Immutable config snapshot at startup (no mutations during run)
   - Policy constraints frozen per request
   
7. **[SECURITY.md](SECURITY.md)** — document
   - Workspace boundaries
   - Tool/model/provider/channel gating
   - Secrets lifecycle
   - Media token expiry
   - Pairing approval workflow

### Handoff checklist
- [ ] WS handshake rate-limited: 10 attempts/min per origin
- [ ] Tool calls checked against policy (workspace, model, provider, channel, group)
- [ ] Secrets decrypted, used in tool call, cleared after
- [ ] Media tokens have 1-hour TTL, cleanup on shutdown
- [ ] Pairing: new device generates challenge, user approves via CLI
- [ ] Tests: rate limiting, policy enforcement edge cases, secret lifecycle, token TTL

---

## **Verification Checklist**

### Build & CI
- [ ] `pnpm install` (ws, zod, jiti, commander deps added)
- [ ] `pnpm build` (TypeScript → dist/)
- [ ] `pnpm typecheck` (no type errors)
- [ ] `pnpm lint` (format + style pass)
- [ ] `pnpm test` (unit tests, 70% coverage minimum)

### Gateway E2E
- [ ] `pnpm test:e2e:gateway` — WS handshake, method dispatch, idempotency, event stream
- [ ] Multi-client connect/disconnect
- [ ] Pairing approval flow
- [ ] Session persistence across reconnect

### Channel I2E
- [ ] **Telegram smoke:** Send message via CLI → Telegram bot → reply → appears in CLI
- [ ] **Discord smoke:** Send DM to bot → receive in Discord client → reply appears in CLI
- [ ] Inbound DM→agent routing, session persistence, outbound chunking (2000/4096 char limits)

### Agent Runtime
- [ ] Tool: core tools (read, write, exec, message) execute correctly
- [ ] Model: selection logic + allowlist filtering
- [ ] Policy: tool blocked by workspace boundary, by channel, by provider
- [ ] Streaming: blocks yielded in real-time, summary on completion

### Plugin/Config
- [ ] Load bundled extension from `extensions/example-plugin/manifest.json`
- [ ] Load npm package plugin dynamically
- [ ] Config validation catches schema errors
- [ ] Config migration: old version → current, log changes
- [ ] Plugin manifest validation validates tool/channel metadata
- [ ] Hook boundary: reject paths outside workspace

### Security
- [ ] WS handshake rate-limited
- [ ] Tool execution gated by policy (workspace, model, provider, channel)
- [ ] Secrets decrypted/used/cleared correctly
- [ ] Media tokens expire after 1 hour

---

## **Decisions Made**

- **Phases:** All 5 phases implemented sequentially with dependency tracking
- **Channels:** Telegram + Discord (Phase 1 scope cut); defer WhatsApp/Slack/Signal/etc. to phase 2 post-release
- **Base code:** Build on existing HTTP gateway, skills, integrations, and memory — no greenfield
- **CLI framework:** Commander.js for command parsing + routing
- **WebSocket library:** `ws` npm package (lightweight, widely tested)
- **Config:** Zod validation + file-based (JSON in `~/.openclaw/config.json`)
- **Secrets:** Encrypted snapshot at request start, cleared after
- **Testing:** Vitest for unit, custom E2E harness for gateway, live smoke tests for channels (narrow to Anthropic + OpenAI)

---

## **Next Steps**

1. **Approve this plan** — review phases, dependencies, and integration points
2. **Refine scope cuts** — lock in which plugins/extensions ship in Phase 1 vs Phase 2
3. **Scaffold Phase 1** — start with CLI + gateway server (first 2–3 days of work)
4. **Parallel phase setup** — confirm 5-agent model or single-agent phased approach
5. **CI/CD bootstrap** — add GitHub Actions lint/build/test gates (optional but recommended)
