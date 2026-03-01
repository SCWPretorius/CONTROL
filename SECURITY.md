# SECURITY

This document describes the security boundaries implemented in CONTROL.

---

## WebSocket Gateway

### Handshake Rate Limiting
File: `src/gateway/rate-limiter.ts`

- Maximum **10 WebSocket connection attempts per minute per origin IP**
- Origin is resolved from `X-Forwarded-For` header (first entry) or socket remote address
- Connections exceeding the limit are rejected with close code `1008` (policy violation)
- Windows reset after 60 seconds; no persistent storage (in-memory only)

### Request Validation
File: `src/gateway/server.impl.ts`

- All WS messages must be valid JSON — parse errors return JSON-RPC `-32700`
- All requests must have a non-null `id` and a `method` field

---

## Tool / Skill Execution

### Policy Evaluation
File: `src/agents/policy-evaluator.ts`

- **Source policy**: only known sources (`gateway`, `telegram`, `discord`, etc.) may submit events
- **Tool policy**: every tool call is checked against `src/permissions/rbac.ts` before execution
  - Role hierarchy: `guest < user < admin`
  - Per-skill minimum role (e.g., `deleteCalendarEvent` requires `admin`)
  - Per-integration source role requirement
  - Daily per-user per-skill usage limits
- **Workspace boundary**: file path operations must resolve within `process.cwd()` (uses `path.sep` for cross-platform correctness)
- **Model policy**: optional provider allowlist checked before model selection

### Usage Recording
File: `src/agents/agent-runner.ts`

- `recordUsage(skill, event)` is called after every **successful** tool execution
- Failures do not count against daily limits

---

## Secrets Lifecycle

### Encrypted at Rest
File: `src/config/secrets.ts`

- Secrets stored in `data/secrets.enc` using AES-256-GCM
- `ENCRYPTION_KEY` must be at least 32 characters
- All secret access is audit-logged to `logs/secret-access.jsonl`

### Use-and-Clear Pattern
File: `src/secrets/runtime.ts`

- `useSecret(name, fn)` — retrieves a secret, passes it to `fn`, then lets it go out of scope
- `getSecretBuffer(name)` — returns a `Buffer` that the caller can zero with `buf.fill(0)` after use
- Prefer these over calling `getSecret()` directly and storing the result in a long-lived variable

---

## Media Tokens

### HMAC-Signed TTL Tokens
File: `src/media/token.ts`

- Media paths are gated behind HMAC-SHA256 signed tokens
- Tokens encode: `base64url(mediaPath:expiry:hmac)`
- **TTL: 1 hour** from generation
- Signatures use `timingSafeEqual` to prevent timing oracle attacks
- Signing key derived from `ENCRYPTION_KEY`; falls back to a non-secret default (logs a warning)

### HTTP Route
File: `src/server/server.ts`

- `GET /media/:token` — verifies the token; returns `{ mediaPath }` on success or `401` on failure

---

## Device Pairing

### Challenge/Response
File: `src/gateway/device-pairing.ts`

- Devices prove identity by computing `HMAC-SHA256(nonce, secret)` where `nonce` is issued by the server
- Challenges expire after **5 minutes**
- Approved devices are persisted to `data/devices.json` and survive restarts
- `isApproved(deviceId)` is available for future auth gating on RPC methods

---

## Configuration

### Immutable Snapshot
File: `src/config/io.ts`

- `validateConfig()` calls `Object.freeze(config)` at startup
- Prevents runtime mutation of config values (defense-in-depth alongside TypeScript `as const`)
- Logs warnings for: missing `LLM_PRIMARY_MODEL`, missing integration tokens, missing `ENCRYPTION_KEY`

### Deprecated Env Var Migration
File: `src/config/migration.ts`

- `migrateConfig()` logs warnings when deprecated env vars are detected (e.g., `OLLAMA_URL` → `LMSTUDIO_URL`)
- Does not silently accept deprecated values without warning

---

## Plugin System

### Workspace Boundary
File: `src/plugins/loader.ts`

- Plugin entry point paths are resolved with `path.resolve()` and must start with `process.cwd() + path.sep`
- Absolute paths or `../` traversal outside the workspace root are rejected
- Applies to both bundled (`extensions/`) and future npm-loaded plugins

---

## Deferred / Known Gaps

| Gap | File | Phase |
|-----|------|-------|
| `channels.send` has no auth gate — any WS client can send to any chat ID | `server-methods.ts` | Future |
| Webhook body HMAC uses `JSON.stringify(req.body)` which is key-order/whitespace sensitive | `server/server.ts` | Future |
| WS message auth (device approval not yet enforced per-request) | `server.impl.ts` | Future |
| Rate limiter state is in-memory (resets on restart) | `gateway/rate-limiter.ts` | Future |
