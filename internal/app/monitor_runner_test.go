package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

func TestHTTPMonitorRunnerExecuteHTTPCheckDedupesWithinCooldown(t *testing.T) {
	t.Parallel()

	checkStore := &memoryMonitorCheckpointStore{}
	alerts := &stubMonitorAlertSender{}
	doer := &staticHTTPDoer{
		response: &http.Response{
			StatusCode: 500,
			Body:       http.NoBody,
		},
	}
	now := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	runner := &httpMonitorRunner{
		logger:      defaultLogger(nil),
		config:      config.MonitorConfig{Mode: config.MonitorModeNotifyOnly, Timeout: 50 * time.Millisecond, Cooldown: time.Minute},
		checkpoints: checkStore,
		alerts:      alerts,
		client:      doer,
		now:         func() time.Time { return now },
		jitter:      func(time.Duration) time.Duration { return 0 },
	}
	check := config.MonitorHTTPCheckConfig{
		ID:                  "api-health",
		Method:              http.MethodGet,
		URL:                 "https://example.com/health",
		ExpectedStatusCodes: []int{200},
	}

	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck(first) error = %v", err)
	}
	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck(second) error = %v", err)
	}

	if got := len(alerts.messages); got != 1 {
		t.Fatalf("alert count = %d, want 1", got)
	}
	checkpoint, ok, err := checkStore.GetMonitorCheckpoint(context.Background(), check.ID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint not persisted")
	}
	if checkpoint.LastSeenCondition != "status:500" {
		t.Fatalf("LastSeenCondition = %q, want %q", checkpoint.LastSeenCondition, "status:500")
	}
	if checkpoint.LastAlertAt.IsZero() {
		t.Fatal("expected LastAlertAt to be set")
	}
}

func TestHTTPMonitorRunnerExecuteHTTPCheckAlertsAgainAfterCooldownExpires(t *testing.T) {
	t.Parallel()

	checkStore := &memoryMonitorCheckpointStore{}
	alerts := &stubMonitorAlertSender{}
	now := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	runner := &httpMonitorRunner{
		logger: defaultLogger(nil),
		config: config.MonitorConfig{
			Mode:     config.MonitorModeNotifyOnly,
			Timeout:  50 * time.Millisecond,
			Cooldown: time.Minute,
		},
		checkpoints: checkStore,
		alerts:      alerts,
		client: &staticHTTPDoer{
			response: &http.Response{StatusCode: 500, Body: http.NoBody},
		},
		now:    func() time.Time { return now },
		jitter: func(time.Duration) time.Duration { return 0 },
	}
	check := config.MonitorHTTPCheckConfig{
		ID:                  "api-health",
		Method:              http.MethodGet,
		URL:                 "https://example.com/health",
		ExpectedStatusCodes: []int{200},
	}

	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck(first) error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck(second) error = %v", err)
	}

	if got := len(alerts.messages); got != 2 {
		t.Fatalf("alert count = %d, want 2", got)
	}
}

func TestHTTPMonitorRunnerExecuteHTTPCheckClearsOldCooldownWhenAlertSendFails(t *testing.T) {
	t.Parallel()

	checkStore := &memoryMonitorCheckpointStore{
		values: map[string]store.MonitorCheckpoint{
			"api-health": {
				CheckID:           "api-health",
				LastSeenCondition: "status:500",
				Fingerprint:       "status:500",
				LastAlertAt:       time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC),
				CooldownUntil:     time.Date(2025, time.January, 1, 12, 10, 0, 0, time.UTC),
			},
		},
	}
	alerts := &stubMonitorAlertSender{err: errors.New("telegram unavailable")}
	runner := &httpMonitorRunner{
		logger: defaultLogger(nil),
		config: config.MonitorConfig{
			Mode:     config.MonitorModeNotifyOnly,
			Timeout:  50 * time.Millisecond,
			Cooldown: time.Minute,
		},
		checkpoints: checkStore,
		alerts:      alerts,
		client: &staticHTTPDoer{
			response: &http.Response{StatusCode: 503, Body: http.NoBody},
		},
		now:    func() time.Time { return time.Date(2025, time.January, 1, 12, 5, 0, 0, time.UTC) },
		jitter: func(time.Duration) time.Duration { return 0 },
	}
	check := config.MonitorHTTPCheckConfig{
		ID:                  "api-health",
		Method:              http.MethodGet,
		URL:                 "https://example.com/health",
		ExpectedStatusCodes: []int{200},
	}

	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck() error = %v", err)
	}

	checkpoint, ok, err := checkStore.GetMonitorCheckpoint(context.Background(), check.ID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint not persisted")
	}
	if checkpoint.LastSeenCondition != "status:503" {
		t.Fatalf("LastSeenCondition = %q, want %q", checkpoint.LastSeenCondition, "status:503")
	}
	if !checkpoint.LastAlertAt.IsZero() {
		t.Fatalf("LastAlertAt = %v, want zero after failed alert send", checkpoint.LastAlertAt)
	}
	if !checkpoint.CooldownUntil.IsZero() {
		t.Fatalf("CooldownUntil = %v, want zero after failed alert send", checkpoint.CooldownUntil)
	}
}

func TestHTTPMonitorRunnerExecuteHTTPCheckUsesPerCheckTimeout(t *testing.T) {
	t.Parallel()

	checkStore := &memoryMonitorCheckpointStore{}
	alerts := &stubMonitorAlertSender{}
	doer := &blockingHTTPDoer{}
	runner := &httpMonitorRunner{
		logger: defaultLogger(nil),
		config: config.MonitorConfig{
			Mode:     config.MonitorModeNotifyOnly,
			Timeout:  20 * time.Millisecond,
			Cooldown: time.Minute,
		},
		checkpoints: checkStore,
		alerts:      alerts,
		client:      doer,
		now:         func() time.Time { return time.Now().UTC() },
		jitter:      func(time.Duration) time.Duration { return 0 },
	}
	check := config.MonitorHTTPCheckConfig{
		ID:                  "slow-health",
		Method:              http.MethodGet,
		URL:                 "https://example.com/slow",
		ExpectedStatusCodes: []int{200},
	}

	start := time.Now()
	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck() error = %v", err)
	}
	elapsed := time.Since(start)

	if !doer.sawCancellation.Load() {
		t.Fatal("expected request context cancellation")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("executeHTTPCheck() took %s, want prompt timeout handling", elapsed)
	}
	if got := len(alerts.messages); got != 1 {
		t.Fatalf("alert count = %d, want 1", got)
	}
	if !strings.Contains(alerts.messages[0], "condition: timeout") {
		t.Fatalf("alert = %q, want timeout condition", alerts.messages[0])
	}
}

func TestHTTPMonitorRunnerNextDelayAddsJitter(t *testing.T) {
	t.Parallel()

	runner := &httpMonitorRunner{
		config: config.MonitorConfig{
			Interval: 10 * time.Second,
			Jitter:   3 * time.Second,
		},
		jitter: func(max time.Duration) time.Duration {
			if max != 3*time.Second {
				t.Fatalf("jitter max = %s, want %s", max, 3*time.Second)
			}
			return 2 * time.Second
		},
	}

	if got, want := runner.nextDelay(), 12*time.Second; got != want {
		t.Fatalf("nextDelay() = %s, want %s", got, want)
	}
}

func TestNewMonitorRunnerRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	_, err := newMonitorRunner(config.Config{
		Monitor: config.MonitorConfig{
			Enabled:    true,
			Mode:       config.MonitorModeAnalyzeThenNotify,
			HTTPChecks: []config.MonitorHTTPCheckConfig{{ID: "api-health", Method: http.MethodGet, URL: "https://example.com/health", ExpectedStatusCodes: []int{200}}},
		},
	}, nil, &memoryMonitorCheckpointStore{}, &stubMonitorAlertSender{})
	if err == nil {
		t.Fatal("newMonitorRunner() error = nil, want unsupported mode error")
	}
}

type staticHTTPDoer struct {
	response *http.Response
	err      error
}

func (d *staticHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return d.response, d.err
}

type blockingHTTPDoer struct {
	sawCancellation atomic.Bool
}

func (d *blockingHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	d.sawCancellation.Store(true)
	return nil, request.Context().Err()
}

type stubMonitorAlertSender struct {
	messages []string
	err      error
	mu       sync.Mutex
}

func (s *stubMonitorAlertSender) SendAlert(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, text)
	return s.err
}

type memoryMonitorCheckpointStore struct {
	values map[string]store.MonitorCheckpoint
	mu     sync.Mutex
}

func (s *memoryMonitorCheckpointStore) GetMonitorCheckpoint(_ context.Context, checkID string) (store.MonitorCheckpoint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		return store.MonitorCheckpoint{}, false, nil
	}
	value, ok := s.values[checkID]
	return value, ok, nil
}

func (s *memoryMonitorCheckpointStore) PutMonitorCheckpoint(_ context.Context, checkpoint store.MonitorCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]store.MonitorCheckpoint)
	}
	if checkpoint.CheckID == "" {
		return errors.New("check id is required")
	}
	s.values[checkpoint.CheckID] = checkpoint
	return nil
}

func (s *memoryMonitorCheckpointStore) ListMonitorCheckpoints(_ context.Context) ([]store.MonitorCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	checkpoints := make([]store.MonitorCheckpoint, 0, len(s.values))
	for _, checkpoint := range s.values {
		checkpoints = append(checkpoints, checkpoint)
	}
	return checkpoints, nil
}
