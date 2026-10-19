package xray

import (
	"os"
	"reflect"
	"testing"
)

func testWarpOutbound(endpoint string) map[string]any {
	return map[string]any{
		"protocol": "wireguard",
		"settings": map[string]any{
			"address":        []any{"172.16.0.2/32", "2606:4700:110:aaaa::2/128"},
			"domainStrategy": "ForceIPv6",
			"peers": []any{map[string]any{
				"allowedIPs": []any{"0.0.0.0/0", "::/0"},
				"endpoint":   endpoint,
				"publicKey":  "public-test-key",
			}},
		},
		"streamSettings": map[string]any{"sockopt": map[string]any{"domainStrategy": "ForceIPv6"}},
		"tag":            "WARP",
		"targetStrategy": "ForceIPv6",
	}
}

func TestGuardWarpOutboundsForcesIPv4AndDoesNotMutateInput(t *testing.T) {
	original := testWarpOutbound("engage.cloudflareclient.com:2408")
	guarded, applied, err := guardWarpOutbounds([]any{original})
	if err != nil || !applied {
		t.Fatalf("guard returned applied=%v err=%v", applied, err)
	}
	value := guarded[0].(map[string]any)
	settings := value["settings"].(map[string]any)
	if settings["domainStrategy"] != "ForceIPv4" || value["targetStrategy"] != "ForceIPv4" {
		t.Fatalf("WARP strategies were not forced: %#v", value)
	}
	if !reflect.DeepEqual(settings["address"], []any{"172.16.0.2/32"}) {
		t.Fatalf("address = %#v", settings["address"])
	}
	if !reflect.DeepEqual(original["settings"].(map[string]any)["address"], []any{"172.16.0.2/32", "2606:4700:110:aaaa::2/128"}) {
		t.Fatal("guard mutated the source config")
	}
}

func TestGuardWarpOutboundsUsesThreeByteClientIDFallback(t *testing.T) {
	previous := os.Getenv("WARP_CLIENT_ID")
	defer os.Setenv("WARP_CLIENT_ID", previous)
	if err := os.Setenv("WARP_CLIENT_ID", "QNT5"); err != nil {
		t.Fatal(err)
	}
	guarded, _, err := guardWarpOutbounds([]any{testWarpOutbound("162.159.192.1:2408")})
	if err != nil {
		t.Fatal(err)
	}
	reserved := guarded[0].(map[string]any)["settings"].(map[string]any)["reserved"]
	if !reflect.DeepEqual(reserved, []any{int(64), int(212), int(249)}) {
		t.Fatalf("reserved = %#v", reserved)
	}
}

func TestGuardWarpOutboundsRejectsIPv6Peer(t *testing.T) {
	if _, _, err := guardWarpOutbounds([]any{testWarpOutbound("[2606:4700:d0::a29f:c001]:2408")}); err == nil {
		t.Fatal("IPv6 peer unexpectedly accepted")
	}
}

func TestWarpProbeUsesLoadedConfigurationPort(t *testing.T) {
	t.Setenv("WARP_PROBE_PORT", "61001")
	config := generateAPIConfig(map[string]any{
		"outbounds": []any{testWarpOutbound("162.159.192.1:2408")},
	}, "test-socket", TorrentBlockerOptions{}, 61234)
	for _, item := range config["inbounds"].([]any) {
		inbound := item.(map[string]any)
		if inbound["tag"] == "REMNAWAVE_WARP_PROBE_INBOUND" {
			if inbound["port"] != 61234 {
				t.Fatalf("probe port = %v", inbound["port"])
			}
			return
		}
	}
	t.Fatal("WARP probe inbound missing")
}
