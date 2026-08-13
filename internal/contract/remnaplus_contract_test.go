package contract_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/12Jack21/remnaplus-alpine-node/internal/system"
	"github.com/12Jack21/remnaplus-alpine-node/internal/tcpstats"
	"github.com/12Jack21/remnaplus-alpine-node/internal/vnstat"
)

const forwardedTCPRoute = "/node/stats/get-forwarded-tcp-connections"

func TestRemnaPlusApprovedRouteContractIsExactAndWired(t *testing.T) {
	t.Parallel()

	expected := []string{
		"/node/xray/start", "/node/xray/stop", "/node/xray/healthcheck",
		"/node/sni-health/status", "/node/sni-health/probe",
		"/node/stats/get-user-online-status", "/node/stats/get-accounting-snapshot", "/node/stats/get-users-stats",
		"/node/stats/get-system-stats", "/node/stats/get-audit-log-chunk",
		"/node/stats/get-audit-log-source-metadata", "/node/stats/clean-audit-logs",
		"/node/stats/get-tcp-connections", "/node/stats/get-inbound-stats",
		"/node/stats/get-outbound-stats", "/node/stats/get-all-outbounds-stats",
		"/node/stats/get-all-inbounds-stats", "/node/stats/get-combined-stats",
		"/node/stats/get-user-ip-list", "/node/stats/get-users-ip-list",
		"/node/handler/add-user", "/node/handler/remove-user",
		"/node/handler/get-inbound-users-count", "/node/handler/get-inbound-users",
		"/node/handler/add-users", "/node/handler/remove-users",
		"/node/handler/drop-users-connections", "/node/handler/drop-ips",
		"/node/plugin/sync", "/node/plugin/torrent-blocker/collect",
		"/node/plugin/nftables/block-ips", "/node/plugin/nftables/unblock-ips",
		"/node/plugin/nftables/recreate-tables",
	}
	actual := append([]string(nil), approvedRoutes...)
	sort.Strings(expected)
	sort.Strings(actual)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("approved routes = %#v, want %#v", actual, expected)
	}
	if implementedRoutes[forwardedTCPRoute] {
		t.Fatalf("forbidden Alpine forwarding telemetry route is implemented")
	}

	serverSource, err := os.ReadFile("../httpserver/server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range expected {
		if !strings.Contains(string(serverSource), `"`+route+`"`) {
			t.Fatalf("approved route %s is not wired in the HTTP server", route)
		}
	}
	if strings.Contains(string(serverSource), forwardedTCPRoute) {
		t.Fatalf("forbidden route %s is wired in the HTTP server", forwardedTCPRoute)
	}
}

func TestRemnaPlusExtendedSystemEnvelopeKeepsNullableTelemetry(t *testing.T) {
	t.Parallel()

	timestamp := int64(1_784_332_800)
	payload := struct {
		ListeningPorts []tcpstats.ListeningPort `json:"listeningPorts"`
		Stats          system.Stats             `json:"stats"`
	}{
		ListeningPorts: nil,
		Stats: system.Stats{
			TCP: tcpstats.Stats{},
			VnstatDaily: []vnstat.DailyEntry{
				{Date: vnstat.Date{Year: 2026, Month: 7, Day: 18}, RX: 10, TX: 20},
				{Date: vnstat.Date{Year: 2026, Month: 7, Day: 19}, RX: 30, TX: 40, Timestamp: &timestamp},
			},
			VnstatError: nil, VnstatTotalBytes: nil,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["listeningPorts"] != nil {
		t.Fatalf("failed listening-port collection must remain nullable: %s", raw)
	}
	stats := decoded["stats"].(map[string]any)
	if stats["vnstatError"] != nil || stats["vnstatTotalBytes"] != nil {
		t.Fatalf("vnStat diagnostics must remain nullable: %s", raw)
	}
	daily := stats["vnstatDaily"].([]any)
	if _, exists := daily[0].(map[string]any)["timestamp"]; exists {
		t.Fatalf("legacy daily row unexpectedly gained a timestamp: %s", raw)
	}
	if daily[1].(map[string]any)["timestamp"] == nil {
		t.Fatalf("source timestamp was not retained: %s", raw)
	}
}

func TestPublicContractDocumentStatesAlpineCapabilityBoundary(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../docs/contract.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"ALPINE_NATIVE", "STANDARD_DOCKER", "forwarding source", "forwarding target",
		"HAProxy", forwardedTCPRoute, "dashboard node data",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("contract document is missing %q", required)
		}
	}
}
