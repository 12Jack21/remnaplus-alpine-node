package xray

import (
	"context"
	"fmt"

	"github.com/12Jack21/remnaplus-alpine-node/internal/xtls"
)

func (m *Manager) routerAPI(ctx context.Context) (*xtls.RouterAPI, func(), error) {
	m.mu.RLock()
	online := m.xrayOnline
	socket := m.xtlsSocket
	m.mu.RUnlock()
	if !online {
		return nil, nil, fmt.Errorf("xray is not online")
	}
	client, err := xtls.NewClient(socket)
	if err != nil {
		return nil, nil, err
	}
	return xtls.NewRouterAPI(client.Conn()), func() { _ = client.Close() }, nil
}

func (m *Manager) RouterAddSourceIPRule(ctx context.Context, ruleTag, outbound, ip string, appendRule bool) xtls.RouterResult {
	api, closeFn, err := m.routerAPI(ctx)
	if err != nil {
		return xtls.RouterResult{Message: err.Error()}
	}
	defer closeFn()
	return api.AddSourceIPRule(ctx, ruleTag, outbound, ip, appendRule)
}

func (m *Manager) RouterRemoveRuleByTag(ctx context.Context, ruleTag string) xtls.RouterResult {
	api, closeFn, err := m.routerAPI(ctx)
	if err != nil {
		return xtls.RouterResult{Message: err.Error()}
	}
	defer closeFn()
	return api.RemoveRuleByTag(ctx, ruleTag)
}
