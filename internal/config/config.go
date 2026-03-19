package config

import (
	"errors"
	"fmt"
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
}

type ToolConfig struct {
	Google     GoogleToolConfig
	Privileged PrivilegedToolConfig
	Runtime    ToolRuntimeConfig
}

type GoogleToolConfig struct {
	OAuth       GoogleOAuthConfig
	AccessToken string
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
			Runtime: ToolRuntimeConfig{},
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
