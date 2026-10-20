package xray

import (
	"context"
	"testing"
)

func TestRunningConfigDoesNotExposePendingOrStoppedSnapshot(t *testing.T) {
	m := &Manager{runningConfigJSON: `{"secret":"synthetic"}`, runningConfigCapturedAt: "2026-10-20T00:00:00Z"}
	m.lifecycleMu.Lock()
	busy := m.RunningConfig(context.Background())
	m.lifecycleMu.Unlock()
	if busy.State != "busy" || busy.ConfigJSON != nil {
		t.Fatal("busy lifecycle exposed configuration")
	}
	stopped := m.RunningConfig(context.Background())
	if stopped.State != "stopped" || stopped.ConfigJSON != nil {
		t.Fatal("stopped core exposed configuration")
	}
	m.mu.Lock()
	_ = m.stopProcessLocked(false)
	m.mu.Unlock()
	if m.runningConfigJSON != "" || m.runningConfigCapturedAt != "" {
		t.Fatal("stop retained an active snapshot")
	}
}
