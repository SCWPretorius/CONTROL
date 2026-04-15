package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace"
	"github.com/SCWPretorius/CONTROL/internal/tools/googleworkspace/gmail"
)

const (
	monitorHealthyCondition              = "healthy"
	monitorMetadataGmailLatestDateMS     = "gmail_latest_internal_date_ms"
	monitorMetadataGmailLatestMessageIDs = "gmail_latest_message_ids"
)

type monitorLifecycle interface {
	Run(context.Context) error
}

type monitorAlertSender interface {
	SendAlert(context.Context, string) error
}

type monitorHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type monitorGmailClient interface {
	ListMessages(context.Context, gmail.ListMessagesRequest) (gmail.ListMessagesResponse, error)
	GetMessage(context.Context, gmail.GetMessageRequest) (gmail.Message, error)
	DownloadAttachment(context.Context, gmail.DownloadAttachmentRequest) ([]byte, error)
}

type compositeMonitorRunner struct {
	runners []monitorLifecycle
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

type gmailMonitorRunner struct {
	logger      *log.Logger
	config      config.MonitorConfig
	checkpoints store.MonitorCheckpointStore
	alerts      monitorAlertSender
	client      monitorGmailClient
	storageDir  string
	now         func() time.Time
	jitter      func(time.Duration) time.Duration
}

type monitorCheckResult struct {
	condition   string
	fingerprint string
	detail      string
}

type gmailMonitorCursor struct {
	latestInternalDateMS int64
	latestMessageIDs     []string
}

type downloadedAttachment struct {
	Filename string
	Path     string
}

type processedGmailMessage struct {
	message     gmail.Message
	attachments []downloadedAttachment
}

func newMonitorRunner(cfg config.Config, logger *log.Logger, checkpoints store.MonitorCheckpointStore, alerts monitorAlertSender) (monitorLifecycle, error) {
	if !cfg.Monitor.Enabled || (len(cfg.Monitor.HTTPChecks) == 0 && len(cfg.Monitor.GmailChecks) == 0) {
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

	now := func() time.Time { return time.Now().UTC() }
	jitter := func(max time.Duration) time.Duration {
		if max <= 0 {
			return 0
		}
		return time.Duration(rand.Int64N(int64(max) + 1))
	}

	runners := make([]monitorLifecycle, 0, 2)
	if len(cfg.Monitor.HTTPChecks) > 0 {
		runners = append(runners, &httpMonitorRunner{
			logger:      defaultLogger(logger),
			config:      cfg.Monitor,
			checkpoints: checkpoints,
			alerts:      alerts,
			client:      &http.Client{},
			now:         now,
			jitter:      jitter,
		})
	}
	if len(cfg.Monitor.GmailChecks) > 0 {
		if !cfg.Tools.Google.RuntimeEnabled() {
			return nil, errors.New("gmail monitoring requires Google OAuth app config and GOOGLE_OAUTH_ACCESS_TOKEN")
		}
		tokenProvider, err := googleworkspace.NewEnvAccessTokenProvider(cfg.Tools.Google)
		if err != nil {
			return nil, fmt.Errorf("gmail monitoring token provider: %w", err)
		}
		gmailClient, err := gmail.NewClient(cfg.Tools.Google.OAuth, tokenProvider, &http.Client{
			Timeout: cfg.Tools.Runtime.HTTPTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("gmail monitoring client: %w", err)
		}
		runners = append(runners, &gmailMonitorRunner{
			logger:      defaultLogger(logger),
			config:      cfg.Monitor,
			checkpoints: checkpoints,
			alerts:      alerts,
			client:      gmailClient,
			storageDir:  cfg.Paths.StorageDir,
			now:         now,
			jitter:      jitter,
		})
	}

	if len(runners) == 1 {
		return runners[0], nil
	}
	return &compositeMonitorRunner{runners: runners}, nil
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

func (r *compositeMonitorRunner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(r.runners))

	for _, runner := range r.runners {
		runner := runner
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
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

func (r *gmailMonitorRunner) nextDelay() time.Duration {
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
	return recordMonitorResult(runCtx, r.logger, r.checkpoints, r.alerts, r.config.Timeout, r.config.Cooldown, check.ID, result, startedAt, nil, func() string {
		return formatHTTPMonitorAlert(check, result, startedAt, r.config.Mode)
	})
}

func (r *gmailMonitorRunner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(r.config.GmailChecks))

	for _, check := range r.config.GmailChecks {
		check := check
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runGmailCheckLoop(ctx, check); err != nil && !errors.Is(err, context.Canceled) {
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

func (r *gmailMonitorRunner) runGmailCheckLoop(ctx context.Context, check config.MonitorGmailCheckConfig) error {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if err := r.executeGmailCheck(ctx, check); err != nil {
				r.logger.Printf("gmail monitor check failed check=%s err=%v", check.ID, err)
			}
			timer.Reset(r.nextDelay())
		}
	}
}

func (r *gmailMonitorRunner) executeGmailCheck(ctx context.Context, check config.MonitorGmailCheckConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	checkpoint, found, err := r.checkpoints.GetMonitorCheckpoint(runCtx, check.ID)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}
	cursor, err := parseGmailMonitorCursor(checkpoint.Metadata)
	if err != nil {
		return fmt.Errorf("parse checkpoint cursor: %w", err)
	}

	listResponse, err := r.client.ListMessages(runCtx, gmail.ListMessagesRequest{
		LabelIDs:         append([]string(nil), check.LabelIDs...),
		MaxResults:       check.MaxResults,
		IncludeSpamTrash: check.IncludeSpamTrash,
	})
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	observedAt := r.now()
	nextCursor := cursor
	var processed []processedGmailMessage

	for index := len(listResponse.Messages) - 1; index >= 0; index-- {
		reference := listResponse.Messages[index]
		message, err := r.client.GetMessage(runCtx, gmail.GetMessageRequest{
			MessageID: reference.ID,
			Format:    gmail.MessageFormatFull,
		})
		if err != nil {
			return fmt.Errorf("get message %q: %w", reference.ID, err)
		}
		if !isNewerThanGmailCursor(message, cursor) {
			continue
		}

		nextCursor = observeGmailCursor(nextCursor, message)
		if !matchesGmailMonitorSubject(check, message.Headers["Subject"]) {
			continue
		}

		attachments, err := r.downloadGmailAttachments(runCtx, check, message)
		if err != nil {
			return err
		}
		processed = append(processed, processedGmailMessage{
			message:     message,
			attachments: attachments,
		})
	}

	metadata := serializeGmailMonitorCursor(nextCursor)
	if !found && len(metadata) == 0 {
		metadata = nil
	}

	if len(processed) == 0 {
		return recordMonitorResult(runCtx, r.logger, r.checkpoints, r.alerts, r.config.Timeout, r.config.Cooldown, check.ID, monitorCheckResult{
			condition: monitorHealthyCondition,
		}, observedAt, metadata, nil)
	}

	result := monitorCheckResult{
		condition:   "matched",
		fingerprint: gmailProcessedFingerprint(processed),
		detail:      gmailProcessedDetail(processed),
	}
	return recordMonitorResult(runCtx, r.logger, r.checkpoints, r.alerts, r.config.Timeout, r.config.Cooldown, check.ID, result, observedAt, metadata, func() string {
		return formatGmailMonitorAlert(check, processed, observedAt, r.config.Mode)
	})
}

func (r *gmailMonitorRunner) downloadGmailAttachments(ctx context.Context, check config.MonitorGmailCheckConfig, message gmail.Message) ([]downloadedAttachment, error) {
	if len(message.Attachments) == 0 {
		return nil, nil
	}

	root := filepath.Join(
		r.storageDir,
		"gmail-attachments",
		"check-"+monitorSafeSegment(check.ID),
		"message-"+monitorSafeSegment(message.ID),
	)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment directory %q: %w", root, err)
	}

	saved := make([]downloadedAttachment, 0, len(message.Attachments))
	for index, attachment := range message.Attachments {
		content, err := r.client.DownloadAttachment(ctx, gmail.DownloadAttachmentRequest{
			MessageID:  message.ID,
			Attachment: attachment,
		})
		if err != nil {
			return nil, fmt.Errorf("download attachment %q for message %q: %w", attachment.Filename, message.ID, err)
		}

		filename := monitorAttachmentFilename(index, attachment.Filename)
		path := filepath.Join(root, filename)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return nil, fmt.Errorf("write attachment %q: %w", path, err)
		}
		saved = append(saved, downloadedAttachment{
			Filename: filename,
			Path:     path,
		})
	}

	return saved, nil
}

func recordMonitorResult(
	ctx context.Context,
	logger *log.Logger,
	checkpoints store.MonitorCheckpointStore,
	alerts monitorAlertSender,
	timeout, cooldown time.Duration,
	checkID string,
	result monitorCheckResult,
	observedAt time.Time,
	metadata map[string]string,
	buildAlert func() string,
) error {
	checkpoint, found, err := checkpoints.GetMonitorCheckpoint(ctx, checkID)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}
	conditionChanged := !found || checkpoint.LastSeenCondition != result.condition || checkpoint.Fingerprint != result.fingerprint

	next := store.MonitorCheckpoint{
		CheckID:           checkID,
		LastSeenCondition: result.condition,
		Fingerprint:       result.fingerprint,
		LastAlertAt:       checkpoint.LastAlertAt,
		CooldownUntil:     checkpoint.CooldownUntil,
		Metadata:          cloneStringMap(metadata),
		UpdatedAt:         observedAt,
	}

	if result.condition == monitorHealthyCondition {
		next.Fingerprint = ""
		next.CooldownUntil = time.Time{}
		if !found ||
			checkpoint.LastSeenCondition != next.LastSeenCondition ||
			checkpoint.Fingerprint != "" ||
			!checkpoint.CooldownUntil.IsZero() ||
			!equalStringMaps(checkpoint.Metadata, next.Metadata) {
			if err := checkpoints.PutMonitorCheckpoint(ctx, next); err != nil {
				return fmt.Errorf("persist healthy checkpoint: %w", err)
			}
		}
		return nil
	}
	if conditionChanged {
		next.LastAlertAt = time.Time{}
		next.CooldownUntil = time.Time{}
	}

	if shouldAlertCheckpoint(checkpoint, found, result, observedAt) && buildAlert != nil {
		alertCtx, cancel := context.WithTimeout(ctx, timeout)
		sendErr := alerts.SendAlert(alertCtx, buildAlert())
		cancel()
		if sendErr != nil {
			logger.Printf("monitor alert failed check=%s err=%v", checkID, sendErr)
		} else {
			next.LastAlertAt = observedAt
			next.CooldownUntil = observedAt.Add(cooldown)
		}
	}

	if err := checkpoints.PutMonitorCheckpoint(ctx, next); err != nil {
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

func formatHTTPMonitorAlert(check config.MonitorHTTPCheckConfig, result monitorCheckResult, observedAt time.Time, mode string) string {
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

func formatGmailMonitorAlert(check config.MonitorGmailCheckConfig, processed []processedGmailMessage, observedAt time.Time, mode string) string {
	lines := []string{
		"CONTROL monitor alert",
		fmt.Sprintf("check: %s", check.ID),
		fmt.Sprintf("mode: %s", mode),
		"source: gmail",
		fmt.Sprintf("condition: matched"),
		fmt.Sprintf("detail: %s", gmailProcessedDetail(processed)),
		fmt.Sprintf("detected_at: %s", observedAt.UTC().Format(time.RFC3339)),
	}
	if len(check.LabelIDs) > 0 {
		lines = append(lines, fmt.Sprintf("labels: %s", strings.Join(check.LabelIDs, ", ")))
	}
	if check.SubjectContains != "" {
		lines = append(lines, fmt.Sprintf("subject_contains: %s", check.SubjectContains))
	}
	if check.SubjectEquals != "" {
		lines = append(lines, fmt.Sprintf("subject_equals: %s", check.SubjectEquals))
	}

	for _, item := range processed {
		subject := strings.TrimSpace(item.message.Headers["Subject"])
		sender := strings.TrimSpace(item.message.Headers["From"])
		if subject == "" {
			subject = "(no subject)"
		}
		if sender == "" {
			sender = "(unknown sender)"
		}
		lines = append(lines, fmt.Sprintf("message: %s | from: %s | attachments: %d", subject, sender, len(item.attachments)))
		for _, attachment := range item.attachments {
			lines = append(lines, fmt.Sprintf("saved: %s -> %s", attachment.Filename, attachment.Path))
		}
	}

	return strings.Join(lines, "\n")
}

func matchesGmailMonitorSubject(check config.MonitorGmailCheckConfig, subject string) bool {
	subject = strings.TrimSpace(subject)
	if check.SubjectEquals != "" {
		return strings.EqualFold(subject, check.SubjectEquals)
	}
	if check.SubjectContains != "" {
		return strings.Contains(strings.ToLower(subject), strings.ToLower(check.SubjectContains))
	}
	return true
}

func parseGmailMonitorCursor(metadata map[string]string) (gmailMonitorCursor, error) {
	var cursor gmailMonitorCursor
	if len(metadata) == 0 {
		return cursor, nil
	}

	if raw := strings.TrimSpace(metadata[monitorMetadataGmailLatestDateMS]); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return gmailMonitorCursor{}, fmt.Errorf("%s: %w", monitorMetadataGmailLatestDateMS, err)
		}
		cursor.latestInternalDateMS = value
	}
	if raw := strings.TrimSpace(metadata[monitorMetadataGmailLatestMessageIDs]); raw != "" {
		cursor.latestMessageIDs = compactCSV(raw)
	}

	return cursor, nil
}

func serializeGmailMonitorCursor(cursor gmailMonitorCursor) map[string]string {
	if cursor.latestInternalDateMS == 0 && len(cursor.latestMessageIDs) == 0 {
		return nil
	}

	metadata := map[string]string{
		monitorMetadataGmailLatestDateMS: strconv.FormatInt(cursor.latestInternalDateMS, 10),
	}
	if len(cursor.latestMessageIDs) > 0 {
		ids := append([]string(nil), cursor.latestMessageIDs...)
		sort.Strings(ids)
		metadata[monitorMetadataGmailLatestMessageIDs] = strings.Join(ids, ",")
	}
	return metadata
}

func observeGmailCursor(cursor gmailMonitorCursor, message gmail.Message) gmailMonitorCursor {
	internalDateMS := message.InternalDate.UnixMilli()
	switch {
	case internalDateMS > cursor.latestInternalDateMS:
		cursor.latestInternalDateMS = internalDateMS
		cursor.latestMessageIDs = []string{message.ID}
	case internalDateMS == cursor.latestInternalDateMS && !slices.Contains(cursor.latestMessageIDs, message.ID):
		cursor.latestMessageIDs = append(cursor.latestMessageIDs, message.ID)
	}
	return cursor
}

func isNewerThanGmailCursor(message gmail.Message, cursor gmailMonitorCursor) bool {
	internalDateMS := message.InternalDate.UnixMilli()
	if internalDateMS > cursor.latestInternalDateMS {
		return true
	}
	if internalDateMS < cursor.latestInternalDateMS {
		return false
	}
	return !slices.Contains(cursor.latestMessageIDs, message.ID)
}

func gmailProcessedFingerprint(processed []processedGmailMessage) string {
	ids := make([]string, 0, len(processed))
	for _, item := range processed {
		ids = append(ids, item.message.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func gmailProcessedDetail(processed []processedGmailMessage) string {
	attachmentCount := 0
	for _, item := range processed {
		attachmentCount += len(item.attachments)
	}
	return fmt.Sprintf("matched %d Gmail message(s); downloaded %d attachment(s)", len(processed), attachmentCount)
}

func monitorSafeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func monitorAttachmentFilename(index int, name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fmt.Sprintf("%02d-attachment.bin", index+1)
	}

	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	sanitized := strings.Trim(builder.String(), "-.")
	if sanitized == "" {
		sanitized = "attachment.bin"
	}
	return fmt.Sprintf("%02d-%s", index+1, sanitized)
}

func compactCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || slices.Contains(values, part) {
			continue
		}
		values = append(values, part)
	}
	return values
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
