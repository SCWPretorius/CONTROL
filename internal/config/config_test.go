package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoadFromLookupDefaultsAndNormalizesPaths(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "-100123",
			"COPILOT_CLI_PATH":         "copilot",
		}),
		func() (string, error) { return cwd, nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if cfg.Copilot.Transport() != "stdio" {
		t.Fatalf("Transport() = %q, want stdio", cfg.Copilot.Transport())
	}

	if cfg.Session.Model != defaultModel {
		t.Fatalf("Model = %q, want %q", cfg.Session.Model, defaultModel)
	}

	if got, want := cfg.Paths.RuntimeDir, filepath.Join(cwd, "var", "runtime"); got != want {
		t.Fatalf("RuntimeDir = %q, want %q", got, want)
	}

	if got, want := cfg.Session.ConfigDir, filepath.Join(cwd, "var", "runtime", "copilot"); got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}

	if got, want := cfg.Tools.Privileged.AllowedWorkspaceRoots, []string{cwd}; !slices.Equal(got, want) {
		t.Fatalf("AllowedWorkspaceRoots = %#v, want %#v", got, want)
	}

	if got, want := cfg.Tools.Privileged.AssistantWritableRoots, []string{
		filepath.Join(cwd, "var", "runtime"),
		filepath.Join(cwd, "var", "storage"),
	}; !slices.Equal(got, want) {
		t.Fatalf("AssistantWritableRoots = %#v, want %#v", got, want)
	}

	if got, want := cfg.Tools.Google.OAuth.Scopes, defaultGoogleOAuthScopes; !slices.Equal(got, want) {
		t.Fatalf("Scopes = %#v, want %#v", got, want)
	}
	if cfg.Tools.Google.AccessToken != "" {
		t.Fatalf("AccessToken = %q, want empty by default", cfg.Tools.Google.AccessToken)
	}

	if got, want := cfg.Tools.Runtime.HTTPTimeout, defaultToolHTTPTimeout; got != want {
		t.Fatalf("HTTPTimeout = %s, want %s", got, want)
	}

	if got, want := cfg.Tools.Runtime.ShellCommandTimeout, defaultToolShellTimeout; got != want {
		t.Fatalf("ShellCommandTimeout = %s, want %s", got, want)
	}

	if got, want := cfg.Tools.Runtime.MaxCommandOutputBytes, defaultToolMaxOutputBytes; got != want {
		t.Fatalf("MaxCommandOutputBytes = %d, want %d", got, want)
	}
}

func TestLoadFromLookupLoadsToolingSettings(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	workspaceOne := filepath.Join(cwd, "workspace-one")
	workspaceTwo := filepath.Join(cwd, "workspace-two")
	writable := filepath.Join(cwd, "assistant-data")
	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":                     "token",
			"TELEGRAM_ALLOWED_USER_ID":               "1",
			"TELEGRAM_ALLOWED_CHAT_ID":               "2",
			"COPILOT_CLI_PATH":                       "copilot",
			"ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS": strings.Join([]string{"workspace-one", workspaceTwo, "workspace-one"}, string(filepath.ListSeparator)),
			"ASSISTANT_TOOL_WRITABLE_ROOTS":          strings.Join([]string{"assistant-data", writable}, string(filepath.ListSeparator)),
			"ASSISTANT_TOOL_SHELL_AUTO_APPROVE":      "git status --short,go test ./internal/config",
			"ASSISTANT_TOOL_HTTP_TIMEOUT":            "45s",
			"ASSISTANT_TOOL_SHELL_TIMEOUT":           "2m",
			"ASSISTANT_TOOL_MAX_OUTPUT_BYTES":        "2048",
			"GOOGLE_OAUTH_CLIENT_ID":                 "client-id",
			"GOOGLE_OAUTH_CLIENT_SECRET":             "client-secret",
			"GOOGLE_OAUTH_REDIRECT_URL":              "http://127.0.0.1:8787/oauth/callback",
			"GOOGLE_OAUTH_ACCESS_TOKEN":              "access-token",
			"GOOGLE_OAUTH_SCOPES":                    "scope-a, scope-b",
		}),
		func() (string, error) { return cwd, nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if got, want := cfg.Tools.Privileged.AllowedWorkspaceRoots, []string{workspaceOne, workspaceTwo}; !slices.Equal(got, want) {
		t.Fatalf("AllowedWorkspaceRoots = %#v, want %#v", got, want)
	}

	if got, want := cfg.Tools.Privileged.AssistantWritableRoots, []string{writable}; !slices.Equal(got, want) {
		t.Fatalf("AssistantWritableRoots = %#v, want %#v", got, want)
	}

	if got, want := cfg.Tools.Privileged.ShellAutoApprove, []string{"git status --short", "go test ./internal/config"}; !slices.Equal(got, want) {
		t.Fatalf("ShellAutoApprove = %#v, want %#v", got, want)
	}

	if !cfg.Tools.Google.OAuth.Enabled() {
		t.Fatal("Google OAuth should be enabled when credentials are set")
	}
	if got, want := cfg.Tools.Google.AccessToken, "access-token"; got != want {
		t.Fatalf("AccessToken = %q, want %q", got, want)
	}
	if !cfg.Tools.Google.RuntimeEnabled() {
		t.Fatal("Google tool runtime should be enabled when OAuth config and access token are set")
	}
	if cfg.Tools.MCP.Enabled() {
		t.Fatal("MCP should be disabled by default")
	}

	if got, want := cfg.Tools.Google.OAuth.Scopes, []string{"scope-a", "scope-b"}; !slices.Equal(got, want) {
		t.Fatalf("Scopes = %#v, want %#v", got, want)
	}

	if got, want := cfg.Tools.Runtime.HTTPTimeout, 45*time.Second; got != want {
		t.Fatalf("HTTPTimeout = %s, want %s", got, want)
	}

	if got, want := cfg.Tools.Runtime.ShellCommandTimeout, 2*time.Minute; got != want {
		t.Fatalf("ShellCommandTimeout = %s, want %s", got, want)
	}

	if got, want := cfg.Tools.Runtime.MaxCommandOutputBytes, 2048; got != want {
		t.Fatalf("MaxCommandOutputBytes = %d, want %d", got, want)
	}
}

func TestLoadFromLookupLoadsAndNormalizesMCPServers(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
			"ASSISTANT_TOOL_MCP_SERVERS_JSON": `{
				"filesystem": {
					"type": "stdio",
					"command": "npx",
					"args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
					"tools": ["*"],
					"env": {"MODE": " readonly "},
					"cwd": ".\\tools"
				},
				"tickets": {
					"type": "http",
					"url": "https://example.com/mcp",
					"tools": ["list_tickets"],
					"headers": {"Authorization": " Bearer token "},
					"timeout": 15
				}
			}`,
		}),
		func() (string, error) { return cwd, nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if !cfg.Tools.MCP.Enabled() {
		t.Fatal("MCP should be enabled when servers are configured")
	}

	filesystem := cfg.Tools.MCP.Servers["filesystem"]
	if filesystem.Type != "stdio" {
		t.Fatalf("filesystem.Type = %q, want stdio", filesystem.Type)
	}
	if got, want := filesystem.Cwd, filepath.Join(cwd, "tools"); got != want {
		t.Fatalf("filesystem.Cwd = %q, want %q", got, want)
	}
	if got, want := filesystem.Env["MODE"], "readonly"; got != want {
		t.Fatalf("filesystem.Env[MODE] = %q, want %q", got, want)
	}

	tickets := cfg.Tools.MCP.Servers["tickets"]
	if tickets.Type != "http" {
		t.Fatalf("tickets.Type = %q, want http", tickets.Type)
	}
	if got, want := tickets.Headers["Authorization"], "Bearer token"; got != want {
		t.Fatalf("tickets.Headers[Authorization] = %q, want %q", got, want)
	}
	if got, want := tickets.Timeout, 15; got != want {
		t.Fatalf("tickets.Timeout = %d, want %d", got, want)
	}
}

func TestLoadFromLookupRejectsConflictingCopilotEndpoints(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
			"COPILOT_CLI_URL":          "http://127.0.0.1:4141",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want conflict error")
	}
}

func TestLoadFromLookupRequiresTelegramValues(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"COPILOT_CLI_PATH": "copilot",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want missing env error")
	}
}

func TestLoadFromLookupRequiresTelegramBotToken(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want missing bot token error")
	}
}

func TestLoadFromLookupRejectsPartialGoogleOAuthConfig(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
			"GOOGLE_OAUTH_CLIENT_ID":   "client-id",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want partial Google OAuth error")
	}
}

func TestLoadFromLookupRejectsAccessTokenWithoutOAuthConfig(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":        "token",
			"TELEGRAM_ALLOWED_USER_ID":  "1",
			"TELEGRAM_ALLOWED_CHAT_ID":  "2",
			"COPILOT_CLI_PATH":          "copilot",
			"GOOGLE_OAUTH_ACCESS_TOKEN": "access-token",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want access-token validation error")
	}
}

func TestLoadFromLookupRejectsUnsafeShellAutoApproveEntry(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{
		"git status && git diff",
		"git status & whoami",
	} {
		entry := entry
		t.Run(entry, func(t *testing.T) {
			t.Parallel()

			_, err := LoadFromLookup(
				envLookup(map[string]string{
					"TELEGRAM_BOT_TOKEN":                "token",
					"TELEGRAM_ALLOWED_USER_ID":          "1",
					"TELEGRAM_ALLOWED_CHAT_ID":          "2",
					"COPILOT_CLI_PATH":                  "copilot",
					"ASSISTANT_TOOL_SHELL_AUTO_APPROVE": entry,
				}),
				func() (string, error) { return t.TempDir(), nil },
			)
			if err == nil {
				t.Fatalf("LoadFromLookup() error = nil, want unsafe shell allowlist error for %q", entry)
			}
		})
	}
}

func TestLoadFromLookupRejectsInvalidMCPConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"invalid json":        `{`,
		"missing tools":       `{"bad":{"type":"stdio","command":"npx"}}`,
		"missing local cmd":   `{"bad":{"type":"stdio","tools":["*"]}}`,
		"local with url":      `{"bad":{"type":"local","command":"npx","url":"https://example.com","tools":["*"]}}`,
		"missing remote url":  `{"bad":{"type":"http","tools":["*"]}}`,
		"remote local fields": `{"bad":{"type":"sse","url":"https://example.com/sse","command":"npx","tools":["*"]}}`,
		"unsupported type":    `{"bad":{"type":"socket","tools":["*"]}}`,
		"bad url":             `{"bad":{"type":"http","url":"not-a-url","tools":["*"]}}`,
		"bad url scheme":      `{"bad":{"type":"http","url":"ftp://example.com/mcp","tools":["*"]}}`,
		"unknown field":       `{"bad":{"type":"http","url":"https://example.com/mcp","tools":["*"],"unknown":true}}`,
	}

	for name, raw := range cases {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadFromLookup(
				envLookup(map[string]string{
					"TELEGRAM_BOT_TOKEN":              "token",
					"TELEGRAM_ALLOWED_USER_ID":        "1",
					"TELEGRAM_ALLOWED_CHAT_ID":        "2",
					"COPILOT_CLI_PATH":                "copilot",
					"ASSISTANT_TOOL_MCP_SERVERS_JSON": raw,
				}),
				func() (string, error) { return t.TempDir(), nil },
			)
			if err == nil {
				t.Fatalf("LoadFromLookup() error = nil, want MCP validation error for %q", name)
			}
		})
	}
}

func TestLoadFromLookupRejectsInvalidToolRuntimeKnobs(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":              "token",
			"TELEGRAM_ALLOWED_USER_ID":        "1",
			"TELEGRAM_ALLOWED_CHAT_ID":        "2",
			"COPILOT_CLI_PATH":                "copilot",
			"ASSISTANT_TOOL_HTTP_TIMEOUT":     "not-a-duration",
			"ASSISTANT_TOOL_MAX_OUTPUT_BYTES": "0",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want invalid runtime knob error")
	}
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
