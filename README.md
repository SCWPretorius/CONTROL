# CONTROL

Go Telegram personal assistant backed by the GitHub Copilot SDK.

The current implementation includes:

- Telegram long polling with single-user and single-chat access control
- Copilot session creation/resume over the official Go SDK runtime boundary
- local-file session persistence and privileged tool audit logs
- Telegram admin commands for runtime/session inspection and reset
- v1 tool registration for Gmail, Google Calendar, local file write, and local shell

Google Workspace tools are enabled only when the OAuth client config and a runtime access token are present. Privileged local tools are always registered, but remain constrained by the local policy layer.

## Package layout

```text
cmd/assistant         local development entrypoint
internal/app          service wiring and startup
internal/config       env loading and validation
internal/copilot      Copilot SDK runtime boundary
internal/router       Telegram message orchestration and admin commands
internal/store        local persistence contracts and implementations
internal/telegram     long-polling transport and access control
internal/tools        Google Workspace and privileged local tools
```

## Required environment variables

| Variable | Purpose |
| --- | --- |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token for the assistant runtime. |
| `TELEGRAM_ALLOWED_USER_ID` | Single allowed Telegram user ID for v1 access control. |
| `TELEGRAM_ALLOWED_CHAT_ID` | Single allowed Telegram chat ID for v1 access control. |
| `COPILOT_CLI_PATH` or `COPILOT_CLI_URL` | Either a local Copilot CLI executable path or a remote runtime URL. Set exactly one. |

## Optional environment variables

| Variable | Default |
| --- | --- |
| `ASSISTANT_RUNTIME_DIR` | `var/runtime` |
| `ASSISTANT_STORAGE_DIR` | `var/storage` |
| `COPILOT_MODEL` | `gpt-5.4` |
| `COPILOT_REASONING_EFFORT` | `medium` |
| `COPILOT_SESSION_NAMESPACE` | `telegram-personal-assistant` |
| `COPILOT_RESUME_SESSIONS` | `true` |
| `COPILOT_WORKING_DIR` | current working directory |
| `COPILOT_CONFIG_DIR` | `<runtime dir>/copilot` |
| `GOOGLE_OAUTH_CLIENT_ID` | empty; must be set together with `GOOGLE_OAUTH_CLIENT_SECRET` and `GOOGLE_OAUTH_REDIRECT_URL` |
| `GOOGLE_OAUTH_CLIENT_SECRET` | empty; must be set together with `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_REDIRECT_URL` |
| `GOOGLE_OAUTH_REDIRECT_URL` | empty; must be set together with `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` |
| `GOOGLE_OAUTH_SCOPES` | `https://www.googleapis.com/auth/gmail.modify,https://www.googleapis.com/auth/calendar,https://www.googleapis.com/auth/userinfo.email` |
| `GOOGLE_OAUTH_ACCESS_TOKEN` | empty; when set alongside the Google OAuth app config, enables the Gmail + Calendar Copilot tools |
| `ASSISTANT_TOOL_MCP_SERVERS_JSON` | empty; optional JSON object of named MCP servers wired into every Copilot session |
| `ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR` | empty; when set together with `ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN`, enables the local runtime MCP registration endpoint on `127.0.0.1` |
| `ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN` | empty; required bearer token for the local runtime MCP registration endpoint |
| `ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS` | current working directory |
| `ASSISTANT_TOOL_WRITABLE_ROOTS` | `<runtime dir>` and `<storage dir>` |
| `ASSISTANT_TOOL_SHELL_AUTO_APPROVE` | empty |
| `ASSISTANT_TOOL_HTTP_TIMEOUT` | `30s` |
| `ASSISTANT_TOOL_SHELL_TIMEOUT` | `30s` |
| `ASSISTANT_TOOL_MAX_OUTPUT_BYTES` | `65536` |
| `ASSISTANT_MONITOR_ENABLED` | `false` |
| `ASSISTANT_MONITOR_MODE` | `notify_only` |
| `ASSISTANT_MONITOR_INTERVAL` | `1m` |
| `ASSISTANT_MONITOR_JITTER` | `10s` |
| `ASSISTANT_MONITOR_TIMEOUT` | `10s` |
| `ASSISTANT_MONITOR_COOLDOWN` | `15m` |
| `ASSISTANT_MONITOR_HTTP_CHECKS_JSON` | empty; JSON array of HTTP checks required when monitoring is enabled |

## Shared tooling configuration

The tool layer uses shared config from `internal/config` so privileged capabilities stay env-driven and narrow by default:

- `GOOGLE_OAUTH_*` is the single shared OAuth client config for Gmail + Calendar integrations.
- `GOOGLE_OAUTH_ACCESS_TOKEN` is the v1 runtime bearer token for Google Workspace tools. Leave it unset to keep Google tools disabled while still building and testing locally.
- `ASSISTANT_TOOL_MCP_SERVERS_JSON` is an optional JSON object keyed by MCP server name. Supported `type` values are `local`/`stdio` for subprocess servers and `http`/`sse` for remote servers. Every server must declare at least one `tools` selector.
- `ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR` and `ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN` enable a loopback-only admin endpoint for registering MCP servers at runtime. Both must be set together, and the listener must bind to `127.0.0.1`.
- `ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS` is the allowlist of workspace roots tools may read from. Use the OS path-list separator (`;` on Windows, `:` on Unix-like systems).
- `ASSISTANT_TOOL_WRITABLE_ROOTS` is the narrower set of assistant-owned roots tools may write to. Use the same OS path-list separator.
- `ASSISTANT_TOOL_SHELL_AUTO_APPROVE` is a comma-separated allowlist of exact command prefixes that can be auto-approved without interactive review. Keep it intentionally small and do not include shell chaining/operators.
- `ASSISTANT_TOOL_HTTP_TIMEOUT`, `ASSISTANT_TOOL_SHELL_TIMEOUT`, and `ASSISTANT_TOOL_MAX_OUTPUT_BYTES` are shared runtime knobs for tool execution and Google API calls.

MCP notes:

- `ASSISTANT_TOOL_MCP_SERVERS_JSON` still seeds startup MCP servers for the initial runtime snapshot.
- When the local admin endpoint is enabled, operators can register or replace MCP servers at runtime without restarting CONTROL.
- Newly registered MCP servers apply to future session create/resume operations only; existing in-memory sessions keep their current MCP snapshot until they are reset or reloaded.
- CONTROL still does not allow Telegram users to add or mutate MCP servers at runtime.
- MCP permission requests remain deny-by-default unless a future policy explicitly approves them.
- Keep MCP secrets out of git history. Put bearer tokens in local-only environment values and use placeholders in committed examples.

Home Assistant example:

```powershell
$env:ASSISTANT_TOOL_MCP_SERVERS_JSON = '{"Home Assistant":{"type":"stdio","command":"mcp-proxy","args":["--transport=streamablehttp","--stateless","http://localhost:8123/api/mcp"],"env":{"API_ACCESS_TOKEN":"replace-with-your-home-assistant-token"},"tools":["*"]}}'
```

## Local setup

PowerShell example:

```powershell
$env:TELEGRAM_BOT_TOKEN = "replace-me"
$env:TELEGRAM_ALLOWED_USER_ID = "123456789"
$env:TELEGRAM_ALLOWED_CHAT_ID = "123456789"
$env:COPILOT_CLI_PATH = "copilot"
$env:GOOGLE_OAUTH_CLIENT_ID = "replace-me.apps.googleusercontent.com"
$env:GOOGLE_OAUTH_CLIENT_SECRET = "replace-me"
$env:GOOGLE_OAUTH_REDIRECT_URL = "http://127.0.0.1:8787/oauth/callback"
$env:GOOGLE_OAUTH_ACCESS_TOKEN = "replace-with-a-valid-user-access-token"
$env:ASSISTANT_TOOL_MCP_SERVERS_JSON = '{"filesystem":{"type":"stdio","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","C:\\Users\\you\\Documents\\Repos\\CONTROL"],"tools":["*"]},"issues":{"type":"http","url":"https://example.com/mcp","headers":{"Authorization":"Bearer replace-me"},"tools":["list_issues"]}}'
$env:ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR = "127.0.0.1:8788"
$env:ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN = "replace-with-a-long-random-token"
$env:ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS = "C:\Users\you\Documents\Repos\CONTROL"
$env:ASSISTANT_TOOL_WRITABLE_ROOTS = "C:\Users\you\Documents\Repos\CONTROL\var\runtime;C:\Users\you\Documents\Repos\CONTROL\var\storage"
$env:ASSISTANT_TOOL_SHELL_AUTO_APPROVE = "git status --short,go test ./internal/config"
$env:ASSISTANT_MONITOR_ENABLED = "true"
$env:ASSISTANT_MONITOR_MODE = "notify_only"
$env:ASSISTANT_MONITOR_HTTP_CHECKS_JSON = '[{"id":"api-health","url":"https://example.com/health","method":"GET","headers":{"Authorization":"Bearer replace-me"},"expected_status_codes":[200]}]'
go run ./cmd/assistant
```

Expected result with valid runtime credentials:

- configuration loads successfully
- runtime and storage directories are created if missing
- Telegram long polling starts successfully for the configured bot
- Copilot sessions are created and resumed per authorized Telegram chat
- privileged tools are always registered behind the local policy layer
- Google Workspace tools are registered only when both OAuth app config and `GOOGLE_OAUTH_ACCESS_TOKEN` are set
- configured MCP servers are attached to every Copilot session create/resume request
- when the local MCP admin endpoint is enabled, operators can register additional MCP servers for future sessions via `POST /admin/mcp/servers`
- when monitoring is enabled, configured HTTP checks run on their own schedule and send direct Telegram alerts to the allowed chat on unhealthy conditions
- the process logs startup/runtime activity and continues serving until stopped

Runtime MCP registration example:

```powershell
$headers = @{ Authorization = "Bearer replace-with-a-long-random-token" }
$body = @{
  name = "issues"
  type = "http"
  url = "https://example.com/mcp"
  tools = @("list_issues")
  headers = @{ Authorization = "Bearer replace-me" }
} | ConvertTo-Json

Invoke-RestMethod -Method Post `
  -Uri "http://127.0.0.1:8788/admin/mcp/servers" `
  -Headers $headers `
  -ContentType "application/json" `
  -Body $body
```

Runtime MCP registration behavior:

- Send `POST /admin/mcp/servers` to the loopback admin listener with `Authorization: Bearer <token>`.
- The JSON body must include `name`, `type`, and at least one `tools` selector, plus the same transport-specific fields accepted by `ASSISTANT_TOOL_MCP_SERVERS_JSON`.
- Registering a new server name returns `201 Created`; posting an existing name replaces that server definition and returns `200 OK`.
- Invalid payloads or failed validation are rejected with `400 Bad Request`; missing or incorrect bearer tokens are rejected with `401 Unauthorized`.
- New registrations apply to future Copilot session create/resume requests only. If an existing Telegram chat should pick up the new MCP server immediately, run `/reset` for that chat before the next prompt.

## Monitoring (first slice)

The first monitoring slice is a **notify-only** HTTP checker. It polls configured HTTP endpoints on a timer, persists per-check checkpoint state under `<storage dir>/monitors`, and sends direct outbound Telegram alerts to `TELEGRAM_ALLOWED_CHAT_ID` when a check enters or remains in an unhealthy condition after cooldown.

Current slice behavior:

- `ASSISTANT_MONITOR_MODE=notify_only` is the only implemented runtime mode. Other mode names are reserved in config validation but are not wired into the monitor runner yet.
- Checks come from `ASSISTANT_MONITOR_HTTP_CHECKS_JSON`, a JSON array. Each check requires an `id` and absolute `url`; `method` defaults to `GET`; `headers` are optional; `expected_status_codes` defaults to `[200]`.
- Each alert includes the check ID, monitor mode, HTTP method, URL, detected condition, detail text, and UTC detection timestamp.
- Alerts are sent directly through Telegram to the configured allowed chat. They do not depend on an inbound Telegram message or an active Copilot session.
- Cooldown/dedupe is checkpoint-driven: repeated identical failures are suppressed until the cooldown expires, while a changed condition fingerprint alerts immediately and a healthy result clears the stored unhealthy fingerprint/cooldown state.

HTTP check JSON example:

```json
[
  {
    "id": "api-health",
    "url": "https://example.com/health",
    "method": "GET",
    "headers": {
      "Authorization": "Bearer replace-me"
    },
    "expected_status_codes": [200]
  }
]
```

Explicitly not implemented yet:

- `analyze_then_notify` monitor actions
- any automatic remediation or auto-fix path
- non-HTTP monitor sources such as log/file, process/resource, or external API checks beyond direct HTTP requests
- delivery retries, escalation policies beyond cooldown, or monitor health telemetry

## Verification

```powershell
go test ./...
go build ./...
# requires valid Telegram + Copilot runtime configuration
go run ./cmd/assistant
```
