package app

import (
	"sync"

	"github.com/SCWPretorius/CONTROL/internal/copilot"
	"github.com/SCWPretorius/CONTROL/internal/router"
)

type runtimeStatusTracker struct {
	mu       sync.RWMutex
	snapshot router.RuntimeStatus
}

func newRuntimeStatusTracker() *runtimeStatusTracker {
	return &runtimeStatusTracker{}
}

func (t *runtimeStatusTracker) Observe(event copilot.RuntimeEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !event.OccurredAt.IsZero() {
		t.snapshot.LastEventAt = event.OccurredAt
	}
	t.snapshot.LastEventKind = event.Kind
	t.snapshot.EventCount++
	if event.Err != nil {
		t.snapshot.LastError = event.Err.Error()
	}

	switch event.Kind {
	case "runtime.started":
		t.snapshot.Running = true
		if !event.OccurredAt.IsZero() {
			t.snapshot.StartedAt = event.OccurredAt
		}
	case "runtime.start_failed", "runtime.stopped":
		t.snapshot.Running = false
	case "permission.requested":
		t.snapshot.PermissionRequests++
	case "hook.pre_tool_use":
		t.snapshot.ToolCalls++
	}
}

func (t *runtimeStatusTracker) Snapshot() router.RuntimeStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshot
}
