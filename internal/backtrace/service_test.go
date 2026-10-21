package backtrace

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
)

const plainTextOutput = "NextTrace v1.7.4 2026-09-17T16:10:04Z 8884923\n" +
	"1.1.1.1 -> 1.1.1.1, 30 hops max, 28 byte packets, ICMP mode\n" +
	"2   59.43.187.1    AS4809  [CHINANET]  China  China Telecom\n" +
	"    1.50 ms\n" +
	"Trace Stopped: Destination Reached at Hop 2 (ICMP: Echo Reply)\n"

var publicResolver = func(context.Context, string) ([]string, error) {
	return []string{"1.1.1.1"}, nil
}

func serviceReturning(stdout string) *Service {
	return newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, args ...string) RunResult {
		if contains(args, "-j") {
			return RunResult{Stdout: stdout}
		}
		return RunResult{Stdout: plainTextOutput}
	}, publicResolver)
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func oneHopStdout(stopReason string) string {
	return `{"Hops":[[{"Success":true,"Address":{"IP":"59.43.187.1"},"Hostname":"","TTL":2,"RTT":1500000,"Geo":{"asnumber":"4809","owner":"CHINANET","country_en":"China"}}]],"StopReason":{"hop":2,"reason":"` + stopReason + `"}}`
}

var target = Target{ISP: IspTelecom, Host: "ipv4.pek-4134.endpoint.nxtrace.org"}

func TestMarksTraceWithoutResponsesUnsuccessful(t *testing.T) {
	service := serviceReturning(`{"Hops":[[{"Success":false,"Address":null,"Hostname":"","TTL":1,"RTT":0,"Geo":null}]],"StopReason":{"hop":1,"reason":"max_hops"}}`)

	probes, err := service.Run(context.Background(), []Target{target})
	if err != nil {
		t.Fatal(err)
	}
	probe := probes[0]
	if probe.Success {
		t.Fatal("success = true, want false")
	}
	if probe.Error == nil || *probe.Error != "nexttrace-tiny did not receive any route responses." {
		t.Fatalf("error = %v", probe.Error)
	}
}

func TestReportsSuccessWhenDestinationReached(t *testing.T) {
	service := serviceReturning(oneHopStdout("destination_reached"))

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if !probe.Success || probe.Error != nil {
		t.Fatalf("success = %v, error = %v", probe.Success, probe.Error)
	}
	if probe.Hops[0].ASN == nil || *probe.Hops[0].ASN != "AS4809" {
		t.Fatalf("asn = %v", probe.Hops[0].ASN)
	}
	if probe.Hops[0].RTTMs == nil || *probe.Hops[0].RTTMs != 1.5 {
		t.Fatalf("rttMs = %v", probe.Hops[0].RTTMs)
	}
}

func TestFailsTruncatedTraceWithAnsweredHops(t *testing.T) {
	service := serviceReturning(oneHopStdout("max_hops"))

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if probe.Success {
		t.Fatal("success = true, want false")
	}
	if probe.Error == nil || !strings.Contains(*probe.Error, "stopped before reaching the destination (max_hops)") {
		t.Fatalf("error = %v", probe.Error)
	}
}

func TestAcceptsLastRespondingHopEqualToCheckpoint(t *testing.T) {
	service := serviceReturning(`{"Hops":[[{"Success":true,"Address":{"IP":"1.1.1.1"},"Hostname":"","TTL":3,"RTT":900000,"Geo":{"asnumber":"4809"}}]],"StopReason":{"hop":3,"reason":"max_hops"}}`)

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if !probe.Success || probe.Error != nil {
		t.Fatalf("success = %v, error = %v", probe.Success, probe.Error)
	}
}

func TestTracesValidatedAddressWithTracemapDisabled(t *testing.T) {
	var calls [][]string
	var mutex sync.Mutex
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, args ...string) RunResult {
		mutex.Lock()
		calls = append(calls, append([]string{}, args...))
		mutex.Unlock()
		return RunResult{Stdout: oneHopStdout("destination_reached")}
	}, publicResolver)

	if _, err := service.Run(context.Background(), []Target{target}); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	for _, call := range calls {
		if !contains(call, "-M") {
			t.Fatalf("missing -M in %v", call)
		}
		last := call[len(call)-1]
		if last != "1.1.1.1" {
			t.Fatalf("last argument = %q, want validated address", last)
		}
		if contains(call, target.Host) {
			t.Fatalf("hostname leaked into %v", call)
		}
	}
}

func TestRawOutputUsesPlainTextPass(t *testing.T) {
	var calls [][]string
	var mutex sync.Mutex
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, args ...string) RunResult {
		mutex.Lock()
		calls = append(calls, append([]string{}, args...))
		mutex.Unlock()
		if contains(args, "-j") {
			return RunResult{Stdout: oneHopStdout("destination_reached")}
		}
		return RunResult{Stdout: plainTextOutput}
	}, publicResolver)

	probes, _ := service.Run(context.Background(), []Target{target})
	mutex.Lock()
	defer mutex.Unlock()
	probe := probes[0]
	if !strings.HasPrefix(probe.RawOutput, "NextTrace v1.7.4") {
		t.Fatalf("rawOutput = %q", probe.RawOutput)
	}
	if !strings.Contains(probe.RawOutput, "Trace Stopped: Destination Reached") {
		t.Fatalf("rawOutput missing stop line: %q", probe.RawOutput)
	}
	if strings.Contains(probe.RawOutput, `"StopReason"`) {
		t.Fatalf("rawOutput contains JSON: %q", probe.RawOutput)
	}

	var jsonCall, displayCall []string
	for _, call := range calls {
		if contains(call, "-j") {
			jsonCall = call
		} else {
			displayCall = call
		}
	}
	if jsonCall == nil || displayCall == nil {
		t.Fatalf("calls = %v", calls)
	}
	if !contains(displayCall, "-C") {
		t.Fatalf("display call missing -C: %v", displayCall)
	}
}

func TestSurfacesExecutableStderr(t *testing.T) {
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, args ...string) RunResult {
		return RunResult{
			Err:    errors.New("exit status 1"),
			Stderr: "\x1b[31m(InitICMP) ListenPacket(ip4:icmp) failed: operation not permitted\x1b[0m\n",
		}
	}, publicResolver)

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if probe.Success || probe.Error == nil {
		t.Fatalf("success = %v, error = %v", probe.Success, probe.Error)
	}
	if *probe.Error != "(InitICMP) ListenPacket(ip4:icmp) failed: operation not permitted" {
		t.Fatalf("error = %q", *probe.Error)
	}
}

func TestRefusesPrivateCheckpointResolution(t *testing.T) {
	executed := 0
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, _ ...string) RunResult {
		executed++
		return RunResult{}
	}, func(context.Context, string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	})

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if probe.Success {
		t.Fatal("success = true, want false")
	}
	if probe.Error == nil || *probe.Error != "Checkpoint resolves to a private or reserved address." {
		t.Fatalf("error = %v", probe.Error)
	}
	if len(probe.Hops) != 0 {
		t.Fatalf("hops = %v", probe.Hops)
	}
	if executed != 0 {
		t.Fatalf("executed = %d, want 0", executed)
	}
}

func TestReportsUnresolvableCheckpoint(t *testing.T) {
	service := newServiceWithDeps("nexttrace-tiny", nil, func(context.Context, string) ([]string, error) {
		return nil, errors.New("getaddrinfo ENOTFOUND missing.example")
	})

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if probe.Error == nil || !strings.Contains(*probe.Error, "ENOTFOUND") {
		t.Fatalf("error = %v", probe.Error)
	}
}

func TestReportsMissingBinary(t *testing.T) {
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, _ ...string) RunResult {
		return RunResult{Err: fs.ErrNotExist}
	}, publicResolver)

	probes, _ := service.Run(context.Background(), []Target{target})
	probe := probes[0]
	if probe.Error == nil || *probe.Error != "nexttrace-tiny is not installed on this node." {
		t.Fatalf("error = %v", probe.Error)
	}
}

func TestReportsTimeout(t *testing.T) {
	service := newServiceWithDeps("nexttrace-tiny", func(_ context.Context, _ string, _ ...string) RunResult {
		return RunResult{Err: context.DeadlineExceeded, TimedOut: true}
	}, publicResolver)

	probes, _ := service.Run(context.Background(), []Target{target})
	if probes[0].Error == nil || *probes[0].Error != "nexttrace-tiny timed out." {
		t.Fatalf("error = %v", probes[0].Error)
	}
}

func TestValidateTargetsRejectsUnsafeInput(t *testing.T) {
	cases := []struct {
		name    string
		targets []Target
	}{
		{"empty", nil},
		{"too many", []Target{
			{ISP: IspTelecom, Host: "a.example"},
			{ISP: IspUnicom, Host: "b.example"},
			{ISP: IspMobile, Host: "c.example"},
			{ISP: IspTelecom, Host: "d.example"},
		}},
		{"private ipv4", []Target{{ISP: IspTelecom, Host: "192.168.1.1"}}},
		{"reserved ipv4", []Target{{ISP: IspTelecom, Host: "198.18.0.1"}}},
		{"legacy inet_aton", []Target{{ISP: IspTelecom, Host: "127.1"}}},
		{"private suffix", []Target{{ISP: IspTelecom, Host: "router.local"}}},
		{"url", []Target{{ISP: IspTelecom, Host: "https://example.com"}}},
		{"bad isp", []Target{{ISP: Isp("other"), Host: "example.com"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := ValidateTargets(testCase.targets); err == nil {
				t.Fatalf("ValidateTargets(%v) accepted unsafe input", testCase.targets)
			}
		})
	}
}

func TestDecodeJSONAcceptsStringAddress(t *testing.T) {
	parsed, err := decodeJSON([]byte(`{"Hops":[[{"Success":true,"Address":"59.43.187.1","TTL":1}]]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Hops) != 1 || parsed.Hops[0].IP == nil || *parsed.Hops[0].IP != "59.43.187.1" {
		t.Fatalf("hops = %+v", parsed.Hops)
	}
}
