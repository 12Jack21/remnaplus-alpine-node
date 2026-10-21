package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/auditlog"
	"github.com/12Jack21/remnaplus-alpine-node/internal/backtrace"
	"github.com/12Jack21/remnaplus-alpine-node/internal/snihealth"
	"github.com/12Jack21/remnaplus-alpine-node/internal/stats"
	"github.com/12Jack21/remnaplus-alpine-node/internal/warp"
	"github.com/12Jack21/remnaplus-alpine-node/internal/xray"
	"github.com/12Jack21/remnaplus-alpine-node/internal/xtls"
)

func TestWarpHealthUsesDashboardContractPath(t *testing.T) {
	manager, err := xray.NewManager(xray.Options{XrayBin: "missing-canary-core"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{warpService: warp.NewService(manager, 61001)}
	recorder := httptest.NewRecorder()
	server.handleWarpRoutes(recorder, httptest.NewRequest(http.MethodGet, "/node/warp/healthcheck", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Response warp.Response `json:"response"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Response.State != "not-configured" {
		t.Fatalf("state=%s", body.Response.State)
	}
}

type failingUsersStatsProvider struct{}

type routeAccountingReader struct{}

func (routeAccountingReader) ReadAccountingCounters(context.Context) (map[string]int64, string, error) {
	return map[string]int64{"user>>>29>>>uplink": 1}, "xray-1", nil
}

func (failingUsersStatsProvider) GetSysStats(context.Context) (*xtls.SysStats, error) {
	return &xtls.SysStats{}, nil
}
func (f failingUsersStatsProvider) GetAllUsersStats(context.Context, bool) ([]xtls.UserTraffic, error) {
	return nil, errors.New("grpc unavailable")
}
func (f failingUsersStatsProvider) GetUserOnlineStatus(context.Context, string) (bool, error) {
	return false, nil
}
func (f failingUsersStatsProvider) GetInboundStats(context.Context, string, bool) (xtls.TagTraffic, error) {
	return xtls.TagTraffic{}, nil
}
func (f failingUsersStatsProvider) GetOutboundStats(context.Context, string, bool) (xtls.TagTraffic, error) {
	return xtls.TagTraffic{}, nil
}
func (f failingUsersStatsProvider) GetAllInboundsStats(context.Context, bool) ([]xtls.TagTraffic, error) {
	return nil, nil
}
func (f failingUsersStatsProvider) GetAllOutboundsStats(context.Context, bool) ([]xtls.TagTraffic, error) {
	return nil, nil
}
func (f failingUsersStatsProvider) GetUserIPList(context.Context, string, bool) ([]xtls.IPEntry, error) {
	return nil, nil
}
func (f failingUsersStatsProvider) GetUsersIPList(context.Context) ([]xtls.UserIPEntry, error) {
	return nil, nil
}

func TestHandleNodeRoutesUsersStatsError(t *testing.T) {
	t.Parallel()

	server := &Server{
		statsService: stats.NewService(failingUsersStatsProvider{}, nil),
	}
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-users-stats", strings.NewReader(`{"reset":false}`))
	rec := httptest.NewRecorder()

	server.handleNodeRoutes(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["errorCode"] != "A011" {
		t.Fatalf("errorCode = %v, want A011", body["errorCode"])
	}
}

func TestHandleNodeRoutesAccountingSnapshot(t *testing.T) {
	service := stats.NewAccountingSnapshotService(routeAccountingReader{}, filepath.Join(t.TempDir(), "accounting.json"))
	server := &Server{statsService: stats.NewService(failingUsersStatsProvider{}, nil, service)}
	req := httptest.NewRequest(http.MethodPost, "/node/stats/get-accounting-snapshot", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	server.handleNodeRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Response struct {
			ContractVersion int  `json:"contractVersion"`
			Pending         bool `json:"pending"`
			Users           []struct {
				Uplink string `json:"uplink"`
			} `json:"users"`
		} `json:"response"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Response.ContractVersion != 1 || !body.Response.Pending || len(body.Response.Users) != 0 {
		t.Fatalf("unexpected response: %+v", body.Response)
	}
}

func TestHandleNodeRoutesUnknownPath(t *testing.T) {
	t.Parallel()

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/node/unknown", nil)
	rec := httptest.NewRecorder()

	server.handleNodeRoutes(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleNodeRoutesTCPConnections(t *testing.T) {
	t.Parallel()

	server := &Server{statsService: stats.NewService(failingUsersStatsProvider{}, nil)}
	req := httptest.NewRequest(http.MethodGet, "/node/stats/get-tcp-connections", nil)
	rec := httptest.NewRecorder()

	server.handleNodeRoutes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Response struct {
			CollectedAt string `json:"collectedAt"`
			Connections []any  `json:"connections"`
		} `json:"response"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339Nano, body.Response.CollectedAt); err != nil {
		t.Fatalf("collectedAt = %q: %v", body.Response.CollectedAt, err)
	}
	if body.Response.Connections == nil {
		t.Fatal("connections must be an array, not null")
	}
}

func TestHandleNodeRoutesAuditLogAPIs(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	accessPath := filepath.Join(directory, "access.log")
	errorPath := filepath.Join(directory, "error.log")
	if err := os.WriteFile(accessPath, []byte("2026/07/18 00:00:00 access\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errorPath, []byte("2026/07/18 00:00:00 error\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := &Server{auditLogService: auditlog.NewService(accessPath, errorPath)}

	for _, route := range []string{
		"/node/stats/get-audit-log-chunk?source=access&offset=0",
		"/node/stats/get-audit-log-source-metadata?source=error",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		server.handleNodeRoutes(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", route, rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["response"] == nil {
			t.Fatalf("%s response = %s, error = %v", route, rec.Body.String(), err)
		}
	}

	cleanReq := httptest.NewRequest(http.MethodPost, "/node/stats/clean-audit-logs", strings.NewReader(`{"retentionDays":1}`))
	cleanRec := httptest.NewRecorder()
	server.handleNodeRoutes(cleanRec, cleanReq)
	if cleanRec.Code != http.StatusOK {
		t.Fatalf("clean status = %d, want 200: %s", cleanRec.Code, cleanRec.Body.String())
	}

	conflictReq := httptest.NewRequest(http.MethodGet, "/node/stats/get-audit-log-chunk?source=access&expectedInode=changed", nil)
	conflictRec := httptest.NewRecorder()
	server.handleNodeRoutes(conflictRec, conflictReq)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", conflictRec.Code)
	}
	var conflict map[string]any
	if err := json.Unmarshal(conflictRec.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict["message"] != "AUDIT_LOG_INODE_CHANGED" {
		t.Fatalf("conflict response = %v", conflict)
	}
}

func TestHandleNodeRoutesSNIHealthAPIs(t *testing.T) {
	t.Parallel()

	provider := &routeSNIProvider{}
	service := snihealth.NewService(provider, func(_ context.Context, target snihealth.Target) snihealth.Result {
		return snihealth.Result{
			CorrelationID: target.CorrelationID, InboundTag: target.InboundTag, SNI: target.SNI, Dest: target.Dest,
			Hostname: target.NormalizedSNI, Port: target.Port, Verdict: snihealth.VerdictUsable, Healthy: true,
			Checks: []snihealth.Check{}, Addresses: []snihealth.Address{}, LatencySamples: []int{}, CheckedAt: time.Now(),
		}
	}, time.Now)
	server := &Server{sniHealthService: service}

	status := httptest.NewRecorder()
	server.handleNodeRoutes(status, httptest.NewRequest(http.MethodGet, "/node/sni-health/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"results"`) {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}

	probe := httptest.NewRecorder()
	server.handleNodeRoutes(probe, httptest.NewRequest(http.MethodPost, "/node/sni-health/probe", strings.NewReader(`{"mode":"candidates","targets":[{"correlationId":"candidate-1","hostname":"Example.COM.","port":443}]}`)))
	if probe.Code != http.StatusOK || !strings.Contains(probe.Body.String(), `"aggregateVerdict":"usable"`) {
		t.Fatalf("probe = %d %s", probe.Code, probe.Body.String())
	}

	bad := httptest.NewRecorder()
	server.handleNodeRoutes(bad, httptest.NewRequest(http.MethodPost, "/node/sni-health/probe", strings.NewReader(`{"mode":"active","targets":[]}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid active probe = %d %s", bad.Code, bad.Body.String())
	}
}

type routeSNIProvider struct{}

func (*routeSNIProvider) CurrentConfig() map[string]any {
	return map[string]any{"inbounds": []any{map[string]any{
		"tag": "main", "streamSettings": map[string]any{"realitySettings": map[string]any{
			"dest": "example.com:443", "serverNames": []any{"example.com"},
		}},
	}}}
}

func (*routeSNIProvider) InboundTags() []string { return []string{"main"} }

func TestHandleNodeRoutesBacktraceRunRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	server := &Server{backtraceService: backtrace.NewService("nexttrace-tiny")}
	for _, body := range []string{
		`{"targets":[{"isp":"telecom","host":"192.168.1.1"}]}`,
		`{"targets":[{"isp":"other","host":"example.com"}]}`,
		`{"targets":[]}`,
		`{"targets":[{"isp":"telecom","host":"example.com","extra":true}]}`,
		`not-json`,
	} {
		recorder := httptest.NewRecorder()
		server.handleNodeRoutes(recorder, httptest.NewRequest(http.MethodPost, "/node/backtrace/run", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s => status %d: %s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandleNodeRoutesBacktraceRunReportsMissingBinary(t *testing.T) {
	t.Parallel()

	binary := filepath.Join(t.TempDir(), "missing-nexttrace-tiny")
	server := &Server{backtraceService: backtrace.NewService(binary)}
	recorder := httptest.NewRecorder()
	server.handleNodeRoutes(recorder, httptest.NewRequest(
		http.MethodPost,
		"/node/backtrace/run",
		strings.NewReader(`{"targets":[{"isp":"telecom","host":"1.1.1.1"}]}`),
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Response struct {
			Probes []struct {
				ISP        string  `json:"isp"`
				Checkpoint string  `json:"checkpoint"`
				Success    bool    `json:"success"`
				Error      *string `json:"error"`
				Hops       []any   `json:"hops"`
				RawOutput  string  `json:"rawOutput"`
			} `json:"probes"`
			GeneratedAt string `json:"generatedAt"`
		} `json:"response"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Response.Probes) != 1 || body.Response.Probes[0].Success {
		t.Fatalf("unexpected probes: %+v", body.Response.Probes)
	}
	probe := body.Response.Probes[0]
	if probe.Error == nil || *probe.Error != "nexttrace-tiny is not installed on this node." {
		t.Fatalf("error = %v", probe.Error)
	}
	if probe.Hops == nil {
		t.Fatal("hops must be an array, not null")
	}
	if _, err := time.Parse(time.RFC3339Nano, body.Response.GeneratedAt); err != nil {
		t.Fatalf("generatedAt = %q: %v", body.Response.GeneratedAt, err)
	}
}

func TestHandleNodeRoutesBacktraceRejectsGet(t *testing.T) {
	t.Parallel()

	server := &Server{backtraceService: backtrace.NewService("nexttrace-tiny")}
	recorder := httptest.NewRecorder()
	server.handleNodeRoutes(recorder, httptest.NewRequest(http.MethodGet, "/node/backtrace/run", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}
