package privileged

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/github/copilot-sdk/go"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

const (
	// ToolNameFileWrite is the local privileged file-write tool name.
	ToolNameFileWrite = "local_file_write"
	// ToolNameShell is the local privileged shell tool name.
	ToolNameShell = "local_shell"
)

var (
	// ErrPolicyDenied indicates the requested operation is outside the local
	// privileged policy boundary.
	ErrPolicyDenied = errors.New("operation denied by privileged tool policy")
	// ErrApprovalRequired indicates the action would need an approval path that
	// is not available in the current application.
	ErrApprovalRequired = errors.New("operation requires approval but no interactive approval path is configured")
)

// AuditHook receives best-effort privileged tool audit events.
type AuditHook func(context.Context, store.PrivilegedToolEvent)

// ShellRunner executes an approved shell command.
type ShellRunner interface {
	Run(ctx context.Context, workingDirectory, command string, maxOutputBytes int) (ShellExecution, error)
}

// Options configures optional layer dependencies.
type Options struct {
	AuditStore  store.PrivilegedToolEventStore
	AuditHook   AuditHook
	ShellRunner ShellRunner
	Now         func() time.Time
}

// InvocationMetadata carries lightweight audit context from a tool invocation.
type InvocationMetadata struct {
	SessionID  string
	ToolCallID string
}

// Layer owns the privileged local tool policy and implementations.
type Layer struct {
	workspaceRoots []string
	writableRoots  []string
	shellAllowlist []string

	shellTimeout   time.Duration
	maxOutputBytes int

	auditStore  store.PrivilegedToolEventStore
	auditHook   AuditHook
	shellRunner ShellRunner
	now         func() time.Time
}

// NewLayer constructs the local privileged tool layer from existing config.
func NewLayer(privileged config.PrivilegedToolConfig, runtime config.ToolRuntimeConfig, opts Options) (*Layer, error) {
	workspaceRoots, err := normalizeRoots(privileged.AllowedWorkspaceRoots)
	if err != nil {
		return nil, fmt.Errorf("normalize allowed workspace roots: %w", err)
	}
	if len(workspaceRoots) == 0 {
		return nil, errors.New("at least one allowed workspace root is required")
	}

	writableRoots, err := normalizeRoots(privileged.AssistantWritableRoots)
	if err != nil {
		return nil, fmt.Errorf("normalize assistant writable roots: %w", err)
	}
	if len(writableRoots) == 0 {
		return nil, errors.New("at least one assistant writable root is required")
	}

	if runtime.ShellCommandTimeout <= 0 {
		return nil, errors.New("shell command timeout must be greater than zero")
	}
	if runtime.MaxCommandOutputBytes <= 0 {
		return nil, errors.New("max command output bytes must be greater than zero")
	}

	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.ShellRunner == nil {
		opts.ShellRunner = execShellRunner{}
	}

	return &Layer{
		workspaceRoots: workspaceRoots,
		writableRoots:  writableRoots,
		shellAllowlist: append([]string(nil), privileged.ShellAutoApprove...),
		shellTimeout:   runtime.ShellCommandTimeout,
		maxOutputBytes: runtime.MaxCommandOutputBytes,
		auditStore:     opts.AuditStore,
		auditHook:      opts.AuditHook,
		shellRunner:    opts.ShellRunner,
		now:            opts.Now,
	}, nil
}

// Tools exposes the local privileged tools without wiring them into the runtime.
func (l *Layer) Tools() []sdk.Tool {
	return []sdk.Tool{
		l.FileWriteTool(),
		l.ShellTool(),
	}
}

func (l *Layer) emitAudit(ctx context.Context, event store.PrivilegedToolEvent) {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = l.now()
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}

	if l.auditHook != nil {
		l.auditHook(ctx, event)
	}
	if l.auditStore != nil {
		_ = l.auditStore.Append(ctx, event)
	}
}

func (l *Layer) newAuditEvent(meta InvocationMetadata, toolName, eventType, outcome, summary string, metadata map[string]string) store.PrivilegedToolEvent {
	cloned := cloneMetadata(metadata)
	if meta.ToolCallID != "" {
		cloned["tool_call_id"] = meta.ToolCallID
	}

	return store.PrivilegedToolEvent{
		SessionID:  meta.SessionID,
		ToolName:   toolName,
		EventType:  eventType,
		Outcome:    outcome,
		Summary:    summary,
		OccurredAt: l.now(),
		Metadata:   cloned,
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func normalizeRoots(roots []string) ([]string, error) {
	normalized := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		resolved, err := resolvePathAllowMissing(root)
		if err != nil {
			return nil, err
		}

		key := comparablePath(resolved)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		normalized = append(normalized, resolved)
	}

	return normalized, nil
}

func deniedErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPolicyDenied, fmt.Sprintf(format, args...))
}

func approvalRequiredErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrApprovalRequired, fmt.Sprintf(format, args...))
}

func toolErrorResult(err error, details string) sdk.ToolResult {
	text := err.Error()
	if details != "" {
		text += "\n" + details
	}

	return sdk.ToolResult{
		TextResultForLLM: text,
		ResultType:       "error",
		Error:            err.Error(),
	}
}

func formatShellResult(result ShellResult) string {
	details := []string{
		fmt.Sprintf("working_directory=%s", result.WorkingDirectory),
		fmt.Sprintf("exit_code=%d", result.ExitCode),
	}
	if result.Truncated {
		details = append(details, "output_truncated=true")
	}
	if result.Output != "" {
		details = append(details, "output:\n"+result.Output)
	}
	return strings.Join(details, "\n")
}
