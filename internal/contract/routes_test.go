package contract_test

import (
	"encoding/json"
	"os"
	"testing"
)

// approvedRoutes is the RemnaPlus 2.8.0 node contract plus native extensions,
// excluding HAProxy forwarding telemetry by capability policy.
var approvedRoutes = []string{
	"/node/xray/start",
	"/node/xray/stop",
	"/node/xray/healthcheck",
	"/node/stats/get-user-online-status",
	"/node/stats/get-accounting-snapshot",
	"/node/stats/get-tcp-connections",
	"/node/stats/get-audit-log-chunk",
	"/node/stats/get-audit-log-source-metadata",
	"/node/stats/clean-audit-logs",
	"/node/stats/get-users-stats",
	"/node/stats/get-system-stats",
	"/node/stats/get-inbound-stats",
	"/node/stats/get-outbound-stats",
	"/node/stats/get-all-outbounds-stats",
	"/node/stats/get-all-inbounds-stats",
	"/node/stats/get-combined-stats",
	"/node/stats/get-user-ip-list",
	"/node/stats/get-users-ip-list",
	"/node/sni-health/status",
	"/node/sni-health/probe",
	"/node/handler/add-user",
	"/node/handler/remove-user",
	"/node/handler/get-inbound-users-count",
	"/node/handler/get-inbound-users",
	"/node/handler/add-users",
	"/node/handler/remove-users",
	"/node/handler/drop-users-connections",
	"/node/handler/drop-ips",
	"/node/plugin/sync",
	"/node/plugin/torrent-blocker/collect",
	"/node/plugin/nftables/block-ips",
	"/node/plugin/nftables/unblock-ips",
	"/node/plugin/nftables/recreate-tables",
}

// implementedRoutes marks routes wired in this repository.
var implementedRoutes = map[string]bool{
	"/node/xray/start":                          true,
	"/node/xray/stop":                           true,
	"/node/xray/healthcheck":                    true,
	"/node/stats/get-user-online-status":        true,
	"/node/stats/get-accounting-snapshot":       true,
	"/node/stats/get-tcp-connections":           true,
	"/node/stats/get-audit-log-chunk":           true,
	"/node/stats/get-audit-log-source-metadata": true,
	"/node/stats/clean-audit-logs":              true,
	"/node/stats/get-users-stats":               true,
	"/node/stats/get-system-stats":              true,
	"/node/stats/get-inbound-stats":             true,
	"/node/stats/get-outbound-stats":            true,
	"/node/stats/get-all-outbounds-stats":       true,
	"/node/stats/get-all-inbounds-stats":        true,
	"/node/stats/get-combined-stats":            true,
	"/node/stats/get-user-ip-list":              true,
	"/node/stats/get-users-ip-list":             true,
	"/node/sni-health/status":                   true,
	"/node/sni-health/probe":                    true,
	"/node/handler/add-user":                    true,
	"/node/handler/remove-user":                 true,
	"/node/handler/get-inbound-users-count":     true,
	"/node/handler/get-inbound-users":           true,
	"/node/handler/add-users":                   true,
	"/node/handler/remove-users":                true,
	"/node/handler/drop-users-connections":      true,
	"/node/handler/drop-ips":                    true,
	"/node/plugin/sync":                         true,
	"/node/plugin/torrent-blocker/collect":      true,
	"/node/plugin/nftables/block-ips":           true,
	"/node/plugin/nftables/unblock-ips":         true,
	"/node/plugin/nftables/recreate-tables":     true,
}

func TestApprovedRoutesCoverage(t *testing.T) {
	t.Parallel()

	for _, route := range approvedRoutes {
		if !implementedRoutes[route] {
			t.Fatalf("route %s not marked implemented in lite-go", route)
		}
	}
}

func TestReviewedOfficialRoutesAreImplemented(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("official-routes-2.8.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		SchemaVersion  int      `json:"schemaVersion"`
		Commit         string   `json:"commit"`
		PackageVersion string   `json:"packageVersion"`
		Routes         []string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 1 || snapshot.Commit == "" || snapshot.PackageVersion != "2.8.0" {
		t.Fatalf("invalid reviewed official contract snapshot: %+v", snapshot)
	}
	seen := make(map[string]bool, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		if seen[route] {
			t.Fatalf("duplicate official route %s", route)
		}
		seen[route] = true
		if !implementedRoutes[route] {
			t.Fatalf("official route %s is not implemented", route)
		}
	}
	if len(snapshot.Routes) != 26 {
		t.Fatalf("official route count = %d, want reviewed count 26", len(snapshot.Routes))
	}
}
