package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

const monitorHealthyCondition = "healthy"

type monitorLifecycle interface {
	Run(context.Context) error
}

type monitorAlertSender interface {
	SendAlert(context.Context, string) error
}

type monitorHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type httpMonitorRunner struct {
	logger      *log.Logger
	config      config.MonitorConfig
	checkpoints store.MonitorCheckpointStore
	alerts      monitorAlertSender
	client      monitorHTTPDoer
	now         func() time.Time
	jitter      func(time.Duration) time.Duration
}

type monitorCheckResult struct {
	condition   string
	fingerprint string
	detail      string
}

func newMonitorRunner(cfg config.Config, logger *log.Logger, checkpoints store.MonitorCheckpointStore, alerts monitorAlertSender) (monitorLifecycle, error) {
	if !cfg.Monitor.Enabled || len(cfg.Monitor.HTTPChecks) == 0 {
		return nil, nil
	}
	if cfg.Monitor.Mode != config.MonitorModeNotifyOnly {
		return nil, fmt.Errorf("monitor mode %q is not implemented yet", cfg.Monitor.Mode)
	}
	if checkpoints == nil {
		return nil, errors.New("monitor checkpoint store is required")
	}
	if alerts == nil {
		return nil, errors.New("monitor alert sender is required")
	}

	return &httpMonitorRunner{
		logger:      defaultLogger(logger),
		config:      cfg.Monitor,
		checkpoints: checkpoints,
		alerts:      alerts,
		client:      &http.Client{},
		now:         func() time.Time { return time.Now().UTC() },
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(max) + 1))
		},
	}, nil
}

func startMonitorLifecycle(ctx context.Context, runner monitorLifecycle) func() error {
	if runner == nil {
		return func() error { return nil }
	}

	done := make(chan error, 1)
	go func() {
		err := runner.Run(ctx)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		done <- err
	}()

	return func() error {
		return <-done
	}
}

func (r *httpMonitorRunner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(r.config.HTTPChecks))

	for _, check := range r.config.HTTPChecks {
		check := check
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runHTTPCheckLoop(ctx, check); err != nil && !errors.Is(err, context.Canceled) {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	var runErr error
	for err := range errs {
		runErr = errors.Join(runErr, err)
	}
	return runErr
}

func (r *httpMonitorRunner) runHTTPCheckLoop(ctx context.Context, check config.MonitorHTTPCheckConfig) error {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if err := r.executeHTTPCheck(ctx, check); err != nil {
				r.logger.Printf("monitor check failed check=%s method=%s url=%s err=%v", check.ID, check.Method, check.URL, err)
			}
			timer.Reset(r.nextDelay())
		}
	}
}

func (r *httpMonitorRunner) nextDelay() time.Duration {
	return r.config.Interval + r.jitter(r.config.Jitter)
}

func (r *httpMonitorRunner) executeHTTPCheck(ctx context.Context, check config.MonitorHTTPCheckConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(runCtx, check.Method, check.URL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for name, value := range check.Headers {
		request.Header.Set(name, value)
	}

	startedAt := r.now()
	response, err := r.client.Do(request)
	if response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}()
	}

	if err != nil && ctx.Err() != nil {
		return nil
	}

	result := evaluateHTTPCheckResult(check, response, err)
	return r.recordHTTPResult(ctx, check, result, startedAt)
}

func (r *httpMonitorRunner) recordHTTPResult(ctx context.Context, check config.MonitorHTTPCheckConfig, result monitorCheckResult, observedAt time.Time) error {
	checkpoint, found, err := r.checkpoints.GetMonitorCheckpoint(ctx, check.ID)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}
	conditionChanged := !found || checkpoint.LastSeenCondition != result.condition || checkpoint.Fingerprint != result.fingerprint

	next := store.MonitorCheckpoint{
		CheckID:           check.ID,
		LastSeenCondition: result.condition,
		Fingerprint:       result.fingerprint,
		LastAlertAt:       checkpoint.LastAlertAt,
		CooldownUntil:     checkpoint.CooldownUntil,
		UpdatedAt:         observedAt,
	}

	if result.condition == monitorHealthyCondition {
		next.Fingerprint = ""
		next.CooldownUntil = time.Time{}
		if !found || checkpoint.LastSeenCondition != next.LastSeenCondition || checkpoint.Fingerprint != "" || !checkpoint.CooldownUntil.IsZero() {
			if err := r.checkpoints.PutMonitorCheckpoint(ctx, next); err != nil {
				return fmt.Errorf("persist healthy checkpoint: %w", err)
			}
		}
		return nil
	}
	if conditionChanged {
		next.LastAlertAt = time.Time{}
		next.CooldownUntil = time.Time{}
	}

	if shouldAlertCheckpoint(checkpoint, found, result, observedAt) {
		alertCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
		sendErr := r.alerts.SendAlert(alertCtx, formatMonitorAlert(check, result, observedAt, r.config.Mode))
		cancel()
		if sendErr != nil {
			r.logger.Printf("monitor alert failed check=%s err=%v", check.ID, sendErr)
		} else {
			next.LastAlertAt = observedAt
			next.CooldownUntil = observedAt.Add(r.config.Cooldown)
		}
	}

	if err := r.checkpoints.PutMonitorCheckpoint(ctx, next); err != nil {
		return fmt.Errorf("persist unhealthy checkpoint: %w", err)
	}
	return nil
}

func shouldAlertCheckpoint(checkpoint store.MonitorCheckpoint, found bool, result monitorCheckResult, now time.Time) bool {
	if !found {
		return true
	}
	if checkpoint.LastSeenCondition != result.condition || checkpoint.Fingerprint != result.fingerprint {
		return true
	}
	if checkpoint.LastAlertAt.IsZero() {
		return true
	}
	return !now.Before(checkpoint.CooldownUntil)
}

func evaluateHTTPCheckResult(check config.MonitorHTTPCheckConfig, response *http.Response, err error) monitorCheckResult {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return monitorCheckResult{
				condition:   "timeout",
				fingerprint: "timeout",
				detail:      fmt.Sprintf("request timed out after %s", err),
			}
		}
		return monitorCheckResult{
			condition:   "request_error",
			fingerprint: "request_error",
			detail:      err.Error(),
		}
	}

	if response == nil {
		return monitorCheckResult{
			condition:   "request_error",
			fingerprint: "request_error",
			detail:      "request returned no response",
		}
	}

	if slices.Contains(check.ExpectedStatusCodes, response.StatusCode) {
		return monitorCheckResult{condition: monitorHealthyCondition}
	}

	return monitorCheckResult{
		condition:   fmt.Sprintf("status:%d", response.StatusCode),
		fingerprint: fmt.Sprintf("status:%d", response.StatusCode),
		detail:      fmt.Sprintf("received HTTP %d; expected one of %v", response.StatusCode, check.ExpectedStatusCodes),
	}
}

func formatMonitorAlert(check config.MonitorHTTPCheckConfig, result monitorCheckResult, observedAt time.Time, mode string) string {
	return strings.Join([]string{
		"CONTROL monitor alert",
		fmt.Sprintf("check: %s", check.ID),
		fmt.Sprintf("mode: %s", mode),
		fmt.Sprintf("method: %s", check.Method),
		fmt.Sprintf("url: %s", check.URL),
		fmt.Sprintf("condition: %s", result.condition),
		fmt.Sprintf("detail: %s", result.detail),
		fmt.Sprintf("detected_at: %s", observedAt.UTC().Format(time.RFC3339)),
	}, "\n")
}
