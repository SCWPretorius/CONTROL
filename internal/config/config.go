package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRuntimeDir         = "var/runtime"
	defaultStorageDir         = "var/storage"
	defaultModel              = "gpt-5.4"
	defaultReasoning          = "medium"
	defaultNamespace          = "telegram-personal-assistant"
	defaultResumeSession      = true
	defaultToolHTTPTimeout    = 30 * time.Second
	defaultToolShellTimeout   = 30 * time.Second
	defaultToolMaxOutputBytes = 64 * 1024
	defaultProviderType       = "openai"
	defaultProviderWireAPI    = "completions"
	defaultAzureAPIVersion    = "2024-10-21"
	defaultMonitorMode        = MonitorModeNotifyOnly
	defaultMonitorInterval    = time.Minute
	defaultMonitorJitter      = 10 * time.Second
	defaultMonitorTimeout     = 10 * time.Second
	defaultMonitorCooldown    = 15 * time.Minute
)

var defaultGoogleOAuthScopes = []string{
	"https://www.googleapis.com/auth/gmail.modify",
	"https://www.googleapis.com/auth/calendar",
	"https://www.googleapis.com/auth/userinfo.email",
}

// Config contains the foundation configuration shared by the future runtime.
type Config struct {
	Telegram TelegramConfig
	Copilot  CopilotConfig
	Paths    PathConfig
	Session  SessionConfig
	Tools    ToolConfig
	Monitor  MonitorConfig
}

type TelegramConfig struct {
	BotToken      string
	AllowedUserID int64
	AllowedChatID int64
}

type CopilotConfig struct {
	CLIPath string
	CLIURL  string
}

func (c CopilotConfig) Transport() string {
	if c.CLIURL != "" {
		return "remote"
	}

	return "stdio"
}

type PathConfig struct {
	RuntimeDir string
	StorageDir string
}

type SessionConfig struct {
	Model           string
	ReasoningEffort string
	Namespace       string
	ResumeSessions  bool
	WorkingDir      string
	ConfigDir       string
	Provider        *CopilotProviderConfig
}

type ToolConfig struct {
	Google     GoogleToolConfig
	MCP        MCPToolConfig
	Privileged PrivilegedToolConfig
	Runtime    ToolRuntimeConfig
}

type CopilotProviderConfig struct {
	Type        string                      `json:"type,omitempty"`
	WireAPI     string                      `json:"wireApi,omitempty"`
	BaseURL     string                      `json:"baseUrl"`
	APIKey      string                      `json:"apiKey,omitempty"`
	BearerToken string                      `json:"bearerToken,omitempty"`
	Azure       *CopilotAzureProviderConfig `json:"azure,omitempty"`
}

type CopilotAzureProviderConfig struct {
	APIVersion string `json:"apiVersion,omitempty"`
}

func (c *CopilotProviderConfig) Enabled() bool {
	return c != nil
}

func (c *CopilotProviderConfig) NormalizedType() string {
	if c == nil || strings.TrimSpace(c.Type) == "" {
		return defaultProviderType
	}
	return strings.TrimSpace(strings.ToLower(c.Type))
}

type GoogleToolConfig struct {
	OAuth       GoogleOAuthConfig
	AccessToken string
}

type MCPToolConfig struct {
	Servers map[string]MCPServerConfig
	Admin   MCPAdminConfig
}

type MCPServerConfig struct {
	Type    string
	Tools   []string
	Timeout int
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
	URL     string
	Headers map[string]string
}

type MCPAdminConfig struct {
	ListenAddress string
	BearerToken   string
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

func (c GoogleOAuthConfig) Enabled() bool {
	return strings.TrimSpace(c.ClientID) != "" ||
		strings.TrimSpace(c.ClientSecret) != "" ||
		strings.TrimSpace(c.RedirectURL) != ""
}

func (c GoogleToolConfig) Enabled() bool {
	return c.OAuth.Enabled()
}

func (c GoogleToolConfig) AccessTokenConfigured() bool {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		return false
	}

	// Treat common placeholder values as unset so first-run OAuth can proceed.
	lower := strings.ToLower(token)
	if strings.Contains(lower, "replace-with") || strings.Contains(lower, "your-") || strings.Contains(lower, "placeholder") {
		return false
	}

	return true
}

func (c GoogleToolConfig) RuntimeEnabled() bool {
	return c.Enabled() && c.AccessTokenConfigured()
}

func (c MCPToolConfig) Enabled() bool {
	return len(c.Servers) > 0
}

func (c MCPAdminConfig) Enabled() bool {
	return strings.TrimSpace(c.ListenAddress) != "" && strings.TrimSpace(c.BearerToken) != ""
}

type PrivilegedToolConfig struct {
	AllowedWorkspaceRoots  []string
	AssistantWritableRoots []string
	ShellAutoApprove       []string
}

type ToolRuntimeConfig struct {
	HTTPTimeout           time.Duration
	ShellCommandTimeout   time.Duration
	MaxCommandOutputBytes int
}

const (
	MonitorModeNotifyOnly        = "notify_only"
	MonitorModeAnalyzeThenNotify = "analyze_then_notify"
	MonitorModeAutoFixDisabled   = "auto_fix_disabled"
)

type MonitorConfig struct {
	Enabled    bool
	Mode       string
	Interval   time.Duration
	Jitter     time.Duration
	Timeout    time.Duration
	Cooldown   time.Duration
	HTTPChecks []MonitorHTTPCheckConfig
}

func (c MonitorConfig) Configured() bool {
	return c.Enabled || len(c.HTTPChecks) > 0
}

func (c MonitorConfig) UsesCopilotAnalysis() bool {
	return c.Mode == MonitorModeAnalyzeThenNotify
}

type MonitorHTTPCheckConfig struct {
	ID                  string            `json:"id"`
	URL                 string            `json:"url"`
	Method              string            `json:"method,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	ExpectedStatusCodes []int             `json:"expected_status_codes,omitempty"`
}

// Load reads configuration from environment variables and resolves paths.
func Load() (Config, error) {
	return LoadFromLookup(os.LookupEnv, os.Getwd)
}

func LoadFromLookup(lookup func(string) (string, bool), getwd func() (string, error)) (Config, error) {
	cwd, err := getwd()
	if err != nil {
		return Config{}, fmt.Errorf("get working directory: %w", err)
	}

	cfg := Config{
		Telegram: TelegramConfig{
			BotToken: strings.TrimSpace(requiredEnv(lookup, "TELEGRAM_BOT_TOKEN")),
		},
		Copilot: CopilotConfig{
			CLIPath: strings.TrimSpace(optionalEnv(lookup, "COPILOT_CLI_PATH", "")),
			CLIURL:  strings.TrimSpace(optionalEnv(lookup, "COPILOT_CLI_URL", "")),
		},
		Paths: PathConfig{
			RuntimeDir: optionalEnv(lookup, "ASSISTANT_RUNTIME_DIR", defaultRuntimeDir),
			StorageDir: optionalEnv(lookup, "ASSISTANT_STORAGE_DIR", defaultStorageDir),
		},
		Session: SessionConfig{
			Model:           optionalEnv(lookup, "COPILOT_MODEL", defaultModel),
			ReasoningEffort: optionalEnv(lookup, "COPILOT_REASONING_EFFORT", defaultReasoning),
			Namespace:       optionalEnv(lookup, "COPILOT_SESSION_NAMESPACE", defaultNamespace),
			ResumeSessions:  optionalBoolEnv(lookup, "COPILOT_RESUME_SESSIONS", defaultResumeSession),
			WorkingDir:      optionalEnv(lookup, "COPILOT_WORKING_DIR", cwd),
		},
		Tools: ToolConfig{
			Google: GoogleToolConfig{
				OAuth: GoogleOAuthConfig{
					ClientID:     strings.TrimSpace(optionalEnv(lookup, "GOOGLE_OAUTH_CLIENT_ID", "")),
					ClientSecret: strings.TrimSpace(optionalEnv(lookup, "GOOGLE_OAUTH_CLIENT_SECRET", "")),
					RedirectURL:  strings.TrimSpace(optionalEnv(lookup, "GOOGLE_OAUTH_REDIRECT_URL", "")),
					Scopes:       defaultGoogleOAuthScopes,
				},
				AccessToken: strings.TrimSpace(optionalEnv(lookup, "GOOGLE_OAUTH_ACCESS_TOKEN", "")),
			},
			MCP: MCPToolConfig{
				Admin: MCPAdminConfig{
					ListenAddress: optionalEnv(lookup, "ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR", ""),
					BearerToken:   optionalEnv(lookup, "ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN", ""),
				},
			},
			Runtime: ToolRuntimeConfig{},
		},
		Monitor: MonitorConfig{
			Enabled:  optionalBoolEnv(lookup, "ASSISTANT_MONITOR_ENABLED", false),
			Mode:     optionalEnv(lookup, "ASSISTANT_MONITOR_MODE", defaultMonitorMode),
			Interval: defaultMonitorInterval,
			Jitter:   defaultMonitorJitter,
			Timeout:  defaultMonitorTimeout,
			Cooldown: defaultMonitorCooldown,
		},
	}

	if cfg.Telegram.AllowedUserID, err = requiredInt64Env(lookup, "TELEGRAM_ALLOWED_USER_ID"); err != nil {
		return Config{}, err
	}

	if cfg.Telegram.AllowedChatID, err = requiredInt64Env(lookup, "TELEGRAM_ALLOWED_CHAT_ID"); err != nil {
		return Config{}, err
	}

	if cfg.Telegram.BotToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	if cfg.Copilot.CLIPath == "" && cfg.Copilot.CLIURL == "" {
		return Config{}, errors.New("set exactly one of COPILOT_CLI_PATH or COPILOT_CLI_URL")
	}

	if cfg.Copilot.CLIPath != "" && cfg.Copilot.CLIURL != "" {
		return Config{}, errors.New("COPILOT_CLI_PATH and COPILOT_CLI_URL are mutually exclusive")
	}

	cfg.Paths.RuntimeDir, err = normalizePath(cwd, cfg.Paths.RuntimeDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve runtime dir: %w", err)
	}

	cfg.Paths.StorageDir, err = normalizePath(cwd, cfg.Paths.StorageDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve storage dir: %w", err)
	}

	cfg.Session.WorkingDir, err = normalizePath(cwd, cfg.Session.WorkingDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve working dir: %w", err)
	}

	cfg.Session.ConfigDir = optionalEnv(lookup, "COPILOT_CONFIG_DIR", filepath.Join(cfg.Paths.RuntimeDir, "copilot"))
	cfg.Session.ConfigDir, err = normalizePath(cwd, cfg.Session.ConfigDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve copilot config dir: %w", err)
	}
	cfg.Session.Provider, err = optionalCopilotProviderEnv(lookup, "COPILOT_PROVIDER_JSON")
	if err != nil {
		return Config{}, err
	}
	if err := ValidateAndNormalizeCopilotProvider(cfg.Session.Provider); err != nil {
		return Config{}, err
	}

	cfg.Tools.Google.OAuth.Scopes = optionalCSVEnv(lookup, "GOOGLE_OAUTH_SCOPES", defaultGoogleOAuthScopes)
	if cfg.Tools.Google.OAuth.Enabled() {
		if cfg.Tools.Google.OAuth.ClientID == "" || cfg.Tools.Google.OAuth.ClientSecret == "" || cfg.Tools.Google.OAuth.RedirectURL == "" {
			return Config{}, errors.New("GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and GOOGLE_OAUTH_REDIRECT_URL must all be set together")
		}
	}
	if cfg.Tools.Google.AccessTokenConfigured() && !cfg.Tools.Google.OAuth.Enabled() {
		return Config{}, errors.New("GOOGLE_OAUTH_ACCESS_TOKEN requires GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and GOOGLE_OAUTH_REDIRECT_URL")
	}
	if len(cfg.Tools.Google.OAuth.Scopes) == 0 {
		return Config{}, errors.New("GOOGLE_OAUTH_SCOPES must include at least one scope")
	}

	cfg.Tools.MCP.Servers, err = optionalMCPServersEnv(lookup, "ASSISTANT_TOOL_MCP_SERVERS_JSON")
	if err != nil {
		return Config{}, err
	}
	if err := ValidateAndNormalizeMCPServers(cwd, cfg.Tools.MCP.Servers); err != nil {
		return Config{}, err
	}
	if err := ValidateAndNormalizeMCPAdminConfig(&cfg.Tools.MCP.Admin); err != nil {
		return Config{}, err
	}

	cfg.Tools.Privileged.AllowedWorkspaceRoots, err = normalizePathList(cwd,
		optionalPathListEnv(lookup, "ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS", cfg.Session.WorkingDir),
	)
	if err != nil {
		return Config{}, fmt.Errorf("resolve allowed workspace roots: %w", err)
	}
	if len(cfg.Tools.Privileged.AllowedWorkspaceRoots) == 0 {
		return Config{}, errors.New("ASSISTANT_TOOL_ALLOWED_WORKSPACE_ROOTS must include at least one path")
	}

	cfg.Tools.Privileged.AssistantWritableRoots, err = normalizePathList(cwd,
		optionalPathListEnv(lookup, "ASSISTANT_TOOL_WRITABLE_ROOTS", cfg.Paths.RuntimeDir, cfg.Paths.StorageDir),
	)
	if err != nil {
		return Config{}, fmt.Errorf("resolve assistant writable roots: %w", err)
	}
	if len(cfg.Tools.Privileged.AssistantWritableRoots) == 0 {
		return Config{}, errors.New("ASSISTANT_TOOL_WRITABLE_ROOTS must include at least one path")
	}

	cfg.Tools.Privileged.ShellAutoApprove = optionalCSVEnv(lookup, "ASSISTANT_TOOL_SHELL_AUTO_APPROVE", nil)
	for _, entry := range cfg.Tools.Privileged.ShellAutoApprove {
		if err := validateShellAutoApproveEntry(entry); err != nil {
			return Config{}, fmt.Errorf("invalid ASSISTANT_TOOL_SHELL_AUTO_APPROVE entry %q: %w", entry, err)
		}
	}

	cfg.Tools.Runtime.HTTPTimeout, err = optionalDurationEnv(lookup, "ASSISTANT_TOOL_HTTP_TIMEOUT", defaultToolHTTPTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg.Tools.Runtime.ShellCommandTimeout, err = optionalDurationEnv(lookup, "ASSISTANT_TOOL_SHELL_TIMEOUT", defaultToolShellTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg.Tools.Runtime.MaxCommandOutputBytes, err = optionalIntEnv(lookup, "ASSISTANT_TOOL_MAX_OUTPUT_BYTES", defaultToolMaxOutputBytes)
	if err != nil {
		return Config{}, err
	}

	cfg.Monitor.Interval, err = optionalDurationEnv(lookup, "ASSISTANT_MONITOR_INTERVAL", defaultMonitorInterval)
	if err != nil {
		return Config{}, err
	}

	cfg.Monitor.Jitter, err = optionalDurationEnv(lookup, "ASSISTANT_MONITOR_JITTER", defaultMonitorJitter)
	if err != nil {
		return Config{}, err
	}

	cfg.Monitor.Timeout, err = optionalDurationEnv(lookup, "ASSISTANT_MONITOR_TIMEOUT", defaultMonitorTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg.Monitor.Cooldown, err = optionalDurationEnv(lookup, "ASSISTANT_MONITOR_COOLDOWN", defaultMonitorCooldown)
	if err != nil {
		return Config{}, err
	}

	cfg.Monitor.HTTPChecks, err = optionalMonitorHTTPChecksEnv(lookup, "ASSISTANT_MONITOR_HTTP_CHECKS_JSON")
	if err != nil {
		return Config{}, err
	}
	if err := ValidateAndNormalizeMonitorConfig(&cfg.Monitor); err != nil {
		return Config{}, err
	}

	if cfg.Session.Model == "" {
		return Config{}, errors.New("COPILOT_MODEL must not be empty")
	}

	if cfg.Session.Namespace == "" {
		return Config{}, errors.New("COPILOT_SESSION_NAMESPACE must not be empty")
	}

	if cfg.Tools.Runtime.HTTPTimeout <= 0 {
		return Config{}, errors.New("ASSISTANT_TOOL_HTTP_TIMEOUT must be greater than zero")
	}

	if cfg.Tools.Runtime.ShellCommandTimeout <= 0 {
		return Config{}, errors.New("ASSISTANT_TOOL_SHELL_TIMEOUT must be greater than zero")
	}

	if cfg.Tools.Runtime.MaxCommandOutputBytes <= 0 {
		return Config{}, errors.New("ASSISTANT_TOOL_MAX_OUTPUT_BYTES must be greater than zero")
	}

	return cfg, nil
}

func requiredEnv(lookup func(string) (string, bool), key string) string {
	value, ok := lookup(key)
	if !ok {
		return ""
	}

	return value
}

func optionalEnv(lookup func(string) (string, bool), key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return trimmed
}

func requiredInt64Env(lookup func(string) (string, bool), key string) (int64, error) {
	raw := strings.TrimSpace(requiredEnv(lookup, key))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid int64: %w", key, err)
	}

	return value, nil
}

func optionalBoolEnv(lookup func(string) (string, bool), key string, fallback bool) bool {
	raw, ok := lookup(key)
	if !ok {
		return fallback
	}

	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}

	return value
}

func optionalIntEnv(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}

	return value, nil
}

func optionalDurationEnv(lookup func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}

	return value, nil
}

func optionalCSVEnv(lookup func(string) (string, bool), key string, fallback []string) []string {
	raw, ok := lookup(key)
	if !ok {
		return append([]string(nil), fallback...)
	}

	values := splitAndTrim(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	if len(values) == 0 {
		return append([]string(nil), fallback...)
	}

	return values
}

func optionalPathListEnv(lookup func(string) (string, bool), key string, fallback ...string) []string {
	raw, ok := lookup(key)
	if !ok {
		return append([]string(nil), fallback...)
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return append([]string(nil), fallback...)
	}

	values := filepath.SplitList(trimmed)
	if len(values) == 0 {
		return append([]string(nil), fallback...)
	}

	return values
}

func optionalMCPServersEnv(lookup func(string) (string, bool), key string) (map[string]MCPServerConfig, error) {
	raw, ok := lookup(key)
	if !ok {
		return nil, nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var rawServers map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawServers); err != nil {
		return nil, fmt.Errorf("%s must be valid JSON: %w", key, err)
	}

	servers := make(map[string]MCPServerConfig, len(rawServers))
	for name, payload := range rawServers {
		var server MCPServerConfig
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&server); err != nil {
			return nil, fmt.Errorf("%s server %q must match the supported MCP schema: %w", key, name, err)
		}
		servers[name] = server
	}

	return servers, nil
}

func optionalMonitorHTTPChecksEnv(lookup func(string) (string, bool), key string) ([]MonitorHTTPCheckConfig, error) {
	raw, ok := lookup(key)
	if !ok {
		return nil, nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var checks []MonitorHTTPCheckConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checks); err != nil {
		return nil, fmt.Errorf("%s must match the supported monitor HTTP check schema: %w", key, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s must contain exactly one JSON array value", key)
	}

	return checks, nil
}

func optionalCopilotProviderEnv(lookup func(string) (string, bool), key string) (*CopilotProviderConfig, error) {
	raw, ok := lookup(key)
	if !ok {
		return nil, nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var provider CopilotProviderConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provider); err != nil {
		return nil, fmt.Errorf("%s must match the supported Copilot provider schema: %w", key, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s must contain exactly one JSON object value", key)
	}

	return &provider, nil
}

func normalizePath(baseDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	return filepath.Abs(filepath.Join(baseDir, path))
}

func normalizePathList(baseDir string, paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, candidate := range paths {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		value, err := normalizePath(baseDir, candidate)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	return normalized, nil
}

func splitAndTrim(raw string, separator func(rune) bool) []string {
	parts := strings.FieldsFunc(raw, separator)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}

	return values
}

func validateShellAutoApproveEntry(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return errors.New("entry must not be empty")
	}

	for _, forbidden := range []string{"&&", "&", "||", "|", ";", ">", "<", "`", "$("} {
		if strings.Contains(entry, forbidden) {
			return fmt.Errorf("entry must not contain %q", forbidden)
		}
	}

	return nil
}

func ValidateAndNormalizeMCPServers(baseDir string, servers map[string]MCPServerConfig) error {
	return validateAndNormalizeMCPServers(baseDir, servers)
}

func validateAndNormalizeMCPServers(baseDir string, servers map[string]MCPServerConfig) error {
	for name, server := range servers {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return errors.New("ASSISTANT_TOOL_MCP_SERVERS_JSON server names must not be empty")
		}
		if trimmedName != name {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server name %q must not contain leading or trailing whitespace", name)
		}
		if err := validateAndNormalizeMCPServer(baseDir, trimmedName, &server); err != nil {
			return err
		}
		servers[name] = server
	}

	return nil
}

func ValidateAndNormalizeMCPServer(baseDir, name string, server *MCPServerConfig) error {
	return validateAndNormalizeMCPServer(baseDir, name, server)
}

func validateAndNormalizeMCPServer(baseDir, name string, server *MCPServerConfig) error {
	server.Type = strings.TrimSpace(strings.ToLower(server.Type))
	server.Tools = cleanNonEmptyStrings(server.Tools)
	server.Command = strings.TrimSpace(server.Command)
	server.Args = cleanStrings(server.Args)
	server.URL = strings.TrimSpace(server.URL)

	if len(server.Tools) == 0 {
		return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q must include at least one tool selector", name)
	}
	if server.Timeout < 0 {
		return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q timeout must not be negative", name)
	}

	switch server.Type {
	case "local", "stdio":
		if server.Command == "" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q requires command for local/stdio MCP", name)
		}
		if server.URL != "" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q must not set url for local/stdio MCP", name)
		}
		if len(server.Headers) > 0 {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q must not set headers for local/stdio MCP", name)
		}
		if server.Cwd = strings.TrimSpace(server.Cwd); server.Cwd != "" {
			cwd, err := normalizePath(baseDir, server.Cwd)
			if err != nil {
				return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q cwd: %w", name, err)
			}
			server.Cwd = cwd
		}
		var err error
		server.Env, err = normalizeStringMap(server.Env)
		if err != nil {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q env: %w", name, err)
		}
	case "http", "sse":
		if server.URL == "" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q requires url for remote MCP", name)
		}
		if server.Command != "" || len(server.Args) > 0 || len(server.Env) > 0 || strings.TrimSpace(server.Cwd) != "" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q must not set local-only fields for remote MCP", name)
		}
		parsedURL, err := url.Parse(server.URL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q url must be an absolute URL", name)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q url scheme must be http or https", name)
		}
		server.Headers, err = normalizeStringMap(server.Headers)
		if err != nil {
			return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q headers: %w", name, err)
		}
	default:
		return fmt.Errorf("ASSISTANT_TOOL_MCP_SERVERS_JSON server %q has unsupported type %q", name, server.Type)
	}

	return nil
}

func ValidateAndNormalizeMCPAdminConfig(admin *MCPAdminConfig) error {
	if admin == nil {
		return nil
	}

	admin.ListenAddress = strings.TrimSpace(admin.ListenAddress)
	admin.BearerToken = strings.TrimSpace(admin.BearerToken)
	if admin.ListenAddress == "" && admin.BearerToken == "" {
		return nil
	}
	if admin.ListenAddress == "" || admin.BearerToken == "" {
		return errors.New("ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR and ASSISTANT_TOOL_MCP_ADMIN_BEARER_TOKEN must both be set to enable runtime MCP registration")
	}

	host, port, err := net.SplitHostPort(admin.ListenAddress)
	if err != nil {
		return fmt.Errorf("ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR must be a valid host:port: %w", err)
	}
	if host != "127.0.0.1" {
		return errors.New("ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR must bind to 127.0.0.1")
	}
	if strings.TrimSpace(port) == "" {
		return errors.New("ASSISTANT_TOOL_MCP_ADMIN_LISTEN_ADDR must include a TCP port")
	}

	return nil
}

func ValidateAndNormalizeCopilotProvider(provider *CopilotProviderConfig) error {
	if provider == nil {
		return nil
	}

	provider.Type = strings.TrimSpace(strings.ToLower(provider.Type))
	if provider.Type == "" {
		provider.Type = defaultProviderType
	}
	provider.WireAPI = strings.TrimSpace(strings.ToLower(provider.WireAPI))
	provider.BaseURL = strings.TrimSpace(provider.BaseURL)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	provider.BearerToken = strings.TrimSpace(provider.BearerToken)

	if provider.BaseURL == "" {
		return errors.New("COPILOT_PROVIDER_JSON baseUrl is required")
	}
	parsedURL, err := url.Parse(provider.BaseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("COPILOT_PROVIDER_JSON baseUrl must be an absolute URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("COPILOT_PROVIDER_JSON baseUrl scheme must be http or https")
	}

	switch provider.Type {
	case "openai", "azure", "anthropic":
	default:
		return fmt.Errorf("COPILOT_PROVIDER_JSON type %q is unsupported", provider.Type)
	}

	switch provider.Type {
	case "openai", "azure":
		if provider.WireAPI == "" {
			provider.WireAPI = defaultProviderWireAPI
		}
		switch provider.WireAPI {
		case "completions", "responses":
		default:
			return fmt.Errorf("COPILOT_PROVIDER_JSON wireApi %q is unsupported for provider type %q", provider.WireAPI, provider.Type)
		}
	case "anthropic":
		if provider.WireAPI != "" {
			return errors.New("COPILOT_PROVIDER_JSON wireApi is only supported for openai or azure providers")
		}
	}

	if provider.Type != "azure" {
		if provider.Azure != nil {
			return errors.New("COPILOT_PROVIDER_JSON azure options are only supported when type is \"azure\"")
		}
		return nil
	}
	if provider.Azure == nil {
		return nil
	}

	provider.Azure.APIVersion = strings.TrimSpace(provider.Azure.APIVersion)
	if provider.Azure.APIVersion == "" {
		provider.Azure.APIVersion = defaultAzureAPIVersion
	}

	return nil
}

func ValidateAndNormalizeMonitorConfig(cfg *MonitorConfig) error {
	if cfg == nil {
		return nil
	}

	cfg.Mode = strings.TrimSpace(strings.ToLower(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = defaultMonitorMode
	}

	switch cfg.Mode {
	case MonitorModeNotifyOnly, MonitorModeAnalyzeThenNotify, MonitorModeAutoFixDisabled:
	default:
		return fmt.Errorf("ASSISTANT_MONITOR_MODE must be one of %s, %s, or %s", MonitorModeNotifyOnly, MonitorModeAnalyzeThenNotify, MonitorModeAutoFixDisabled)
	}

	if cfg.Interval <= 0 {
		return errors.New("ASSISTANT_MONITOR_INTERVAL must be greater than zero")
	}
	if cfg.Jitter < 0 {
		return errors.New("ASSISTANT_MONITOR_JITTER must not be negative")
	}
	if cfg.Timeout <= 0 {
		return errors.New("ASSISTANT_MONITOR_TIMEOUT must be greater than zero")
	}
	if cfg.Cooldown < 0 {
		return errors.New("ASSISTANT_MONITOR_COOLDOWN must not be negative")
	}

	for index := range cfg.HTTPChecks {
		if err := validateAndNormalizeMonitorHTTPCheck(index, &cfg.HTTPChecks[index]); err != nil {
			return err
		}
	}
	seenCheckIDs := make(map[string]struct{}, len(cfg.HTTPChecks))
	for _, check := range cfg.HTTPChecks {
		if _, exists := seenCheckIDs[check.ID]; exists {
			return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON contains duplicate check id %q", check.ID)
		}
		seenCheckIDs[check.ID] = struct{}{}
	}

	if cfg.Enabled && len(cfg.HTTPChecks) == 0 {
		return errors.New("ASSISTANT_MONITOR_HTTP_CHECKS_JSON must include at least one HTTP check when monitoring is enabled")
	}

	return nil
}

func validateAndNormalizeMonitorHTTPCheck(index int, check *MonitorHTTPCheckConfig) error {
	check.ID = strings.TrimSpace(check.ID)
	check.URL = strings.TrimSpace(check.URL)
	check.Method = strings.ToUpper(strings.TrimSpace(check.Method))
	if check.Method == "" {
		check.Method = "GET"
	}

	if check.ID == "" {
		return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %d id is required", index)
	}
	if check.URL == "" {
		return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %q url is required", check.ID)
	}
	parsedURL, err := url.Parse(check.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %q url must be an absolute URL", check.ID)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %q url scheme must be http or https", check.ID)
	}

	check.Headers, err = normalizeStringMap(check.Headers)
	if err != nil {
		return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %q headers: %w", check.ID, err)
	}

	if len(check.ExpectedStatusCodes) == 0 {
		check.ExpectedStatusCodes = []int{200}
	}
	for _, code := range check.ExpectedStatusCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("ASSISTANT_MONITOR_HTTP_CHECKS_JSON check %q expected status codes must be valid HTTP status codes", check.ID)
		}
	}

	return nil
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		cleaned = append(cleaned, strings.TrimSpace(value))
	}

	return cleaned
}

func cleanNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}

	return cleaned
}

func normalizeStringMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("keys must not be empty")
		}
		normalized[key] = strings.TrimSpace(value)
	}

	return normalized, nil
}
