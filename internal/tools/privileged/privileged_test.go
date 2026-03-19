package privileged

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SCWPretorius/CONTROL/internal/config"
	"github.com/SCWPretorius/CONTROL/internal/store"
)

func TestWriteFileAllowsWritableRootAndEmitsAudit(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writable := filepath.Join(workspace, "assistant")
	var events []store.PrivilegedToolEvent

	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{writable},
	}, Options{
		AuditHook: func(_ context.Context, event store.PrivilegedToolEvent) {
			events = append(events, event)
		},
	})

	result, err := layer.WriteFile(context.Background(), FileWriteRequest{
		Path:     "notes/today.txt",
		Contents: "done",
	}, InvocationMetadata{SessionID: "session-1", ToolCallID: "call-1"})
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	wantPath := filepath.Join(writable, "notes", "today.txt")
	if result.Path != wantPath {
		t.Fatalf("WriteFile() path = %q, want %q", result.Path, wantPath)
	}

	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", wantPath, err)
	}
	if string(content) != "done" {
		t.Fatalf("ReadFile(%q) = %q, want %q", wantPath, content, "done")
	}

	if len(events) != 2 {
		t.Fatalf("audit events len = %d, want 2", len(events))
	}
	if events[0].EventType != "tool.requested" || events[1].EventType != "tool.completed" {
		t.Fatalf("audit event types = [%s %s], want [tool.requested tool.completed]", events[0].EventType, events[1].EventType)
	}
}

func TestWriteFileDeniesPathOutsideWritableRoots(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writable := filepath.Join(workspace, "assistant")
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{writable},
	}, Options{})

	_, err := layer.WriteFile(context.Background(), FileWriteRequest{
		Path:          "..\\escape.txt",
		BaseDirectory: writable,
		Contents:      "nope",
	}, InvocationMetadata{})
	if err == nil {
		t.Fatal("WriteFile() error = nil, want denial")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("WriteFile() error = %v, want ErrPolicyDenied", err)
	}
}

func TestRunShellAllowsAutoApprovedCommand(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := &stubShellRunner{
		result: ShellExecution{
			ExitCode: 0,
			Output:   "ok",
		},
	}
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{filepath.Join(workspace, "assistant")},
		ShellAutoApprove:       []string{"echo ok"},
	}, Options{ShellRunner: runner})

	result, err := layer.RunShell(context.Background(), ShellRequest{
		Command:          "echo ok",
		WorkingDirectory: workspace,
	}, InvocationMetadata{})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}

	if result.Output != "ok" {
		t.Fatalf("RunShell() output = %q, want %q", result.Output, "ok")
	}
	if runner.command != "echo ok" {
		t.Fatalf("runner command = %q, want %q", runner.command, "echo ok")
	}
	if runner.workingDirectory != workspace {
		t.Fatalf("runner working directory = %q, want %q", runner.workingDirectory, workspace)
	}
}

func TestRunShellDeniesNonAllowlistedCommand(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := &stubShellRunner{}
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{filepath.Join(workspace, "assistant")},
		ShellAutoApprove:       []string{"echo ok"},
	}, Options{ShellRunner: runner})

	_, err := layer.RunShell(context.Background(), ShellRequest{
		Command:          "echo denied",
		WorkingDirectory: workspace,
	}, InvocationMetadata{})
	if err == nil {
		t.Fatal("RunShell() error = nil, want denial")
	}
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("RunShell() error = %v, want ErrApprovalRequired", err)
	}
	if runner.called {
		t.Fatal("runner should not be called for a denied command")
	}
}

func TestRunShellDeniesUnsafeCommandFragments(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runner := &stubShellRunner{}
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{filepath.Join(workspace, "assistant")},
		ShellAutoApprove:       []string{"echo ok"},
	}, Options{ShellRunner: runner})

	_, err := layer.RunShell(context.Background(), ShellRequest{
		Command:          "echo ok && whoami",
		WorkingDirectory: workspace,
	}, InvocationMetadata{})
	if err == nil {
		t.Fatal("RunShell() error = nil, want denial")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("RunShell() error = %v, want ErrPolicyDenied", err)
	}
}

func TestRunShellAppliesOutputLimit(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{filepath.Join(workspace, "assistant")},
		ShellAutoApprove:       []string{"echo large"},
	}, Options{
		ShellRunner: &stubShellRunner{
			result: ShellExecution{
				ExitCode:  0,
				Output:    strings.Repeat("x", 32),
				Truncated: true,
			},
		},
	})

	result, err := layer.RunShell(context.Background(), ShellRequest{
		Command:          "echo large",
		WorkingDirectory: workspace,
	}, InvocationMetadata{})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if !result.Truncated {
		t.Fatal("RunShell() truncated = false, want true")
	}
}

func TestRunShellTimesOut(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	layer := newTestLayer(t, config.PrivilegedToolConfig{
		AllowedWorkspaceRoots:  []string{workspace},
		AssistantWritableRoots: []string{filepath.Join(workspace, "assistant")},
		ShellAutoApprove:       []string{"sleep-now"},
	}, Options{
		ShellRunner: &stubShellRunner{
			waitForContext: true,
			result: ShellExecution{
				ExitCode: -1,
			},
		},
	})
	layer.shellTimeout = 10 * time.Millisecond

	_, err := layer.RunShell(context.Background(), ShellRequest{
		Command:          "sleep-now",
		WorkingDirectory: workspace,
	}, InvocationMetadata{})
	if err == nil {
		t.Fatal("RunShell() error = nil, want timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunShell() error = %v, want deadline exceeded", err)
	}
}

func TestCappedBufferReportsFullWriteLengthWhenTruncating(t *testing.T) {
	t.Parallel()

	buffer := &cappedBuffer{limit: 4}
	written, err := buffer.Write([]byte("abcdef"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != 6 {
		t.Fatalf("Write() written = %d, want 6", written)
	}
	if got := buffer.String(); got != "abcd" {
		t.Fatalf("buffer.String() = %q, want %q", got, "abcd")
	}
	if !buffer.truncated {
		t.Fatal("buffer.truncated = false, want true")
	}
}

func newTestLayer(t *testing.T, privileged config.PrivilegedToolConfig, opts Options) *Layer {
	t.Helper()

	layer, err := NewLayer(privileged, config.ToolRuntimeConfig{
		ShellCommandTimeout:   100 * time.Millisecond,
		MaxCommandOutputBytes: 32,
	}, opts)
	if err != nil {
		t.Fatalf("NewLayer() error = %v", err)
	}
	return layer
}

type stubShellRunner struct {
	result           ShellExecution
	err              error
	called           bool
	command          string
	workingDirectory string
	waitForContext   bool
}

func (s *stubShellRunner) Run(ctx context.Context, workingDirectory, command string, _ int) (ShellExecution, error) {
	s.called = true
	s.command = command
	s.workingDirectory = workingDirectory

	if s.waitForContext {
		<-ctx.Done()
		return s.result, ctx.Err()
	}

	return s.result, s.err
}
