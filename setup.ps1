# CONTROL — local development setup
# Fill in the required values, then run: .\setup.ps1

# ── Required ─────────────────────────────────────────────────────────────────
# Get a bot token from @BotFather on Telegram (send /newbot).
$env:TELEGRAM_BOT_TOKEN = ""

# Your Telegram numeric user ID — forward a message to @userinfobot to find it.
$env:TELEGRAM_ALLOWED_USER_ID = ""

# The chat ID to allow. For a private chat with the bot this is the same as
# your user ID. For a group, send a message and check the Telegram API:
#   https://api.telegram.org/bot<token>/getUpdates
$env:TELEGRAM_ALLOWED_CHAT_ID = ""

# Path to the Copilot CLI executable already on your PATH.
$env:COPILOT_CLI_PATH = "copilot"

# ── Optional: Copilot model ───────────────────────────────────────────────────
# Uncomment and change to override the default model.
# $env:COPILOT_MODEL            = "Claude Haiku 4.5"
# $env:COPILOT_REASONING_EFFORT = "medium"   # low | medium | high

# ── Optional: Google Workspace tools ─────────────────────────────────────────
# Leave all three OAuth variables unset (or empty) to skip Gmail + Calendar.
# Set all three together with a valid access token to enable them.
$env:GOOGLE_OAUTH_CLIENT_ID     = ""
$env:GOOGLE_OAUTH_CLIENT_SECRET = ""
$env:GOOGLE_OAUTH_REDIRECT_URL  = ""
$env:GOOGLE_OAUTH_ACCESS_TOKEN =  ""

# ── Optional: privileged tool constraints ────────────────────────────────────
# Defaults to the current working directory; extend as needed.
# $env:ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS = "$PWD"
# $env:ASSISTANT_TOOL_WRITABLE_ROOTS = "$PWD\var\runtime;$PWD\var\storage"

# Comma-separated exact command prefixes that skip interactive shell approval.
# Keep this intentionally small.
# $env:ASSISTANT_TOOL_SHELL_AUTO_APPROVE = "git status --short"

# ── Run ───────────────────────────────────────────────────────────────────────
go run ./cmd/assistant
