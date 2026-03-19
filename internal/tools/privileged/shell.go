package privileged

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ShellRequest describes a local privileged shell execution.
type ShellRequest struct {
	Command          string `json:"command" jsonschema:"Shell command text to execute; it must match the narrow auto-approval allowlist"`
	WorkingDirectory string `json:"workingDirectory,omitempty" jsonschema:"Optional working directory; defaults to the first allowed workspace root"`
}

// ShellResult summarizes a shell execution attempt.
type ShellResult struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"workingDirectory"`
	ExitCode         int    `json:"exitCode"`
	Output           string `json:"output,omitempty"`
	Truncated        bool   `json:"truncated,omitempty"`
}

// ShellExecution is the runner-facing shell response.
type ShellExecution struct {
	ExitCode  int
	Output    string
	Truncated bool
}

// RunShell executes an auto-approved shell command inside an allowed workspace root.
func (l *Layer) RunShell(ctx context.Context, request ShellRequest, meta InvocationMetadata) (ShellResult, error) {
	command := strings.TrimSpace(request.Command)
	l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.requested", "requested", "local privileged shell requested", map[string]string{
		"command": command,
		"cwd":     request.WorkingDirectory,
	}))

	if err := commandIsSafe(command); err != nil {
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.denied", "denied", err.Error(), map[string]string{
			"command": command,
		}))
		return ShellResult{Command: command}, err
	}

	workingDirectory, err := l.resolveWorkspaceDirectory(request.WorkingDirectory)
	if err != nil {
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.denied", "denied", err.Error(), map[string]string{
			"command": command,
			"cwd":     request.WorkingDirectory,
		}))
		return ShellResult{Command: command}, err
	}

	result := ShellResult{
		Command:          command,
		WorkingDirectory: workingDirectory,
		ExitCode:         -1,
	}

	if !l.commandIsAutoApproved(command) {
		err := approvalRequiredErrorf("shell command %q is not in the auto-approval allowlist", command)
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.denied", "approval-required", err.Error(), map[string]string{
			"command": command,
			"cwd":     workingDirectory,
		}))
		return result, err
	}

	runContext, cancel := context.WithTimeout(ctx, l.shellTimeout)
	defer cancel()

	execution, err := l.shellRunner.Run(runContext, workingDirectory, command, l.maxOutputBytes)
	result.ExitCode = execution.ExitCode
	result.Output = execution.Output
	result.Truncated = execution.Truncated
	if err != nil {
		outcome := "failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(runContext.Err(), context.DeadlineExceeded) {
			outcome = "timed_out"
		}

		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.failed", outcome, err.Error(), map[string]string{
			"command":          command,
			"cwd":              workingDirectory,
			"exit_code":        fmt.Sprintf("%d", result.ExitCode),
			"output_truncated": fmt.Sprintf("%t", result.Truncated),
		}))
		return result, fmt.Errorf("run shell command %q: %w", command, err)
	}

	l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameShell, "tool.completed", "completed", "shell command completed", map[string]string{
		"command":          command,
		"cwd":              workingDirectory,
		"exit_code":        fmt.Sprintf("%d", result.ExitCode),
		"output_truncated": fmt.Sprintf("%t", result.Truncated),
	}))

	return result, nil
}

type execShellRunner struct{}

func (execShellRunner) Run(ctx context.Context, workingDirectory, command string, maxOutputBytes int) (ShellExecution, error) {
	cmd := newShellCommand(ctx, command)
	cmd.Dir = workingDirectory

	var output cappedBuffer
	output.limit = maxOutputBytes
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	result := ShellExecution{
		ExitCode:  exitCode(err),
		Output:    output.String(),
		Truncated: output.truncated,
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}
		return result, err
	}

	return result, nil
}

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	}

	return exec.CommandContext(ctx, "/bin/sh", "-lc", command)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}

type cappedBuffer struct {
	limit     int
	truncated bool
	buffer    bytes.Buffer
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if b.limit <= 0 {
		b.truncated = true
		return originalLength, nil
	}

	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return originalLength, nil
	}

	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}

	if _, err := b.buffer.Write(data); err != nil {
		return 0, err
	}

	return originalLength, nil
}

func (b *cappedBuffer) String() string {
	return b.buffer.String()
}
