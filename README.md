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
| `ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS` | current working directory |
| `ASSISTANT_TOOL_WRITABLE_ROOTS` | `<runtime dir>` and `<storage dir>` |
| `ASSISTANT_TOOL_SHELL_AUTO_APPROVE` | empty |
| `ASSISTANT_TOOL_HTTP_TIMEOUT` | `30s` |
| `ASSISTANT_TOOL_SHELL_TIMEOUT` | `30s` |
| `ASSISTANT_TOOL_MAX_OUTPUT_BYTES` | `65536` |

## Shared tooling configuration

The tool layer uses shared config from `internal/config` so privileged capabilities stay env-driven and narrow by default:

- `GOOGLE_OAUTH_*` is the single shared OAuth client config for Gmail + Calendar integrations.
- `GOOGLE_OAUTH_ACCESS_TOKEN` is the v1 runtime bearer token for Google Workspace tools. Leave it unset to keep Google tools disabled while still building and testing locally.
- `ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS` is the allowlist of workspace roots tools may read from. Use the OS path-list separator (`;` on Windows, `:` on Unix-like systems).
- `ASSISTANT_TOOL_WRITABLE_ROOTS` is the narrower set of assistant-owned roots tools may write to. Use the same OS path-list separator.
- `ASSISTANT_TOOL_SHELL_AUTO_APPROVE` is a comma-separated allowlist of exact command prefixes that can be auto-approved without interactive review. Keep it intentionally small and do not include shell chaining/operators.
- `ASSISTANT_TOOL_HTTP_TIMEOUT`, `ASSISTANT_TOOL_SHELL_TIMEOUT`, and `ASSISTANT_TOOL_MAX_OUTPUT_BYTES` are shared runtime knobs for tool execution and Google API calls.

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
$env:ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS = "C:\Users\you\Documents\Repos\CONTROL"
$env:ASSISTANT_TOOL_WRITABLE_ROOTS = "C:\Users\you\Documents\Repos\CONTROL\var\runtime;C:\Users\you\Documents\Repos\CONTROL\var\storage"
$env:ASSISTANT_TOOL_SHELL_AUTO_APPROVE = "git status --short,go test ./internal/config"
go run ./cmd/assistant
```

Expected result with valid runtime credentials:

- configuration loads successfully
- runtime and storage directories are created if missing
- Telegram long polling starts successfully for the configured bot
- Copilot sessions are created and resumed per authorized Telegram chat
- privileged tools are always registered behind the local policy layer
- Google Workspace tools are registered only when both OAuth app config and `GOOGLE_OAUTH_ACCESS_TOKEN` are set
- the process logs startup/runtime activity and continues serving until stopped

## Verification

```powershell
go test ./...
go build ./...
# requires valid Telegram + Copilot runtime configuration
go run ./cmd/assistant
```
