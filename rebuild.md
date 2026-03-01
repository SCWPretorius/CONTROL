# Plan: High-Parity OpenClaw Replica

You want a high-parity rebuild (not full 1:1), keeping the same stack (TypeScript + pnpm + Bun/Node), with CLI + Gateway WS/API + messaging channels. The replica should preserve OpenClaw's control-plane model: one gateway process as system-of-record, typed WS methods/events, plugin/channel registries, model/tool orchestration, and policy-driven safety. The handoff below is structured so a coding agent can execute in phases with minimal ambiguity while still allowing scope cuts.

## Architecture Breakdown (what to replicate)

- **Entry + CLI shell:** boot/runtime guards in `src/entry.ts`, command graph in `src/cli/program/build-program.ts`, DI seam in `src/cli/deps.ts`.
- **Gateway composition root:** process startup, config migration, runtime assembly, and WS wiring in `src/gateway/server.impl.ts`.
- **WS/API contract:** method/event model and dispatch in `src/gateway/server-methods.ts`, `src/gateway/server-methods-list.ts`, `docs/concepts/architecture.md`.
- **Agent/tool runtime:** tool assembly/policy filtering in `src/agents/pi-tools.ts`, `src/agents/openclaw-tools.ts`, runner flow in `src/agents/pi-embedded-runner/compact.ts`.
- **Plugin/channel extensibility:** plugin loading/registry in `src/plugins/loader.ts`, `src/plugins/registry.ts`, channel normalization in `src/channels/registry.ts`.
- **State + security surfaces:** config/session/secrets in `src/config/config.ts`, `src/config/io.ts`, `src/config/sessions/store.ts`, `src/secrets/runtime.ts`, security boundaries in `SECURITY.md`.

## Implementation Steps

1. **Replica spec + scope document**
   - Distill bounded scope and parity targets using README, VISION, architecture docs, security policy.
   - Define what "high parity" means: core features + plugin/channel framework, defer long-tail extensions.
   - Explicitly list which channels are "phase 1" (Telegram + Discord) vs "phase 2" (rest).

2. **Protocol-first contracts**
   - Define request/response/event models and JSON schemas for gateway WS.
   - Handshake/auth flow, idempotency semantics, and token/device identity binding.
   - Required core methods: `connect`, `disconnect`, `chat`, `agent`, `send`, `status`, `models.*`, `config.*`.
   - Event stream: `agent`, `chat`, `presence`, `tick`, `health`, `shutdown`.

3. **CLI + bootstrap path**
   - Implement `runCli` (async entry point after arg parsing/profile setup).
   - Command registration system compatible with Commander.
   - Dependency injection seam: `createDefaultDeps` model.
   - Early commands: `gateway run`, `agent --message`, `message send`, `status`, `config get/set`.

4. **Gateway server composition**
   - Start/close lifecycle with graceful shutdown.
   - WS/HTTP server binding and port handling (loopback vs LAN).
   - Config snapshot load/validate/migrate at startup.
   - Plugin registry initialization before WS binding.
   - TLS/auth rate limiter setup.

5. **Runtime state + method dispatcher**
   - In-memory state: connected clients/nodes, active runs, plugin/channel registries, health snapshots.
   - Method router: dispatch typed requests to handlers; dedupe cache for idempotency.
   - Presence/health version tracking for client-side event coalescing.
   - Event multiplexer: server-push events to subscribed clients.

6. **Agent/tool runtime**
   - Model resolver: provider/model allowlisting, catalog fallback.
   - Tool registry: core tools (read/write/exec/message) + plugin tools, policy filtering.
   - Policy engine: apply global/agent/group/provider constraints; workspace boundary enforcement.
   - Stream/block handling: yield tool results, formatted text, structured blocks.

7. **Channel abstraction + first adapters**
   - Channel interface: `connect`, `receive`, `send`, `status`, `capabilities`.
   - Plugin registry entry point for channels (meta, pairing hooks, routing).
   - Telegram adapter: bot token → Update polling/webhook, message normalization, outbound send.
   - Discord adapter: token + Socket Mode, same I/O normalization, multi-server session handling.
   - Session routing: inbound peer/group → agent allocation, per-channel chunking rules.

8. **Config + persistence contract**
   - Config schema + validation using Zod or JSON Schema.
   - Config file path precedence: `~/.openclaw/config.json` (or env override).
   - Session store: indexed by agent ID + session key, with TTL/pruning.
   - Credentials snapshot: runtime secrets binding without storing in config.
   - Migration path: old config versions → current, with explicit change logging.

9. **Plugin loader + registry**
   - Plugin manifest schema: metadata, register fn, config schema, capabilities (tools/channels/hooks).
   - Discovery: bundled extensions under `extensions/*` + npm packages (if installed).
   - Safe loading: boundary checks, path validation, dynamic import via jiti.
   - Active registry: return list of plugins with status, errors, and capability indexes (tool names, channel IDs, etc.).

10. **Security hardening**
    - Auth rate limiting for WS handshake per origin/IP.
    - Tool execution policy: workspace boundaries, model/provider/channel gating.
    - Pairing store: device identity + signed challenges, approval workflows.
    - Secrets runtime snapshot: decrypt/activate only when needed, clear after request.
    - Media token/TTL: temporary file serving with expiry, cleanup on gateway shutdown.
    - Hook boundary enforcement: only allow discovery within workspace or explicit manifests.

11. **Testing + validation**
    - Unit tests: config parsing, policy evaluation, routing logic, tool filtering.
    - Gateway E2E: multi-client WS flow, method dispatch, event streams, idempotency.
    - Channel I2E: adapter I/O, message normalization, session key derivation.
    - Live channel smoke: real Telegram bot, Discord DM, basic message round-trip.
    - Build/release checks: reproducible pack, installer smoke test (non-root + root).

12. **Operational commands + diagnostics**
    - `openclaw gateway` (foreground + daemonize flag).
    - `openclaw agent --message` (RPC path).
    - `openclaw send` (fallback send for explicit channels).
    - `openclaw status` (gateway health, connected clients, sessions, plugins).
    - `openclaw config` (get/set/validate).
    - `openclaw doctor` (config validation, legacy detection, quick diagnostics).

## Component Contract Spec

### Gateway Server
- **Methods:**
  - `start(port?, opts?)` → `GatewayServer`
  - `close(reason?)` → Promise void
  - `getState()` → runtime state snapshot
  - Route methods: call handler, cache idempotency, yield response/stream events
- **Events:** `agent`, `chat`, `presence`, `tick`, `health`
- **Invariants:** exactly one gateway per host, single point of truth for routing/sessions

### Agent Engine
- **Methods:**
  - `resolveModel(desired: string, allowlist?, fallback?)` → provider/model ID
  - `buildToolset(workspace, agent, group, channel?, policies?)` → Tool[]
  - `runAgent(session, message, model, tools, context?)` → AsyncIterator<Block>
- **Contract:** streaming blocks with tool calls + results, final summary

### Channel Adapter
- **Methods:**
  - `connect(credentials)` → Promise void
  - `receive()` → AsyncIterator<InboundMessage>`
  - `send(outbound, context?)` → Promise<SendResult>`
  - `capabilities()` → ChannelCapabilities`
- **Contract:** normalized message model, session/peer routing metadata

### Plugin System
- **Methods:**
  - `loadPlugins(config, workspace?)` → PluginRegistry
  - `validateManifest(plugin)` → { ok, errors? }
  - `registerPlugin(plugin, runtime)` → void
- **Contract:** safe boundaries, config/metadata decoupling, active runtime state

### Config Store
- **Methods:**
  - `readConfig(path?)` → Config`
  - `validateConfig(config)` → { ok, errors? }`
  - `writeConfig(config, path?)` → Promise void`
  - `migrateConfig(old)` → { migrated, changes }`
- **Contract:** file atomicity, schema evolution, immutable snapshots for runtime

### Session Store
- **Methods:**
  - `loadSession(agentId, sessionKey)` → SessionState | null`
  - `saveSession(agentId, sessionKey, state)` → Promise void`
  - `pruneExpired(now)` → Promise { deleted }`
- **Contract:** per-agent scoping, TTL-based cleanup, JSON serialization

## Runtime Flow Contract

1. **Client/Channel Event In**
   - Webhook/polling normalizes event → typed message object
   - Route resolution: derive agent ID + session key from inbound metadata (account, peer, group)
   - Session load: fetch history if exists; initialize if new

2. **Gateway Processing**
   - WS client sends `{type:"req", id, method, params}`
   - Router dispatches to method handler (chat, agent, send, models, etc.)
   - Auth check: token, pairing, device identity validation
   - State mutation: update presence, health, active runs

3. **Agent Execution**
   - Build toolset: core + plugin tools, apply policy filters
   - Model resolution: preferred provider/model, allowlist gates, fallback cascade
   - Run agent: iterate blocks, yield back to client as server-push events
   - Tool calls: policy gate, execute in sandbox, return results to agent

4. **Outbound Delivery**
   - Format response: apply channel chunking, media serialization
   - Channel send: call adapter.send() with normalized message
   - Retry/dedupe: idempotency key tracking for safe retries
   - Callback: update session, emit completion event to client

## Verification Checklist

### Build & CI
- [ ] `pnpm install` (no missing deps)
- [ ] `pnpm build` (reproduces `dist/`)
- [ ] `pnpm tsgo` (no type errors)
- [ ] `pnpm check` (lint + format pass)
- [ ] `pnpm test` (unit + gateway tests pass, 70% coverage)

### Gateway E2E
- [ ] `pnpm test:e2e` (WS handshake, method dispatch, idempotency, event stream)
- [ ] Test: multi-client connect/disconnect
- [ ] Test: pairing approval flow
- [ ] Test: session persistence across reconnect

### Channel I2E
- [ ] Telegram: send message via CLI, receive reply in Telegram app
- [ ] Discord: send DM to bot, get reply in Discord client
- [ ] Test: inbound DM→agent, route to correct session, deliver outbound

### Agent Runtime
- [ ] Tool: read/write/exec basics
- [ ] Model: selection logic + allowlist filtering
- [ ] Policy: tool blocked by workspace boundary, by channel, by provider
- [ ] Streaming: blocks yielded in real-time, final summary on completion

### Plugin/Config
- [ ] Load bundled extension from `extensions/*`
- [ ] Load npm package plugin dynamically
- [ ] Config validation catches schema errors
- [ ] Plugin manifest validation validates tool/channel/hook metadata
- [ ] Hook boundary check: reject paths outside workspace

### Smoke Tests
- [ ] `pnpm test:install:smoke` (non-root docker install, CLI works)
- [ ] `npm pack` tarball includes `dist/*` and `extensions/*`, excludes app bundles
- [ ] `npx -y openclaw@latest --version` (downloads+runs, no ECOMPROMISED)

## Risk Register & Scope Cuts

### Risks

1. **Channel ecosystem breadth**
   - OpenClaw ships 8+ built-in + 5+ extension channels (Zalo, Matrix, BlueBubbles, etc.)
   - Full parity requires adapter + pairing + DM policy + group routing for each
   - Mitigation: Phase 1 = Telegram + Discord only; defer rest to phase 2

2. **Policy overlay complexity**
   - Global + agent + group + provider + subagent + channel level policies interact
   - Matrix of combinations can hide edge cases; unit tests alone miss operational bugs
   - Mitigation: live testing with real channels; acceptance tests document policy precedence

3. **Provider/model drift**
   - Real providers change tool-calling formats, rate limits, auth schemes
   - Unit-only testing passes despite hidden incompatibilities
   - Mitigation: narrowed live test matrix (Anthropic + OpenAI + Google) before first release

4. **Plugin provenance + safety**
   - Bundled vs npm vs workspace plugins have different trust boundaries
   - Malicious or broken plugin can corrupt runtime state
   - Mitigation: explicit allowlist checks, manifest validation, boundary file reads, error isolation

### Scope Cuts (if timeline/resources constrain)

**Scope Cut A: Channels only**
- Keep: Core, Telegram, Discord, plugin runtime
- Defer: WhatsApp, Slack, Signal, iMessage, BlueBubbles, Matrix, Zalo, etc.
- Still high-parity for plugin system + tool/chain architecture

**Scope Cut B: Advanced UX features**
- Keep: Config, sessions, auth
- Defer: Onboarding wizard, doctor CLI, Tailscale auto-expose, TLS, remote SSH gateway
- Still core feature-complete

**Scope Cut C: Optional plugins**
- Keep: Bundled core plugins (if any)
- Defer: Community/optional plugins (lobster, memory-lancedb, skills, etc.)
- Still extensible via plugin API

**Scope Cut D: App surfaces**
- Keep: CLI, Gateway, WebChat (if fast iterable)
- Defer: macOS app, iOS, Android nodes
- Still usable for operator workflows

## Phased Delivery & Handoff

### Phase 1: Control Plane (Weeks 1–2)
- [ ] Protocol + gateway scaffold
- [ ] CLI entry point + DI
- [ ] Config + session store
- [ ] Method dispatcher (chat, agent, send, status, models)
- **Agent to write:** Implementation of gateway server, WS protocol, basic routing

### Phase 2: Agent Runtime (Weeks 3–4)
- [ ] Model selection/catalog
- [ ] Tool registry + policy filter
- [ ] Streaming agent execution
- [ ] Basic tool set (read, write, exec, message send)
- **Agent to write:** Agent engine, tool system, model selection logic

### Phase 3: Channels (Weeks 5–6)
- [ ] Channel interface + normalization
- [ ] Telegram adapter
- [ ] Discord adapter
- [ ] Inbound→routing→outbound pipeline
- **Agent to write:** Channel adapters, routing, session derivation

### Phase 4: Plugin System (Weeks 7–8)
- [ ] Plugin loader + validator
- [ ] Plugin registry integration
- [ ] Config schema binding
- [ ] Hook/tool/channel plugin entry points
- **Agent to write:** Plugin loader, manifest validation, registry

### Phase 5: Hardening (Weeks 9–10)
- [ ] Auth rate limits
- [ ] Tool policy enforcement
- [ ] Secrets snapshot lifecycle
- [ ] Media TTL/token
- [ ] Pairing workflows
- **Agent to write:** Security/policy enforcement, edge case handling

### Phase 6: QA & Release (Weeks 11–12)
- [ ] Test matrix (unit, E2E, live channel narrow, live models narrow)
- [ ] Docker installer smoke test
- [ ] Release checklist validation
- [ ] Docs + examples
- **Agent to write:** Test framework, release automation, docs

## Prompt Pack for Coding Agents

### Agent 1: Gateway Protocol & Core Runtime
**Focus:** Typed WS protocol, method dispatch, runtime state, client/node pairing.

- Read: `docs/concepts/architecture.md`, `src/gateway/server.impl.ts`, `src/gateway/server-methods.ts`, `src/gateway/server-runtime-state.ts`
- Implement:
  - `GatewayServer` composition: boot, WS/HTTP bind, config load, plugin registry init, TLS/auth setup
  - WS handshake: device identity, signed challenge, token auth, pairing approval
  - Method router: request→handler dispatch, stream/event response, idempotency dedupe cache
  - Event bus: client subscriptions, server-push, presence/health versioning
  - Graceful close: drain active runs, alert clients, cleanup state
- Test: `vitest.gateway.config.ts` for handshake, dispatch, idempotency, multi-client

### Agent 2: Agent Engine & Tool System
**Focus:** Model selection, tool assembly, policy gates, streaming execution.

- Read: `src/agents/model-selection.ts`, `src/agents/model-catalog.ts`, `src/agents/pi-tools.ts`, `src/agents/pi-embedded-runner/compact.ts`, `src/gateway/server-methods/agent.ts`
- Implement:
  - Model resolver: provider/model ID normalization, allowlist filtering, fallback cascade
  - Tool registry: index core + plugin tools, apply policy filter (workspace/model/channel/provider/group bounds)
  - Agent runner: execute model with tools, stream blocks, handle tool calls, finalize summary
  - Policy evaluator: check tool is allowed in workspace/channel, model is in provider allowlist, group rules apply
  - Session context: load history, bind agent state, persist post-run
- Test: `vitest.unit.config.ts` for tool selection, policy logic, streaming blocks

### Agent 3: Channel Adapters & Routing
**Focus:** Telegram + Discord; inbound→routing→outbound; session key derivation.

- Read: `src/channels/registry.ts`, `src/channels/plugins/load.ts`, `src/routing/resolve-route.ts`, `src/infra/outbound/deliver.ts`, `extensions/telegram/`, `extensions/discord/`
- Implement:
  - Channel interface: connect, receive AsyncIterator, send, capabilities, pairing hooks
  - Telegram adapter: bot token, long-polling (or webhook), Update normalization, message send
  - Discord adapter: token, Socket Mode, Intents, message normalization, guild/DM routing
  - Routing: derive agent ID + session key from inbound peer/account/group metadata
  - Normalization: convert provider-specific format → canonical Message (content, peer, group, attachments)
  - Delivery: format response chunks, call adapter.send(), handle errors + retries
- Test: Unit message normalization, E2E adapter I/O, live Telegram/Discord narrow smoke

### Agent 4: Plugin System & Policy Enforcement
**Focus:** Plugin loader, config binding, manifest validation, hook/tool/channel registration, policy isolation.

- Read: `src/plugins/loader.ts`, `src/plugins/registry.ts`, `src/config/config.ts`, `src/config/io.ts`, `SECURITY.md`
- Implement:
  - Plugin manifest schema: metadata, register fn, config schema, capabilities list
  - Plugin loader: bundle discovery (extensions/*), npm packages, safe jiti import, error handling
  - Registry: active plugin state, tool/channel/hook/provider index, status + diagnostics
  - Config validation: JSON Schema, apply defaults, reject on schema mismatch + give errors
  - Hook boundary: allow workspace + extension paths only, reject parent dir escapes
  - Policy gates: tool execution gates (workspace, model, provider, channel, group), check before dispatch
- Test: Manifest validation, boundary checks, loading failure modes, config schema application

### Agent 5: Quality & Release
**Focus:** Test matrix, smoke suites, package validation, release checks, installer.

- Read: `docs/help/testing.md`, `docs/reference/RELEASING.md`, `scripts/test-parallel.mjs`, `scripts/test-install-sh-docker.sh`
- Implement:
  - Vitest configs: unit, E2E, live + narrow allowlist model/provider filtering
  - Docker installer smoke test: build image, run install.sh, verify CLI entrypoints
  - Release checklist automation: version bump, build, pack, validate tarball, smoke run
  - GH Actions CI/CD (if desired): lint, build, test gate before push, smoke on release tag
  - Docs: architecture overview, channel setup, plugin authoring, onboarding outline
- Test: Smoke suite passes, pack tarball has correct files, CLI runs via global install

## Next Steps

1. **Refine scope:** Lock in "phase 1" channel list (Telegram + Discord vs broader set?), defer/include plugin ecosystem, lock app surface list.
2. **Protocol spec:** Write formal request/response/event schemas in JSON Schema or TypeBox; version/evolve policy.
3. **Agent assignments:** Confirm 5-agent parallel execution model; assign owners to phases.
4. **CI/CD setup:** Bootstrap GitHub Actions or similar; integrate lint/build/test gates before merge.
5. **Documentation jigsaw:** Outline architecture docs, onboarding flow, plugin tutorial, security boundary guide.

---

**Notes**
- This plan prioritizes **control-plane correctness** (gateway + agent + plugin) over perfection of edge features. High parity is achievable; near-perfect parity (including all extension channels + policy matrix completeness) requires 2–3x the timeline.
- **Live tests are non-negotiable:** unit tests alone will miss operator-facing bugs in model/provider/channel integration. Budget 20% of QA time for narrowed live smoke runs.
- **Repo structure mirrors OpenClaw:**
  - `src/` — CLI, gateway, core runtime, channels, plugins, config, security
  - `extensions/` — bundled channel adapters, hooks, optional plugins
  - `packages/` — shared utilities, optional plugins (if any)
  - `docs/` — architecture, channel setup, plugin SDK, API reference
  - `test/`, `scripts/` — helpers, CI/CD, release automation
- **Release model:** Follow semantic versioning + GitHub tags + npm dist-tags (`latest` stable, `dev` main, `beta` prerelease). Smoke test every release before publishing.
