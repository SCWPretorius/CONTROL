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
	if cfg.Session.Provider != nil {
		t.Fatalf("Provider = %#v, want nil by default", cfg.Session.Provider)
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

	if cfg.Monitor.Enabled {
		t.Fatal("Monitor should be disabled by default")
	}
	if got, want := cfg.Monitor.Mode, defaultMonitorMode; got != want {
		t.Fatalf("Monitor.Mode = %q, want %q", got, want)
	}
	if got, want := cfg.Monitor.Interval, defaultMonitorInterval; got != want {
		t.Fatalf("Monitor.Interval = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Jitter, defaultMonitorJitter; got != want {
		t.Fatalf("Monitor.Jitter = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Timeout, defaultMonitorTimeout; got != want {
		t.Fatalf("Monitor.Timeout = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Cooldown, defaultMonitorCooldown; got != want {
		t.Fatalf("Monitor.Cooldown = %s, want %s", got, want)
	}
	if len(cfg.Monitor.HTTPChecks) != 0 {
		t.Fatalf("Monitor.HTTPChecks len = %d, want 0", len(cfg.Monitor.HTTPChecks))
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

func TestLoadFromLookupLoadsMCPAdminConfig(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":                    "token",
			"TELEGRAM_ALLOWED_USER_ID":              "1",
			"TELEGRAM_ALLOWED_CHAT_ID":              "2",
			"COPILOT_CLI_PATH":                      "copilot",
			"ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR":  "127.0.0.1:8788",
			"ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN": " secret-token ",
		}),
		func() (string, error) { return cwd, nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if !cfg.Tools.MCP.Admin.Enabled() {
		t.Fatal("MCP admin should be enabled when listen addr and token are set")
	}
	if got, want := cfg.Tools.MCP.Admin.ListenAddress, "127.0.0.1:8788"; got != want {
		t.Fatalf("ListenAddress = %q, want %q", got, want)
	}
	if got, want := cfg.Tools.MCP.Admin.BearerToken, "secret-token"; got != want {
		t.Fatalf("BearerToken = %q, want %q", got, want)
	}
}

func TestLoadFromLookupRejectsPartialMCPAdminConfig(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":                   "token",
			"TELEGRAM_ALLOWED_USER_ID":             "1",
			"TELEGRAM_ALLOWED_CHAT_ID":             "2",
			"COPILOT_CLI_PATH":                     "copilot",
			"ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR": "127.0.0.1:8788",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want MCP admin config error")
	}
}

func TestLoadFromLookupRejectsNonLoopbackMCPAdminAddress(t *testing.T) {
	t.Parallel()

	_, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":                    "token",
			"TELEGRAM_ALLOWED_USER_ID":              "1",
			"TELEGRAM_ALLOWED_CHAT_ID":              "2",
			"COPILOT_CLI_PATH":                      "copilot",
			"ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR":  "0.0.0.0:8788",
			"ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN": "secret-token",
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err == nil {
		t.Fatal("LoadFromLookup() error = nil, want loopback-only MCP admin error")
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

func TestLoadFromLookupLoadsCopilotProviderConfig(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
			"COPILOT_PROVIDER_JSON":    `{"baseUrl":" http://127.0.0.1:11434/v1 ","apiKey":" test-key "}`,
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if cfg.Session.Provider == nil {
		t.Fatal("Provider = nil, want configured provider")
	}
	if got, want := cfg.Session.Provider.Type, defaultProviderType; got != want {
		t.Fatalf("Provider.Type = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.WireAPI, defaultProviderWireAPI; got != want {
		t.Fatalf("Provider.WireAPI = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.BaseURL, "http://127.0.0.1:11434/v1"; got != want {
		t.Fatalf("Provider.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.APIKey, "test-key"; got != want {
		t.Fatalf("Provider.APIKey = %q, want %q", got, want)
	}
}

func TestLoadFromLookupLoadsAzureCopilotProviderConfig(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":       "token",
			"TELEGRAM_ALLOWED_USER_ID": "1",
			"TELEGRAM_ALLOWED_CHAT_ID": "2",
			"COPILOT_CLI_PATH":         "copilot",
			"COPILOT_PROVIDER_JSON":    `{"type":" azure ","baseUrl":"https://example.openai.azure.com","bearerToken":" bearer-token ","azure":{"apiVersion":" "}}`,
		}),
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if cfg.Session.Provider == nil || cfg.Session.Provider.Azure == nil {
		t.Fatal("Provider.Azure = nil, want configured azure provider")
	}
	if got, want := cfg.Session.Provider.Type, "azure"; got != want {
		t.Fatalf("Provider.Type = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.WireAPI, defaultProviderWireAPI; got != want {
		t.Fatalf("Provider.WireAPI = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.BearerToken, "bearer-token"; got != want {
		t.Fatalf("Provider.BearerToken = %q, want %q", got, want)
	}
	if got, want := cfg.Session.Provider.Azure.APIVersion, defaultAzureAPIVersion; got != want {
		t.Fatalf("Provider.Azure.APIVersion = %q, want %q", got, want)
	}
}

func TestLoadFromLookupRejectsInvalidCopilotProviderConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"invalid json":     `{`,
		"missing base url": `{"type":"openai"}`,
		"bad base url":     `{"baseUrl":"not-a-url"}`,
		"bad url scheme":   `{"baseUrl":"ftp://example.com"}`,
		"unsupported type": `{"type":"local","baseUrl":"https://example.com"}`,
		"bad wire api":     `{"type":"openai","baseUrl":"https://example.com","wireApi":"chat"}`,
		"anthropic wire":   `{"type":"anthropic","baseUrl":"https://example.com","wireApi":"responses"}`,
		"azure on openai":  `{"type":"openai","baseUrl":"https://example.com","azure":{"apiVersion":"2024-10-21"}}`,
		"unknown field":    `{"baseUrl":"https://example.com","unknown":true}`,
	}

	for name, raw := range cases {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadFromLookup(
				envLookup(map[string]string{
					"TELEGRAM_BOT_TOKEN":       "token",
					"TELEGRAM_ALLOWED_USER_ID": "1",
					"TELEGRAM_ALLOWED_CHAT_ID": "2",
					"COPILOT_CLI_PATH":         "copilot",
					"COPILOT_PROVIDER_JSON":    raw,
				}),
				func() (string, error) { return t.TempDir(), nil },
			)
			if err == nil {
				t.Fatalf("LoadFromLookup() error = nil, want provider validation error for %q", name)
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

func TestLoadFromLookupLoadsMonitorConfig(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	cfg, err := LoadFromLookup(
		envLookup(map[string]string{
			"TELEGRAM_BOT_TOKEN":         "token",
			"TELEGRAM_ALLOWED_USER_ID":   "1",
			"TELEGRAM_ALLOWED_CHAT_ID":   "2",
			"COPILOT_CLI_PATH":           "copilot",
			"ASSISTANT_MONITOR_ENABLED":  "true",
			"ASSISTANT_MONITOR_MODE":     " analyze_then_notify ",
			"ASSISTANT_MONITOR_INTERVAL": "2m",
			"ASSISTANT_MONITOR_JITTER":   "15s",
			"ASSISTANT_MONITOR_TIMEOUT":  "20s",
			"ASSISTANT_MONITOR_COOLDOWN": "30m",
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[
				{
					"id": "api-health",
					"url": "https://example.com/health",
					"headers": {"Authorization": " Bearer token "},
					"expected_status_codes": [200, 204]
				},
				{
					"id": "ready-health",
					"url": "https://example.com/ready",
					"method": " head "
				}
			]`,
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[
				{
					"id": "gmail-payments",
					"label_ids": [" INBOX ", " finance "],
					"subject_contains": " invoice ",
					"max_results": 25
				},
				{
					"id": "gmail-ops",
					"subject_equals": " Nightly report "
				}
			]`,
		}),
		func() (string, error) { return cwd, nil },
	)
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if !cfg.Monitor.Enabled {
		t.Fatal("Monitor should be enabled when configured")
	}
	if got, want := cfg.Monitor.Mode, MonitorModeAnalyzeThenNotify; got != want {
		t.Fatalf("Monitor.Mode = %q, want %q", got, want)
	}
	if !cfg.Monitor.UsesCopilotAnalysis() {
		t.Fatal("Monitor should report Copilot analysis mode")
	}
	if got, want := cfg.Monitor.Interval, 2*time.Minute; got != want {
		t.Fatalf("Monitor.Interval = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Jitter, 15*time.Second; got != want {
		t.Fatalf("Monitor.Jitter = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Timeout, 20*time.Second; got != want {
		t.Fatalf("Monitor.Timeout = %s, want %s", got, want)
	}
	if got, want := cfg.Monitor.Cooldown, 30*time.Minute; got != want {
		t.Fatalf("Monitor.Cooldown = %s, want %s", got, want)
	}
	if got := len(cfg.Monitor.HTTPChecks); got != 2 {
		t.Fatalf("Monitor.HTTPChecks len = %d, want 2", got)
	}
	if got := len(cfg.Monitor.GmailChecks); got != 2 {
		t.Fatalf("Monitor.GmailChecks len = %d, want 2", got)
	}

	first := cfg.Monitor.HTTPChecks[0]
	if got, want := first.Headers["Authorization"], "Bearer token"; got != want {
		t.Fatalf("first.Headers[Authorization] = %q, want %q", got, want)
	}
	if got, want := first.ExpectedStatusCodes, []int{200, 204}; !slices.Equal(got, want) {
		t.Fatalf("first.ExpectedStatusCodes = %#v, want %#v", got, want)
	}

	second := cfg.Monitor.HTTPChecks[1]
	if got, want := second.Method, "HEAD"; got != want {
		t.Fatalf("second.Method = %q, want %q", got, want)
	}
	if got, want := second.ExpectedStatusCodes, []int{200}; !slices.Equal(got, want) {
		t.Fatalf("second.ExpectedStatusCodes = %#v, want %#v", got, want)
	}

	firstGmail := cfg.Monitor.GmailChecks[0]
	if got, want := firstGmail.LabelIDs, []string{"INBOX", "finance"}; !slices.Equal(got, want) {
		t.Fatalf("firstGmail.LabelIDs = %#v, want %#v", got, want)
	}
	if got, want := firstGmail.SubjectContains, "invoice"; got != want {
		t.Fatalf("firstGmail.SubjectContains = %q, want %q", got, want)
	}
	if got, want := firstGmail.MaxResults, 25; got != want {
		t.Fatalf("firstGmail.MaxResults = %d, want %d", got, want)
	}

	secondGmail := cfg.Monitor.GmailChecks[1]
	if got, want := secondGmail.SubjectEquals, "Nightly report"; got != want {
		t.Fatalf("secondGmail.SubjectEquals = %q, want %q", got, want)
	}
	if got, want := secondGmail.MaxResults, 10; got != want {
		t.Fatalf("secondGmail.MaxResults = %d, want %d", got, want)
	}
}

func TestLoadFromLookupRejectsInvalidMonitorConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"enabled without checks": {
			"ASSISTANT_MONITOR_ENABLED": "true",
		},
		"invalid mode": {
			"ASSISTANT_MONITOR_MODE": "fire-and-forget",
		},
		"negative jitter": {
			"ASSISTANT_MONITOR_JITTER": "-1s",
		},
		"bad checks json": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[{`,
		},
		"bad gmail checks json": {
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[{`,
		},
		"trailing checks json": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[] {}`,
		},
		"trailing gmail checks json": {
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[] {}`,
		},
		"bad check url": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[{"id":"api","url":"ftp://example.com/health"}]`,
		},
		"unknown check field": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[{"id":"api","url":"https://example.com/health","unexpected":true}]`,
		},
		"duplicate check id": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON": `[
				{"id":"api","url":"https://example.com/health"},
				{"id":"api","url":"https://example.com/ready"}
			]`,
		},
		"gmail check without filters": {
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[{"id":"mail"}]`,
		},
		"gmail mutually exclusive subject filters": {
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[{"id":"mail","subject_contains":"invoice","subject_equals":"invoice"}]`,
		},
		"gmail duplicate check id": {
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[
				{"id":"mail","subject_contains":"invoice"},
				{"id":"mail","label_ids":["INBOX"]}
			]`,
		},
		"cross-source duplicate check id": {
			"ASSISTANT_MONITOR_HTTP_CHECKS_JSON":  `[{"id":"shared","url":"https://example.com/health"}]`,
			"ASSISTANT_MONITOR_GMAIL_CHECKS_JSON": `[{"id":"shared","subject_contains":"invoice"}]`,
		},
	}

	for name, extra := range cases {
		name, extra := name, extra
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			values := map[string]string{
				"TELEGRAM_BOT_TOKEN":       "token",
				"TELEGRAM_ALLOWED_USER_ID": "1",
				"TELEGRAM_ALLOWED_CHAT_ID": "2",
				"COPILOT_CLI_PATH":         "copilot",
			}
			for key, value := range extra {
				values[key] = value
			}

			_, err := LoadFromLookup(
				envLookup(values),
				func() (string, error) { return t.TempDir(), nil },
			)
			if err == nil {
				t.Fatalf("LoadFromLookup() error = nil, want monitor validation error for %q", name)
			}
		})
	}
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
