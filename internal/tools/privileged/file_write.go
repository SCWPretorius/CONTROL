package privileged

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteRequest describes a local privileged file write.
type FileWriteRequest struct {
	Path          string `json:"path" jsonschema:"Path to write, absolute or relative to baseDirectory"`
	BaseDirectory string `json:"baseDirectory,omitempty" jsonschema:"Optional base directory for relative paths; defaults to the first writable root"`
	Contents      string `json:"contents" jsonschema:"Complete file contents to write"`
}

// FileWriteResult summarizes a successful file write.
type FileWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytesWritten"`
}

// WriteFile writes a file only when the target remains inside writable roots.
func (l *Layer) WriteFile(ctx context.Context, request FileWriteRequest, meta InvocationMetadata) (FileWriteResult, error) {
	l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameFileWrite, "tool.requested", "requested", "local privileged file write requested", map[string]string{
		"path": request.Path,
	}))

	target, err := l.resolveWritePath(request.Path, request.BaseDirectory)
	if err != nil {
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameFileWrite, "tool.denied", "denied", err.Error(), map[string]string{
			"path": request.Path,
		}))
		return FileWriteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		wrapped := fmt.Errorf("create parent directory for %q: %w", target, err)
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameFileWrite, "tool.failed", "failed", wrapped.Error(), map[string]string{
			"path": target,
		}))
		return FileWriteResult{}, wrapped
	}

	if err := os.WriteFile(target, []byte(request.Contents), 0o600); err != nil {
		wrapped := fmt.Errorf("write file %q: %w", target, err)
		l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameFileWrite, "tool.failed", "failed", wrapped.Error(), map[string]string{
			"path": target,
		}))
		return FileWriteResult{}, wrapped
	}

	result := FileWriteResult{
		Path:         target,
		BytesWritten: len([]byte(request.Contents)),
	}
	l.emitAudit(ctx, l.newAuditEvent(meta, ToolNameFileWrite, "tool.completed", "completed", fmt.Sprintf("wrote %d bytes", result.BytesWritten), map[string]string{
		"path":          target,
		"bytes_written": fmt.Sprintf("%d", result.BytesWritten),
	}))

	return result, nil
}
