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

func TestHTTPMonitorRunnerExecuteHTTPCheckUsesConfiguredMethodAndHeaders(t *testing.T) {
	t.Parallel()

	checkStore := &memoryMonitorCheckpointStore{}
	alerts := &stubMonitorAlertSender{}
	doer := &recordingHTTPDoer{
		response: &http.Response{StatusCode: 200, Body: http.NoBody},
	}
	runner := &httpMonitorRunner{
		logger: defaultLogger(nil),
		config: config.MonitorConfig{
			Mode:     config.MonitorModeNotifyOnly,
			Timeout:  50 * time.Millisecond,
			Cooldown: time.Minute,
		},
		checkpoints: checkStore,
		alerts:      alerts,
		client:      doer,
		now:         func() time.Time { return time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC) },
		jitter:      func(time.Duration) time.Duration { return 0 },
	}
	check := config.MonitorHTTPCheckConfig{
		ID:     "api-health",
		Method: http.MethodPost,
		URL:    "https://example.com/health",
		Headers: map[string]string{
			"Authorization": "Bearer token",
			"X-Check-ID":    "api-health",
		},
		ExpectedStatusCodes: []int{200},
	}

	if err := runner.executeHTTPCheck(context.Background(), check); err != nil {
		t.Fatalf("executeHTTPCheck() error = %v", err)
	}

	if doer.request == nil {
		t.Fatal("expected request to be recorded")
	}
	if got, want := doer.request.Method, http.MethodPost; got != want {
		t.Fatalf("request method = %q, want %q", got, want)
	}
	if got, want := doer.request.Header.Get("Authorization"), "Bearer token"; got != want {
		t.Fatalf("Authorization header = %q, want %q", got, want)
	}
	if got, want := doer.request.Header.Get("X-Check-ID"), "api-health"; got != want {
		t.Fatalf("X-Check-ID header = %q, want %q", got, want)
	}
	if got := len(alerts.messages); got != 0 {
		t.Fatalf("alert count = %d, want 0 for healthy response", got)
	}

	checkpoint, ok, err := checkStore.GetMonitorCheckpoint(context.Background(), check.ID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint not persisted")
	}
	if got, want := checkpoint.LastSeenCondition, monitorHealthyCondition; got != want {
		t.Fatalf("LastSeenCondition = %q, want %q", got, want)
	}
	if checkpoint.CooldownUntil != (time.Time{}) {
		t.Fatalf("CooldownUntil = %v, want zero for healthy checkpoint", checkpoint.CooldownUntil)
	}
}

func TestHTTPMonitorRunnerExecuteHTTPCheckClearsCooldownWhenCheckRecovers(t *testing.T) {
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
	alerts := &stubMonitorAlertSender{}
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
			response: &http.Response{StatusCode: 200, Body: http.NoBody},
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

	if got := len(alerts.messages); got != 0 {
		t.Fatalf("alert count = %d, want 0 after recovery", got)
	}

	checkpoint, ok, err := checkStore.GetMonitorCheckpoint(context.Background(), check.ID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint not persisted")
	}
	if got, want := checkpoint.LastSeenCondition, monitorHealthyCondition; got != want {
		t.Fatalf("LastSeenCondition = %q, want %q", got, want)
	}
	if got := checkpoint.Fingerprint; got != "" {
		t.Fatalf("Fingerprint = %q, want empty after recovery", got)
	}
	if checkpoint.CooldownUntil != (time.Time{}) {
		t.Fatalf("CooldownUntil = %v, want zero after recovery", checkpoint.CooldownUntil)
	}
}

func TestHTTPMonitorRunnerExecuteHTTPCheckAlertsImmediatelyWhenConditionChanges(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2025, time.January, 1, 12, 5, 0, 0, time.UTC)
	checkStore := &memoryMonitorCheckpointStore{
		values: map[string]store.MonitorCheckpoint{
			"api-health": {
				CheckID:           "api-health",
				LastSeenCondition: "status:500",
				Fingerprint:       "status:500",
				LastAlertAt:       observedAt.Add(-5 * time.Minute),
				CooldownUntil:     observedAt.Add(5 * time.Minute),
			},
		},
	}
	alerts := &stubMonitorAlertSender{}
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
		now:    func() time.Time { return observedAt },
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

	if got := len(alerts.messages); got != 1 {
		t.Fatalf("alert count = %d, want 1", got)
	}
	if !strings.Contains(alerts.messages[0], "condition: status:503") {
		t.Fatalf("alert = %q, want updated status fingerprint", alerts.messages[0])
	}

	checkpoint, ok, err := checkStore.GetMonitorCheckpoint(context.Background(), check.ID)
	if err != nil {
		t.Fatalf("GetMonitorCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint not persisted")
	}
	if got, want := checkpoint.LastSeenCondition, "status:503"; got != want {
		t.Fatalf("LastSeenCondition = %q, want %q", got, want)
	}
	if got, want := checkpoint.Fingerprint, "status:503"; got != want {
		t.Fatalf("Fingerprint = %q, want %q", got, want)
	}
	if got, want := checkpoint.LastAlertAt, observedAt; !got.Equal(want) {
		t.Fatalf("LastAlertAt = %v, want %v", got, want)
	}
	if got, want := checkpoint.CooldownUntil, observedAt.Add(time.Minute); !got.Equal(want) {
		t.Fatalf("CooldownUntil = %v, want %v", got, want)
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

type recordingHTTPDoer struct {
	request  *http.Request
	response *http.Response
	err      error
}

func (d *recordingHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.request = request.Clone(request.Context())
	return d.response, d.err
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
