package privileged

import (
	"context"

	sdk "github.com/github/copilot-sdk/go"
)

// FileWriteTool exposes the local privileged file-write handler as an SDK tool.
func (l *Layer) FileWriteTool() sdk.Tool {
	return sdk.DefineTool(ToolNameFileWrite,
		"Write a complete file inside assistant-owned writable roots. Relative paths default to the first writable root; out-of-policy paths are denied.",
		func(request FileWriteRequest, invocation sdk.ToolInvocation) (any, error) {
			result, err := l.WriteFile(context.Background(), request, InvocationMetadata{
				SessionID:  invocation.SessionID,
				ToolCallID: invocation.ToolCallID,
			})
			if err != nil {
				return toolErrorResult(err, ""), nil
			}
			return result, nil
		},
	)
}

// ShellTool exposes the local privileged shell handler as an SDK tool.
func (l *Layer) ShellTool() sdk.Tool {
	return sdk.DefineTool(ToolNameShell,
		"Run a narrowly auto-approved shell command inside an allowed workspace root. Commands outside the allowlist or containing shell operators are denied.",
		func(request ShellRequest, invocation sdk.ToolInvocation) (any, error) {
			result, err := l.RunShell(context.Background(), request, InvocationMetadata{
				SessionID:  invocation.SessionID,
				ToolCallID: invocation.ToolCallID,
			})
			if err != nil {
				return toolErrorResult(err, formatShellResult(result)), nil
			}
			return result, nil
		},
	)
}
