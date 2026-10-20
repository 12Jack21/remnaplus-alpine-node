package xray

import "context"

// RunningConfigResponse contains the exact startup JSON supplied to the current
// core, not a newly rendered Panel configuration. Hot API mutations have no
// canonical JSON representation and do not replace this original startup input.
type RunningConfigResponse struct {
	State      string  `json:"state"`
	ConfigJSON *string `json:"configJson"`
	CapturedAt *string `json:"capturedAt"`
}

func (m *Manager) RunningConfig(ctx context.Context) RunningConfigResponse {
	if !m.lifecycleMu.TryLock() {
		return RunningConfigResponse{State: "busy"}
	}
	defer m.lifecycleMu.Unlock()
	m.mu.RLock()
	online := m.xrayOnline
	snapshot := m.runningConfigJSON
	capturedAt := m.runningConfigCapturedAt
	m.mu.RUnlock()
	if !online || !m.PingXrayGRPC(ctx) {
		return RunningConfigResponse{State: "stopped"}
	}
	if snapshot == "" || capturedAt == "" {
		return RunningConfigResponse{State: "unavailable"}
	}
	return RunningConfigResponse{State: "running", ConfigJSON: &snapshot, CapturedAt: &capturedAt}
}
