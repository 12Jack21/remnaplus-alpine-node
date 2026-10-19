package xray

import (
	"os"
	"strconv"

	"github.com/12Jack21/remnaplus-alpine-node/internal/netadmin"
)

const (
	apiTag                    = "REMNAWAVE_API"
	apiInboundTag             = "REMNAWAVE_API_INBOUND"
	torrentBlockerOutboundTag = "RW_TB_OUTBOUND_BLOCK"
)

type TorrentBlockerOptions struct {
	Enabled         bool
	IncludeRuleTags []string
	SocketPath      string
	RESTToken       string
}

func generateAPIConfig(input map[string]any, xtlsSocket string, torrent TorrentBlockerOptions, configuredProbePort ...int) map[string]any {
	result := cloneMap(input)
	outbounds, hasWarp, err := guardedOutbounds(arrayFrom(result["outbounds"]))
	if err != nil {
		// The start path reports configuration errors through the normal Xray
		// response; keep this helper panic-free for callers that only inspect
		// generated configuration in tests.
		result["__warpGuardError"] = err.Error()
		outbounds = arrayFrom(result["outbounds"])
	}

	result["stats"] = map[string]any{}
	result["api"] = map[string]any{
		"services": []any{"HandlerService", "StatsService", "RoutingService"},
		"tag":      apiTag,
	}
	inbounds := append([]any{apiInbound(xtlsSocket)}, arrayFrom(result["inbounds"])...)
	if hasWarp {
		probePort := 61001
		if value := os.Getenv("WARP_PROBE_PORT"); value != "" {
			if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed > 0 && parsed <= 65535 {
				probePort = parsed
			}
		}
		if len(configuredProbePort) > 0 && configuredProbePort[0] > 0 && configuredProbePort[0] <= 65535 {
			probePort = configuredProbePort[0]
		}
		inbounds = append([]any{inbounds[0], map[string]any{
			"tag":      "REMNAWAVE_WARP_PROBE_INBOUND",
			"listen":   "127.0.0.1",
			"port":     probePort,
			"protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": false},
		}}, inbounds[1:]...)
	}
	result["inbounds"] = inbounds
	result["outbounds"] = outbounds
	result["policy"] = policyFrom(result["policy"], netadmin.HasCapNetAdmin())
	result["routing"] = routingFrom(result["routing"])
	if hasWarp {
		routing, _ := result["routing"].(map[string]any)
		rules := arrayFrom(routing["rules"])
		probeRule := map[string]any{
			"inboundTag":  []any{"REMNAWAVE_WARP_PROBE_INBOUND"},
			"outboundTag": "WARP",
		}
		if len(rules) == 0 {
			rules = []any{map[string]any{}, probeRule}
		} else {
			rules = append(rules[:1], append([]any{probeRule}, rules[1:]...)...)
		}
		routing["rules"] = rules
		result["routing"] = routing
	}

	if torrent.Enabled {
		webhookURL := buildWebhookURL(torrent.SocketPath)
		outbounds := arrayFrom(result["outbounds"])
		outbounds = append(outbounds, map[string]any{
			"tag":      torrentBlockerOutboundTag,
			"protocol": "blackhole",
		})
		result["outbounds"] = outbounds

		routing, _ := result["routing"].(map[string]any)
		rules := arrayFrom(routing["rules"])
		torrentRule := map[string]any{
			"protocol":    []any{"bittorrent"},
			"outboundTag": torrentBlockerOutboundTag,
			"webhook": map[string]any{
				"url":           webhookURL,
				"deduplication": 5,
			},
		}
		if len(rules) == 0 {
			rules = []any{torrentRule}
		} else {
			inserted := make([]any, 0, len(rules)+1)
			inserted = append(inserted, rules[0], torrentRule)
			inserted = append(inserted, rules[1:]...)
			rules = inserted
		}

		if len(torrent.IncludeRuleTags) > 0 {
			tagSet := make(map[string]struct{}, len(torrent.IncludeRuleTags))
			for _, tag := range torrent.IncludeRuleTags {
				tagSet[tag] = struct{}{}
			}
			for i, item := range rules {
				rule, ok := item.(map[string]any)
				if !ok {
					continue
				}
				ruleTag, _ := rule["ruleTag"].(string)
				if _, ok := tagSet[ruleTag]; ok {
					rule["webhook"] = map[string]any{
						"url":           webhookURL,
						"deduplication": 5,
					}
					rules[i] = rule
				}
			}
		}
		routing["rules"] = rules
		result["routing"] = routing
	}

	return result
}

func guardedOutbounds(outbounds []any) ([]any, bool, error) {
	guarded, hasWarp, err := guardWarpOutbounds(outbounds)
	return guarded, hasWarp, err
}

func buildWebhookURL(socketPath string) string {
	return "/" + socketPath + ":/internal/webhook"
}

// apiInbound exposes the Xray gRPC API on a Linux abstract unix socket via the
// tunnel inbound, matching remnawave/node 2.8.0 (XRAY_API_INBOUND_MODEL). The
// leading "@" selects the abstract namespace; a local socket needs no TLS.
func apiInbound(socketName string) map[string]any {
	return map[string]any{
		"tag":      apiInboundTag,
		"listen":   "@" + socketName,
		"protocol": "tunnel",
	}
}

func policyFrom(existing any, statsUserOnline bool) map[string]any {
	levelZero := map[string]any{}
	if existingPolicy, ok := existing.(map[string]any); ok {
		if levels, ok := existingPolicy["levels"].(map[string]any); ok {
			if zero, ok := levels["0"].(map[string]any); ok {
				for key, value := range zero {
					levelZero[key] = value
				}
			}
		}
	}

	levelZero["statsUserUplink"] = true
	levelZero["statsUserDownlink"] = true
	levelZero["statsUserOnline"] = statsUserOnline

	return map[string]any{
		"levels": map[string]any{
			"0": levelZero,
		},
		"system": map[string]any{
			"statsInboundDownlink":  true,
			"statsInboundUplink":    true,
			"statsOutboundDownlink": true,
			"statsOutboundUplink":   true,
		},
	}
}

func routingFrom(existing any) map[string]any {
	routing := map[string]any{}
	if existingRouting, ok := existing.(map[string]any); ok {
		for key, value := range existingRouting {
			routing[key] = value
		}
	}

	rules := []any{
		map[string]any{
			"inboundTag":  []any{apiInboundTag},
			"outboundTag": apiTag,
		},
	}
	// Drop any pre-existing REMNAWAVE_API rule before re-injecting ours, matching
	// upstream generate-api-config.ts (prevents duplicate API routing rules).
	for _, item := range arrayFrom(routing["rules"]) {
		if rule, ok := item.(map[string]any); ok {
			if tag, _ := rule["outboundTag"].(string); tag == apiTag {
				continue
			}
		}
		rules = append(rules, item)
	}
	routing["rules"] = rules

	return routing
}

func arrayFrom(value any) []any {
	if value == nil {
		return []any{}
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	return []any{}
}
