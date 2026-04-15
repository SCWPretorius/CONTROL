package store

import (
	"bufio"
	"context"
	"encoding/base64"
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
	monitorDirName         = "monitors"
	privilegedToolLogName  = "privileged-tool-events.ndjson"
	monitorEventLogName    = "monitor-events.ndjson"
	monitorCorrelationName = "monitor-correlations.ndjson"
)

// LocalFileStore persists app-owned state as local JSON and NDJSON files.
type LocalFileStore struct {
	rootDir         string
	sessionsDir     string
	auditDir        string
	monitorStateDir string

	mu sync.RWMutex
}

var (
	_ ChatSessionStore         = (*LocalFileStore)(nil)
	_ ChatSessionResetter      = (*LocalFileStore)(nil)
	_ PrivilegedToolEventStore = (*LocalFileStore)(nil)
	_ MonitorCheckpointStore   = (*LocalFileStore)(nil)
	_ MonitorEventStore        = (*LocalFileStore)(nil)
	_ MonitorCorrelationStore  = (*LocalFileStore)(nil)
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
		rootDir:         filepath.Clean(absRoot),
		sessionsDir:     filepath.Join(filepath.Clean(absRoot), sessionBindingsDirName),
		auditDir:        filepath.Join(filepath.Clean(absRoot), auditDirName),
		monitorStateDir: filepath.Join(filepath.Clean(absRoot), monitorDirName),
	}

	for _, dir := range []string{store.rootDir, store.sessionsDir, store.auditDir, store.monitorStateDir} {
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
	if err := normalizePrivilegedToolEvent(&event); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := appendNDJSONRecord(s.privilegedToolLogPath(), "privileged tool event", event); err != nil {
		return err
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

	return loadNDJSONRecords[PrivilegedToolEvent](ctx, s.privilegedToolLogPath(), "privileged tool event")
}

// AppendMonitorEvent stores a monitor event in append-only NDJSON form.
func (s *LocalFileStore) AppendMonitorEvent(ctx context.Context, event MonitorEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := normalizeMonitorEvent(&event); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := appendNDJSONRecord(s.monitorEventLogPath(), "monitor event", event); err != nil {
		return err
	}
	return ctx.Err()
}

// LoadMonitorEvents loads every persisted monitor event from the append-only NDJSON log.
func (s *LocalFileStore) LoadMonitorEvents(ctx context.Context) ([]MonitorEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return loadNDJSONRecords[MonitorEvent](ctx, s.monitorEventLogPath(), "monitor event")
}

// AppendMonitorCorrelation stores a monitor correlation in append-only NDJSON form.
func (s *LocalFileStore) AppendMonitorCorrelation(ctx context.Context, correlation MonitorCorrelation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := normalizeMonitorCorrelation(&correlation); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := appendNDJSONRecord(s.monitorCorrelationLogPath(), "monitor correlation", correlation); err != nil {
		return err
	}
	return ctx.Err()
}

// LoadMonitorCorrelations loads every persisted monitor correlation from the append-only NDJSON log.
func (s *LocalFileStore) LoadMonitorCorrelations(ctx context.Context) ([]MonitorCorrelation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return loadNDJSONRecords[MonitorCorrelation](ctx, s.monitorCorrelationLogPath(), "monitor correlation")
}

// GetMonitorCheckpoint returns the persisted checkpoint for a monitor check, if one exists.
func (s *LocalFileStore) GetMonitorCheckpoint(ctx context.Context, checkID string) (MonitorCheckpoint, bool, error) {
	if err := ctx.Err(); err != nil {
		return MonitorCheckpoint{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getMonitorCheckpointLocked(checkID)
}

// PutMonitorCheckpoint writes monitor state as a human-readable JSON document.
func (s *LocalFileStore) PutMonitorCheckpoint(ctx context.Context, checkpoint MonitorCheckpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeMonitorCheckpointLocked(checkpoint); err != nil {
		return err
	}
	return ctx.Err()
}

// ListMonitorCheckpoints loads every persisted monitor checkpoint from disk.
func (s *LocalFileStore) ListMonitorCheckpoints(ctx context.Context) ([]MonitorCheckpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.monitorStateDir)
	if err != nil {
		return nil, fmt.Errorf("read monitor state dir %q: %w", s.monitorStateDir, err)
	}

	var checkpoints []MonitorCheckpoint
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		payload, err := os.ReadFile(filepath.Join(s.monitorStateDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read monitor checkpoint %q: %w", entry.Name(), err)
		}

		var checkpoint MonitorCheckpoint
		if err := json.Unmarshal(payload, &checkpoint); err != nil {
			return nil, fmt.Errorf("decode monitor checkpoint %q: %w", entry.Name(), err)
		}

		checkpoints = append(checkpoints, checkpoint)
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].CheckID < checkpoints[j].CheckID
	})

	return checkpoints, nil
}

func (s *LocalFileStore) sessionPath(transport string, chatID int64) string {
	return filepath.Join(s.sessionsDir, fmt.Sprintf("chat-%s-%d.json", sanitizeTransportKey(transport), chatID))
}

func (s *LocalFileStore) privilegedToolLogPath() string {
	return filepath.Join(s.auditDir, privilegedToolLogName)
}

func (s *LocalFileStore) monitorEventLogPath() string {
	return filepath.Join(s.auditDir, monitorEventLogName)
}

func (s *LocalFileStore) monitorCorrelationLogPath() string {
	return filepath.Join(s.auditDir, monitorCorrelationName)
}

func (s *LocalFileStore) monitorCheckpointPath(checkID string) string {
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(checkID)))
	return filepath.Join(s.monitorStateDir, fmt.Sprintf("check-%s.json", encodedID))
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
	return sanitizeFileKey(strings.ToLower(transport))
}

func sanitizeFileKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	var builder strings.Builder
	builder.Grow(len(value))
	lastDash := false
	for _, r := range value {
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

func (s *LocalFileStore) getMonitorCheckpointLocked(checkID string) (MonitorCheckpoint, bool, error) {
	path := s.monitorCheckpointPath(checkID)
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return MonitorCheckpoint{}, false, nil
	}
	if err != nil {
		return MonitorCheckpoint{}, false, fmt.Errorf("read monitor checkpoint %q: %w", path, err)
	}

	var checkpoint MonitorCheckpoint
	if err := json.Unmarshal(payload, &checkpoint); err != nil {
		return MonitorCheckpoint{}, false, fmt.Errorf("decode monitor checkpoint %q: %w", path, err)
	}

	return checkpoint, true, nil
}

func (s *LocalFileStore) writeMonitorCheckpointLocked(checkpoint MonitorCheckpoint) error {
	checkID, err := normalizeMonitorCheckpoint(&checkpoint)
	if err != nil {
		return err
	}

	if checkpoint.UpdatedAt.IsZero() {
		checkpoint.UpdatedAt = time.Now().UTC()
	}

	payload, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("encode monitor checkpoint: %w", err)
	}
	payload = append(payload, '\n')

	path := s.monitorCheckpointPath(checkID)
	if err := writeFileAtomic(path, payload, 0o600); err != nil {
		return fmt.Errorf("write monitor checkpoint %q: %w", path, err)
	}

	return nil
}

func normalizeMonitorCheckpoint(checkpoint *MonitorCheckpoint) (string, error) {
	if checkpoint == nil {
		return "", errors.New("monitor checkpoint is required")
	}

	checkpoint.CheckID = strings.TrimSpace(checkpoint.CheckID)
	checkpoint.LastSeenCondition = strings.TrimSpace(checkpoint.LastSeenCondition)
	checkpoint.Fingerprint = strings.TrimSpace(checkpoint.Fingerprint)

	if checkpoint.CheckID == "" {
		return "", errors.New("monitor checkpoint check id is required")
	}
	if checkpoint.LastSeenCondition == "" {
		return "", errors.New("monitor checkpoint last seen condition is required")
	}
	if !checkpoint.LastAlertAt.IsZero() && !checkpoint.CooldownUntil.IsZero() && checkpoint.CooldownUntil.Before(checkpoint.LastAlertAt) {
		return "", errors.New("monitor checkpoint cooldown until must not be before last alert at")
	}

	return checkpoint.CheckID, nil
}

func normalizePrivilegedToolEvent(event *PrivilegedToolEvent) error {
	if event == nil {
		return errors.New("privileged tool event is required")
	}

	event.ID = strings.TrimSpace(event.ID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.ToolName = strings.TrimSpace(event.ToolName)
	event.EventType = strings.TrimSpace(event.EventType)
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.Summary = strings.TrimSpace(event.Summary)

	if event.ToolName == "" {
		return errors.New("privileged tool event tool name is required")
	}
	if event.EventType == "" {
		return errors.New("privileged tool event type is required")
	}

	now := time.Now().UTC()
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", now.UnixNano())
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	return nil
}

func normalizeMonitorEvent(event *MonitorEvent) error {
	if event == nil {
		return errors.New("monitor event is required")
	}

	event.ID = strings.TrimSpace(event.ID)
	event.CheckID = strings.TrimSpace(event.CheckID)
	event.IncidentID = strings.TrimSpace(event.IncidentID)
	event.CorrelationID = strings.TrimSpace(event.CorrelationID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.Condition = strings.TrimSpace(event.Condition)
	event.Fingerprint = strings.TrimSpace(event.Fingerprint)
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.Summary = strings.TrimSpace(event.Summary)

	if event.CheckID == "" {
		return errors.New("monitor event check id is required")
	}
	if event.EventType == "" {
		return errors.New("monitor event type is required")
	}

	now := time.Now().UTC()
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", now.UnixNano())
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	return nil
}

func normalizeMonitorCorrelation(correlation *MonitorCorrelation) error {
	if correlation == nil {
		return errors.New("monitor correlation is required")
	}

	correlation.ID = strings.TrimSpace(correlation.ID)
	correlation.CheckID = strings.TrimSpace(correlation.CheckID)
	correlation.IncidentID = strings.TrimSpace(correlation.IncidentID)
	correlation.CorrelationID = strings.TrimSpace(correlation.CorrelationID)
	correlation.SourceType = strings.TrimSpace(correlation.SourceType)
	correlation.SourceID = strings.TrimSpace(correlation.SourceID)
	correlation.TargetType = strings.TrimSpace(correlation.TargetType)
	correlation.TargetID = strings.TrimSpace(correlation.TargetID)
	correlation.Relationship = strings.TrimSpace(correlation.Relationship)

	if correlation.CheckID == "" {
		return errors.New("monitor correlation check id is required")
	}
	if correlation.SourceType == "" {
		return errors.New("monitor correlation source type is required")
	}
	if correlation.SourceID == "" {
		return errors.New("monitor correlation source id is required")
	}
	if correlation.TargetType == "" {
		return errors.New("monitor correlation target type is required")
	}
	if correlation.TargetID == "" {
		return errors.New("monitor correlation target id is required")
	}
	if correlation.Relationship == "" {
		return errors.New("monitor correlation relationship is required")
	}

	now := time.Now().UTC()
	if correlation.ID == "" {
		correlation.ID = fmt.Sprintf("%d", now.UnixNano())
	}
	if correlation.RecordedAt.IsZero() {
		correlation.RecordedAt = now
	}
	return nil
}

func appendNDJSONRecord(path string, recordType string, record any) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode %s: %w", recordType, err)
	}
	payload = append(payload, '\n')

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s log: %w", recordType, err)
	}
	defer file.Close()

	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("append %s: %w", recordType, err)
	}
	return nil
}

func loadNDJSONRecords[T any](ctx context.Context, path string, recordType string) ([]T, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s log %q: %w", recordType, path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var records []T
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("decode %s: %w", recordType, err)
		}
		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s log %q: %w", recordType, path, err)
	}
	return records, nil
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
