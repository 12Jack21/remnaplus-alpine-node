package xray

import "testing"

func TestGenerateAPIConfigDedupesAPIRoutingRule(t *testing.T) {
	t.Parallel()

	config := generateAPIConfig(map[string]any{
		"routing": map[string]any{
			"rules": []any{
				map[string]any{"inboundTag": []any{apiInboundTag}, "outboundTag": apiTag},
				map[string]any{"outboundTag": "direct"},
			},
		},
	}, "remnanode-xtls-test", TorrentBlockerOptions{})

	routing := config["routing"].(map[string]any)
	apiRules := 0
	for _, item := range arrayFrom(routing["rules"]) {
		rule, _ := item.(map[string]any)
		if tag, _ := rule["outboundTag"].(string); tag == apiTag {
			apiRules++
		}
	}
	if apiRules != 1 {
		t.Fatalf("expected exactly 1 %s routing rule after dedupe, got %d", apiTag, apiRules)
	}
}

func TestGenerateAPIConfigTorrentBlocker(t *testing.T) {
	t.Parallel()

	cfg := generateAPIConfig(map[string]any{
		"inbounds":  []any{map[string]any{"tag": "in-1"}},
		"outbounds": []any{},
		"routing": map[string]any{
			"rules": []any{
				map[string]any{"ruleTag": "custom", "domain": []any{"example.com"}},
			},
		},
	}, "remnanode-xtls-test", TorrentBlockerOptions{
		Enabled:         true,
		IncludeRuleTags: []string{"custom"},
		SocketPath:      "/run/test.sock",
		RESTToken:       "token",
	})

	outbounds := arrayFrom(cfg["outbounds"])
	foundOutbound := false
	for _, item := range outbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if outbound["tag"] == torrentBlockerOutboundTag {
			foundOutbound = true
		}
	}
	if !foundOutbound {
		t.Fatal("expected torrent blocker outbound")
	}

	routing, ok := cfg["routing"].(map[string]any)
	if !ok {
		t.Fatal("missing routing")
	}
	rules := arrayFrom(routing["rules"])
	if len(rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(rules))
	}
}

func TestGenerateAPIConfigManagedWarpAddsProbe(t *testing.T) {
	t.Parallel()

	cfg := generateAPIConfig(map[string]any{
		"inbounds": []any{},
		"outbounds": []any{map[string]any{
			"tag":      "WARP",
			"protocol": "wireguard",
			"settings": map[string]any{
				"address": []any{"172.16.0.2/32"},
				"peers":   []any{map[string]any{"endpoint": "162.159.192.1:2408"}},
			},
		}},
		"routing": map[string]any{"rules": []any{map[string]any{"outboundTag": "direct"}}},
	}, "remnawave-xtls-test", TorrentBlockerOptions{})

	inbounds := arrayFrom(cfg["inbounds"])
	if len(inbounds) < 2 || inbounds[1].(map[string]any)["tag"] != "REMNAWAVE_WARP_PROBE_INBOUND" {
		t.Fatalf("WARP probe inbound missing: %#v", inbounds)
	}
	routing := cfg["routing"].(map[string]any)
	rules := arrayFrom(routing["rules"])
	if len(rules) < 3 || rules[1].(map[string]any)["outboundTag"] != "WARP" {
		t.Fatalf("WARP probe route missing: %#v", rules)
	}
}
