package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	sessionBindingsDirName = "sessions"
	auditDirName           = "audit"
	privilegedToolLogName  = "privileged-tool-events.ndjson"
)

// LocalFileStore persists app-owned state as local JSON and NDJSON files.
type LocalFileStore struct {
	rootDir     string
	sessionsDir string
	auditDir    string

	mu sync.RWMutex
}

var (
	_ ChatSessionStore         = (*LocalFileStore)(nil)
	_ ChatSessionResetter      = (*LocalFileStore)(nil)
	_ PrivilegedToolEventStore = (*LocalFileStore)(nil)
)

// NewLocalFileStore creates a file-backed store rooted at the configured storage dir.
func NewLocalFileStore(rootDir string) (*LocalFileStore, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, errors.New("storage root dir is required")
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root dir: %w", err)
	}

	store := &LocalFileStore{
		rootDir:     filepath.Clean(absRoot),
		sessionsDir: filepath.Join(filepath.Clean(absRoot), sessionBindingsDirName),
		auditDir:    filepath.Join(filepath.Clean(absRoot), auditDirName),
	}

	for _, dir := range []string{store.rootDir, store.sessionsDir, store.auditDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create store dir %q: %w", dir, err)
		}
	}

	return store, nil
}

// Get returns the persisted binding for a transport/chat pair, if one exists.
func (s *LocalFileStore) Get(ctx context.Context, transport string, chatID int64) (SessionBinding, bool, error) {
	if err := ctx.Err(); err != nil {
		return SessionBinding{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getLocked(transport, chatID)
}

func (s *LocalFileStore) getLocked(transport string, chatID int64) (SessionBinding, bool, error) {
	path := s.sessionPath(transport, chatID)
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SessionBinding{}, false, nil
	}
	if err != nil {
		return SessionBinding{}, false, fmt.Errorf("read session binding %q: %w", path, err)
	}

	var binding SessionBinding
	if err := json.Unmarshal(payload, &binding); err != nil {
		return SessionBinding{}, false, fmt.Errorf("decode session binding %q: %w", path, err)
	}

	return binding, true, nil
}

// Put writes a chat/session binding as a human-readable JSON document.
func (s *LocalFileStore) Put(ctx context.Context, binding SessionBinding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeSessionBindingLocked(binding, false); err != nil {
		return err
	}
	return ctx.Err()
}

// Reset persists a new generation marker for a chat so the next prompt resolves
// to a fresh Copilot session instead of resuming the previous generation.
func (s *LocalFileStore) Reset(ctx context.Context, binding SessionBinding) (SessionBinding, error) {
	if err := ctx.Err(); err != nil {
		return SessionBinding{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	transport, err := validateSessionBinding(binding, true)
	if err != nil {
		return SessionBinding{}, err
	}

	existing, found, err := s.getLocked(transport, binding.ChatID)
	if err != nil {
		return SessionBinding{}, err
	}
	if found && existing.UserID != 0 && existing.UserID != binding.UserID {
		return SessionBinding{}, fmt.Errorf("chat %d is already bound to user %d", binding.ChatID, existing.UserID)
	}

	now := time.Now().UTC()
	resetBinding := SessionBinding{
		Transport:  transport,
		ChatID:     binding.ChatID,
		UserID:     binding.UserID,
		SessionID:  "",
		Generation: max(binding.Generation, existing.Generation) + 1,
		Metadata:   binding.Metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if found {
		resetBinding.CreatedAt = existing.CreatedAt
		if isZeroMetadata(resetBinding.Metadata) {
			resetBinding.Metadata = existing.Metadata
		}
	}

	if err := s.writeSessionBindingLocked(resetBinding, true); err != nil {
		return SessionBinding{}, err
	}
	return resetBinding, ctx.Err()
}

// List loads every persisted chat/session binding from disk.
func (s *LocalFileStore) List(ctx context.Context) ([]SessionBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir %q: %w", s.sessionsDir, err)
	}

	var bindings []SessionBinding
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		payload, err := os.ReadFile(filepath.Join(s.sessionsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read session binding %q: %w", entry.Name(), err)
		}

		var binding SessionBinding
		if err := json.Unmarshal(payload, &binding); err != nil {
			return nil, fmt.Errorf("decode session binding %q: %w", entry.Name(), err)
		}

		bindings = append(bindings, binding)
	}

	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].ChatID < bindings[j].ChatID
	})

	return bindings, nil
}

// Append stores a privileged tool event in append-only NDJSON form.
func (s *LocalFileStore) Append(ctx context.Context, event PrivilegedToolEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(event.ToolName) == "" {
		return errors.New("privileged tool event tool name is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return errors.New("privileged tool event type is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", now.UnixNano())
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode privileged tool event: %w", err)
	}
	payload = append(payload, '\n')

	file, err := os.OpenFile(s.privilegedToolLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open privileged tool event log: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("append privileged tool event: %w", err)
	}

	return ctx.Err()
}

// Load loads every privileged tool event from the append-only NDJSON log.
func (s *LocalFileStore) Load(ctx context.Context) ([]PrivilegedToolEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.privilegedToolLogPath()
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open privileged tool event log %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var events []PrivilegedToolEvent
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event PrivilegedToolEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decode privileged tool event: %w", err)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan privileged tool event log %q: %w", path, err)
	}

	return events, nil
}

func (s *LocalFileStore) sessionPath(transport string, chatID int64) string {
	return filepath.Join(s.sessionsDir, fmt.Sprintf("chat-%s-%d.json", sanitizeTransportKey(transport), chatID))
}

func (s *LocalFileStore) privilegedToolLogPath() string {
	return filepath.Join(s.auditDir, privilegedToolLogName)
}

func (s *LocalFileStore) writeSessionBindingLocked(binding SessionBinding, allowEmptySessionID bool) error {
	transport, err := validateSessionBinding(binding, allowEmptySessionID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if existing, ok, err := s.getLocked(transport, binding.ChatID); err != nil {
		return err
	} else if ok && binding.CreatedAt.IsZero() {
		binding.CreatedAt = existing.CreatedAt
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = now
	}

	payload, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session binding: %w", err)
	}
	payload = append(payload, '\n')

	path := s.sessionPath(transport, binding.ChatID)
	if err := writeFileAtomic(path, payload, 0o600); err != nil {
		return fmt.Errorf("write session binding %q: %w", path, err)
	}

	return nil
}

func validateSessionBinding(binding SessionBinding, allowEmptySessionID bool) (string, error) {
	transport := strings.TrimSpace(binding.Transport)
	if transport == "" {
		return "", errors.New("session binding transport is required")
	}
	if binding.ChatID == 0 {
		return "", errors.New("session binding chat id is required")
	}
	if binding.UserID == 0 {
		return "", errors.New("session binding user id is required")
	}
	if !allowEmptySessionID && strings.TrimSpace(binding.SessionID) == "" {
		return "", errors.New("session binding session id is required")
	}
	return transport, nil
}

func isZeroMetadata(metadata TelegramChatMetadata) bool {
	return metadata == (TelegramChatMetadata{})
}

func sanitizeTransportKey(transport string) string {
	transport = strings.TrimSpace(strings.ToLower(transport))
	if transport == "" {
		return "unknown"
	}

	var builder strings.Builder
	builder.Grow(len(transport))
	lastDash := false
	for _, r := range transport {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "unknown"
	}
	return result
}

func writeFileAtomic(path string, payload []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return err
	}

	tempPath := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(payload); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}
