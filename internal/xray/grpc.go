package xray

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/xtls"
)

func (m *Manager) statsAPI(ctx context.Context, requireOnline bool) (*xtls.StatsAPI, func(), error) {
	m.mu.RLock()
	online := m.xrayOnline
	socket := m.xtlsSocket
	m.mu.RUnlock()

	if requireOnline && !online {
		return nil, nil, fmt.Errorf("xray is not online")
	}

	client, err := xtls.NewClient(socket)
	if err != nil {
		return nil, nil, err
	}

	api := xtls.NewStatsAPI(client.Conn())
	return api, func() { _ = client.Close() }, nil
}

func (m *Manager) PingXrayGRPC(ctx context.Context) bool {
	api, closeFn, err := m.statsAPI(ctx, false)
	if err != nil {
		return false
	}
	defer closeFn()
	return api.Ping(ctx) == nil
}

func (m *Manager) GetSysStats(ctx context.Context) (*xtls.SysStats, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetSysStats(ctx)
}

func (m *Manager) GetAllUsersStats(ctx context.Context, reset bool) ([]xtls.UserTraffic, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetAllUsersStats(ctx, reset)
}

func (m *Manager) ReadAccountingCounters(ctx context.Context) (map[string]int64, string, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, "", err
	}
	defer closeFn()
	result, err := api.GetAccountingCounters(ctx)
	if err != nil {
		return nil, "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	generation := "xray-process-unknown"
	if m.process != nil && m.process.cmd != nil && m.process.cmd.Process != nil {
		generation = fmt.Sprintf("xray-pid-%d", m.process.cmd.Process.Pid)
	}
	return result.Counters, generation, nil
}

func (m *Manager) GetUserOnlineStatus(ctx context.Context, username string) (bool, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return false, err
	}
	defer closeFn()
	return api.GetUserOnlineStatus(ctx, username)
}

func (m *Manager) GetInboundStats(ctx context.Context, tag string, reset bool) (xtls.TagTraffic, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return xtls.TagTraffic{}, err
	}
	defer closeFn()
	return api.GetInboundStats(ctx, tag, reset)
}

func (m *Manager) GetOutboundStats(ctx context.Context, tag string, reset bool) (xtls.TagTraffic, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return xtls.TagTraffic{}, err
	}
	defer closeFn()
	return api.GetOutboundStats(ctx, tag, reset)
}

func (m *Manager) GetAllInboundsStats(ctx context.Context, reset bool) ([]xtls.TagTraffic, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetAllInboundsStats(ctx, reset)
}

func (m *Manager) GetAllOutboundsStats(ctx context.Context, reset bool) ([]xtls.TagTraffic, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetAllOutboundsStats(ctx, reset)
}

func (m *Manager) GetUserIPList(ctx context.Context, userID string, reset bool) ([]xtls.IPEntry, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetUserIPList(ctx, userID, reset)
}

func (m *Manager) GetUsersIPList(ctx context.Context) ([]xtls.UserIPEntry, error) {
	api, closeFn, err := m.statsAPI(ctx, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return api.GetUsersIPList(ctx)
}

func (m *Manager) waitForGRPC(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hint := m.rwCoreExitHint(); hint != "" && strings.Contains(hint, "exited") {
			return false
		}
		if m.PingXrayGRPC(ctx) {
			return true
		}
		// Align official @remnawave/node pRetry: 2s between gRPC readiness attempts.
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
	return false
}
