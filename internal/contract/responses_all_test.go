package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/auditlog"
	"github.com/12Jack21/remnaplus-alpine-node/internal/connections"
	"github.com/12Jack21/remnaplus-alpine-node/internal/nodehandler"
	"github.com/12Jack21/remnaplus-alpine-node/internal/plugin"
	"github.com/12Jack21/remnaplus-alpine-node/internal/snihealth"
	"github.com/12Jack21/remnaplus-alpine-node/internal/stats"
	"github.com/12Jack21/remnaplus-alpine-node/internal/xray"
	"github.com/12Jack21/remnaplus-alpine-node/internal/xtls"
)

var responseShapeTests = map[string]func(t *testing.T){
	"/node/xray/start":                          testXrayStartResponseShape,
	"/node/xray/stop":                           testXrayStopResponseShape,
	"/node/xray/healthcheck":                    testXrayHealthcheckResponseShape,
	"/node/stats/get-user-online-status":        testGetUserOnlineStatusResponseShape,
	"/node/stats/get-tcp-connections":           testGetTCPConnectionsResponseShape,
	"/node/stats/get-audit-log-chunk":           testGetAuditLogChunkResponseShape,
	"/node/stats/get-audit-log-source-metadata": testGetAuditLogSourceMetadataResponseShape,
	"/node/stats/clean-audit-logs":              testCleanAuditLogsResponseShape,
	"/node/stats/get-system-stats":              testGetSystemStatsResponseShape,
	"/node/stats/get-users-stats":               testGetUsersStatsResponseShape,
	"/node/stats/get-inbound-stats":             testGetInboundStatsResponseShape,
	"/node/stats/get-outbound-stats":            testGetOutboundStatsResponseShape,
	"/node/stats/get-all-inbounds-stats":        testGetAllInboundsStatsResponseShape,
	"/node/stats/get-all-outbounds-stats":       testGetAllOutboundsStatsResponseShape,
	"/node/stats/get-combined-stats":            testGetCombinedStatsResponseShape,
	"/node/stats/get-user-ip-list":              testGetUserIPListResponseShape,
	"/node/stats/get-users-ip-list":             testGetUsersIPListResponseShape,
	"/node/sni-health/status":                   testSNIHealthStatusResponseShape,
	"/node/sni-health/probe":                    testSNIHealthProbeResponseShape,
	"/node/handler/add-user":                    testAddUserResponseShape,
	"/node/handler/remove-user":                 testRemoveUserResponseShape,
	"/node/handler/get-inbound-users-count":     testGetInboundUsersCountResponseShape,
	"/node/handler/get-inbound-users":           testGetInboundUsersResponseShape,
	"/node/handler/add-users":                   testAddUsersResponseShape,
	"/node/handler/remove-users":                testRemoveUsersResponseShape,
	"/node/handler/drop-users-connections":      testDropUsersConnectionsResponseShape,
	"/node/handler/drop-ips":                    testDropIPsResponseShape,
	"/node/plugin/sync":                         testPluginSyncResponseShape,
	"/node/plugin/torrent-blocker/collect":      testPluginCollectReportsResponseShape,
	"/node/plugin/nftables/block-ips":           testPluginBlockIPsResponseShape,
	"/node/plugin/nftables/unblock-ips":         testPluginUnblockIPsResponseShape,
	"/node/plugin/nftables/recreate-tables":     testPluginRecreateTablesResponseShape,
}

func TestApprovedResponseShapes(t *testing.T) {
	for _, route := range approvedRoutes {
		route := route
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			fn, ok := responseShapeTests[route]
			if !ok {
				t.Fatalf("missing response shape test for %s", route)
			}
			fn(t)
		})
	}
}

func testManager(t *testing.T) *xray.Manager {
	t.Helper()
	manager, err := xray.NewManager(xray.Options{
		XrayBin:            "definitely-missing-rw-core",
		GeoDir:             t.TempDir(),
		LogDir:             t.TempDir(),
		InternalSocketPath: "/run/remnawave.sock",
		InternalRESTToken:  "token",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager
}

func encodeEnvelope(response any) []byte {
	body, _ := json.Marshal(map[string]any{"response": response})
	return body
}

func testXrayStartResponseShape(t *testing.T) {
	manager := testManager(t)
	resp := manager.Start(context.Background(), xray.StartRequest{
		XrayConfig: map[string]any{"inbounds": []any{}},
	})
	raw := encodeEnvelope(resp)
	assertTopLevelResponse(t, raw)
	assertJSONPath(t, raw, "response.isStarted")
	assertJSONPath(t, raw, "response.nodeInformation.version")
	assertJSONPath(t, raw, "response.system.info.arch")
	assertJSONPath(t, raw, "response.system.stats.memoryFree")
}

func testXrayStopResponseShape(t *testing.T) {
	manager := testManager(t)
	raw := encodeEnvelope(manager.Stop(true))
	assertJSONPath(t, raw, "response.isStopped")
}

func testXrayHealthcheckResponseShape(t *testing.T) {
	manager := testManager(t)
	raw := encodeEnvelope(manager.Health())
	assertJSONPath(t, raw, "response.isAlive")
	assertJSONPath(t, raw, "response.xrayInternalStatusCached")
	assertJSONPath(t, raw, "response.xrayVersion")
	assertJSONPath(t, raw, "response.nodeVersion")
}

func statsService(t *testing.T) *stats.Service {
	t.Helper()
	return stats.NewService(stubStatsProvider{}, stubReportsCounter{})
}

func testGetUserOnlineStatusResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-user-online-status", strings.NewReader(`{"username":"u1"}`))
	rec := httptest.NewRecorder()
	service.HandleGetUserOnlineStatus(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.isOnline")
}

func testGetSystemStatsResponseShape(t *testing.T) {
	service := statsService(t)
	rec := httptest.NewRecorder()
	service.HandleGetSystemStats(rec, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.plugins.torrentBlocker.reportsCount")
	assertJSONPath(t, raw, "response.system.listeningPorts")
	assertJSONPath(t, raw, "response.system.stats.memoryFree")
	assertJSONPath(t, raw, "response.system.stats.loadAvg")
	assertJSONPath(t, raw, "response.system.stats.tcp.established")
	assertJSONPath(t, raw, "response.system.stats.tcp.total")
	assertJSONPath(t, raw, "response.system.stats.vnstatDaily")
	assertJSONPath(t, raw, "response.system.stats.vnstatError")
	assertJSONPath(t, raw, "response.system.stats.vnstatTotalBytes")
}

func testGetTCPConnectionsResponseShape(t *testing.T) {
	service := statsService(t)
	rec := httptest.NewRecorder()
	service.HandleGetTCPConnections(rec, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.collectedAt")
	assertJSONPathArray(t, raw, "response.connections")
}

func testSNIHealthStatusResponseShape(t *testing.T) {
	raw := encodeEnvelope(snihealth.StatusResponse{
		Results: []snihealth.Result{sniContractResult()}, UnhealthyCount: 0, CheckedAt: time.Now(),
	})
	assertSNIResultShape(t, raw)
	assertJSONPath(t, raw, "response.unhealthyCount")
	assertJSONPath(t, raw, "response.checkedAt")
}

func testSNIHealthProbeResponseShape(t *testing.T) {
	raw := encodeEnvelope(snihealth.BuildProbeResponse("active", []snihealth.Result{sniContractResult()}, time.Now()))
	assertJSONPath(t, raw, "response.mode")
	assertSNIResultShape(t, raw)
	assertJSONPath(t, raw, "response.aggregateVerdict")
	assertJSONPath(t, raw, "response.counts.usable")
	assertJSONPath(t, raw, "response.counts.warning")
	assertJSONPath(t, raw, "response.counts.unusable")
	assertJSONPath(t, raw, "response.counts.unverified")
	assertJSONPath(t, raw, "response.unhealthyCount")
	assertJSONPath(t, raw, "response.checkedAt")
}

func sniContractResult() snihealth.Result {
	latency := 40
	correlationID := "contract-1"
	inboundTag := "main"
	return snihealth.Result{
		CorrelationID: &correlationID, InboundTag: &inboundTag, SNI: "example.com", Dest: "example.com:443", Hostname: "example.com", Port: 443,
		Verdict: snihealth.VerdictUsable, Healthy: true, LatencyMS: &latency,
		Checks:         []snihealth.Check{{ID: "dns-resolution", Status: snihealth.StatusPass, ObservedValue: []string{"93.184.216.34"}, Reason: "resolved"}},
		Addresses:      []snihealth.Address{{Address: "93.184.216.34", Family: 4, IsPublic: true, Status: snihealth.StatusPass}},
		LatencySamples: []int{40, 41, 39}, JitterMS: contractIntPtr(2), CheckedAt: time.Now(),
	}
}

func assertSNIResultShape(t *testing.T, raw []byte) {
	t.Helper()
	var body struct {
		Response struct {
			Results []json.RawMessage `json:"results"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || len(body.Response.Results) == 0 {
		t.Fatalf("missing SNI result: %s (%v)", string(raw), err)
	}
	result := body.Response.Results[0]
	for _, field := range []string{
		"correlationId", "inboundTag", "sni", "dest", "hostname", "port", "verdict", "healthy",
		"latencyMs", "error", "checks", "addresses", "latencySamplesMs", "jitterMs", "checkedAt",
	} {
		assertJSONPath(t, result, field)
	}
}

func contractIntPtr(value int) *int { return &value }

func testGetAuditLogChunkResponseShape(t *testing.T) {
	service := auditlog.NewService(filepath.Join(t.TempDir(), "access.log"), filepath.Join(t.TempDir(), "error.log"))
	chunk, err := service.ReadChunk(auditlog.ChunkRequest{Source: auditlog.SourceAccess})
	if err != nil {
		t.Fatal(err)
	}
	raw := encodeEnvelope(chunk)
	assertJSONPath(t, raw, "response.fileInode")
	assertJSONPathArray(t, raw, "response.lines")
	assertJSONPath(t, raw, "response.nextOffset")
	assertJSONPath(t, raw, "response.readStart")
	assertJSONPath(t, raw, "response.source")
	assertJSONPath(t, raw, "response.totalSize")
}

func testGetAuditLogSourceMetadataResponseShape(t *testing.T) {
	service := auditlog.NewService(filepath.Join(t.TempDir(), "access.log"), filepath.Join(t.TempDir(), "error.log"))
	metadata, err := service.SourceMetadata(auditlog.SourceError)
	if err != nil {
		t.Fatal(err)
	}
	raw := encodeEnvelope(metadata)
	assertJSONPath(t, raw, "response.earliestTimestamp")
	assertJSONPath(t, raw, "response.inode")
	assertJSONPath(t, raw, "response.latestTimestamp")
	assertJSONPath(t, raw, "response.source")
	assertJSONPath(t, raw, "response.totalSize")
}

func testCleanAuditLogsResponseShape(t *testing.T) {
	raw := encodeEnvelope(auditlog.CleanResult{})
	assertJSONPath(t, raw, "response.files")
	assertJSONPath(t, raw, "response.keptLines")
	assertJSONPath(t, raw, "response.removedLines")
	assertJSONPath(t, raw, "response.reclaimedBytes")
}

func TestAuditLogResponseShapes(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	accessPath := filepath.Join(directory, "access.log")
	if err := os.WriteFile(accessPath, []byte("2026/07/18 00:00:00 access\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := auditlog.NewService(accessPath, filepath.Join(directory, "error.log"))
	chunk, err := service.ReadChunk(auditlog.ChunkRequest{Source: auditlog.SourceAccess, Offset: contractInt64Ptr(0)})
	if err != nil {
		t.Fatal(err)
	}
	chunkRaw := encodeEnvelope(chunk)
	for _, path := range []string{
		"response.fileInode",
		"response.lines",
		"response.nextOffset",
		"response.readStart",
		"response.source",
		"response.totalSize",
	} {
		assertJSONPath(t, chunkRaw, path)
	}

	metadata, err := service.SourceMetadata(auditlog.SourceAccess)
	if err != nil {
		t.Fatal(err)
	}
	metadataRaw := encodeEnvelope(metadata)
	for _, path := range []string{
		"response.earliestTimestamp",
		"response.inode",
		"response.latestTimestamp",
		"response.source",
		"response.totalSize",
	} {
		assertJSONPath(t, metadataRaw, path)
	}

	cleanRaw := encodeEnvelope(auditlog.CleanResult{})
	for _, path := range []string{
		"response.files",
		"response.keptLines",
		"response.removedLines",
		"response.reclaimedBytes",
	} {
		assertJSONPath(t, cleanRaw, path)
	}
}

func contractInt64Ptr(value int64) *int64 { return &value }

func testGetUsersStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-users-stats", strings.NewReader(`{"reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetUsersStats(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.users")
}

func testGetInboundStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-inbound-stats", strings.NewReader(`{"tag":"in-1","reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetInboundStats(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.inbound")
	assertJSONPath(t, raw, "response.downlink")
	assertJSONPath(t, raw, "response.uplink")
}

func testGetOutboundStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-outbound-stats", strings.NewReader(`{"tag":"out-1","reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetOutboundStats(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.outbound")
	assertJSONPath(t, raw, "response.downlink")
	assertJSONPath(t, raw, "response.uplink")
}

func testGetAllInboundsStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-all-inbounds-stats", strings.NewReader(`{"reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetAllInboundsStats(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.inbounds")
}

func testGetAllOutboundsStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-all-outbounds-stats", strings.NewReader(`{"reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetAllOutboundsStats(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.outbounds")
}

func testGetCombinedStatsResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-combined-stats", strings.NewReader(`{"reset":false}`))
	rec := httptest.NewRecorder()
	service.HandleGetCombinedStats(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPathArray(t, raw, "response.inbounds")
	assertJSONPathArray(t, raw, "response.outbounds")
}

func testGetUserIPListResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-user-ip-list", strings.NewReader(`{"userId":"u1"}`))
	rec := httptest.NewRecorder()
	service.HandleGetUserIPList(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.ips")
}

func testGetUsersIPListResponseShape(t *testing.T) {
	service := statsService(t)
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-users-ip-list", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	service.HandleGetUsersIPList(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.users")
}

func handlerService() *nodehandler.Service {
	return nodehandler.NewService(stubHandlerProvider{}, connections.NewDropper(nil))
}

func testRemoveUserResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/remove-user", strings.NewReader(`{
		"username":"u1",
		"hashData":{"vlessUuid":"00000000-0000-4000-8000-000000000001"}
	}`))
	rec := httptest.NewRecorder()
	service.HandleRemoveUser(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.success")
	assertJSONPath(t, raw, "response.error")
}

func testGetInboundUsersResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/get-inbound-users", strings.NewReader(`{"tag":"in-1"}`))
	rec := httptest.NewRecorder()
	service.HandleGetInboundUsers(rec, req, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.users")
}

func testAddUsersResponseShape(t *testing.T) {
	service := handlerService()
	body := `{
		"data":[{"type":"vless","tag":"in-1","username":"u1","uuid":"00000000-0000-4000-8000-000000000001","flow":""}],
		"hashData":{"vlessUuid":"00000000-0000-4000-8000-000000000002"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/node/handler/add-users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	service.HandleAddUsers(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.success")
	assertJSONPath(t, raw, "response.error")
}

func testRemoveUsersResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/remove-users", strings.NewReader(`{
		"usernames":["u1"],
		"hashData":{"vlessUuid":"00000000-0000-4000-8000-000000000001"}
	}`))
	rec := httptest.NewRecorder()
	service.HandleRemoveUsers(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.success")
	assertJSONPath(t, raw, "response.error")
}

func testDropIPsResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/drop-ips", strings.NewReader(`{"ips":["203.0.113.10"]}`))
	rec := httptest.NewRecorder()
	service.HandleDropIPs(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.success")
}

func testAddUserResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/add-user", strings.NewReader(`{
		"data":[{"type":"vless","tag":"in-1","username":"u1","uuid":"00000000-0000-4000-8000-000000000001","flow":""}],
		"hashData":{"vlessUuid":"00000000-0000-4000-8000-000000000002"}
	}`))
	rec := httptest.NewRecorder()
	service.HandleAddUser(rec, req, writeTestJSON)
	raw := rec.Body.Bytes()
	assertJSONPath(t, raw, "response.success")
	assertJSONPath(t, raw, "response.error")
}

func testDropUsersConnectionsResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/drop-users-connections", strings.NewReader(`{"userIds":["user-1"]}`))
	rec := httptest.NewRecorder()
	service.HandleDropUsersConnections(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.success")
}

func testGetInboundUsersCountResponseShape(t *testing.T) {
	service := handlerService()
	req := httptest.NewRequest(http.MethodPost, "/node/handler/get-inbound-users-count", strings.NewReader(`{"tag":"in-1"}`))
	rec := httptest.NewRecorder()
	service.HandleGetInboundUsersCount(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.count")
}

func testPluginSyncResponseShape(t *testing.T) {
	service := pluginService()
	req := httptest.NewRequest(http.MethodPost, "/node/plugin/sync", strings.NewReader(`{"plugin":null}`))
	rec := httptest.NewRecorder()
	service.HandleSync(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.accepted")
}

func testPluginCollectReportsResponseShape(t *testing.T) {
	service := pluginService()
	rec := httptest.NewRecorder()
	service.HandleCollectReports(rec, writeTestJSON)
	assertJSONPathArray(t, rec.Body.Bytes(), "response.reports")
}

func testPluginBlockIPsResponseShape(t *testing.T) {
	service := pluginService()
	req := httptest.NewRequest(http.MethodPost, "/node/plugin/nftables/block-ips", strings.NewReader(`{"ips":[{"ip":"203.0.113.10","timeout":60}]}`))
	rec := httptest.NewRecorder()
	service.HandleBlockIPs(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.accepted")
}

func pluginService() *plugin.Service {
	state := plugin.NewState()
	return plugin.NewService(state, connections.NewDropper(state.IsWhitelisted), nil)
}

func testPluginUnblockIPsResponseShape(t *testing.T) {
	service := pluginService()
	req := httptest.NewRequest(http.MethodPost, "/node/plugin/nftables/unblock-ips", strings.NewReader(`{"ips":["203.0.113.10"]}`))
	rec := httptest.NewRecorder()
	service.HandleUnblockIPs(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.accepted")
}

func testPluginRecreateTablesResponseShape(t *testing.T) {
	service := pluginService()
	req := httptest.NewRequest(http.MethodPost, "/node/plugin/nftables/recreate-tables", nil)
	rec := httptest.NewRecorder()
	service.HandleRecreateTables(rec, req, writeTestJSON)
	assertJSONPath(t, rec.Body.Bytes(), "response.accepted")
}

type stubStatsProvider struct{}

func (stubStatsProvider) GetSysStats(context.Context) (*xtls.SysStats, error) {
	return &xtls.SysStats{NumGoroutine: 1, Uptime: 10}, nil
}
func (stubStatsProvider) GetAllUsersStats(context.Context, bool) ([]xtls.UserTraffic, error) {
	return []xtls.UserTraffic{{Username: "u1", Uplink: 1, Downlink: 2}}, nil
}
func (stubStatsProvider) GetUserOnlineStatus(context.Context, string) (bool, error) {
	return false, nil
}
func (stubStatsProvider) GetInboundStats(context.Context, string, bool) (xtls.TagTraffic, error) {
	return xtls.TagTraffic{Tag: "in-1"}, nil
}
func (stubStatsProvider) GetOutboundStats(context.Context, string, bool) (xtls.TagTraffic, error) {
	return xtls.TagTraffic{Tag: "out-1"}, nil
}
func (stubStatsProvider) GetAllInboundsStats(context.Context, bool) ([]xtls.TagTraffic, error) {
	return []xtls.TagTraffic{{Tag: "in-1"}}, nil
}
func (stubStatsProvider) GetAllOutboundsStats(context.Context, bool) ([]xtls.TagTraffic, error) {
	return []xtls.TagTraffic{{Tag: "out-1"}}, nil
}
func (stubStatsProvider) GetUserIPList(context.Context, string, bool) ([]xtls.IPEntry, error) {
	return []xtls.IPEntry{{IP: "203.0.113.10", LastSeen: time.Now()}}, nil
}
func (stubStatsProvider) GetUsersIPList(context.Context) ([]xtls.UserIPEntry, error) {
	return []xtls.UserIPEntry{{UserID: "u1"}}, nil
}

type stubReportsCounter struct{}

func (stubReportsCounter) ReportsCount() int { return 0 }
