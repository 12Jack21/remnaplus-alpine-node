package snihealth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func healthyTarget() Target {
	tag := "main"
	return Target{
		Identity: "main\x00example.com\x00example.com\x00443", InboundTag: &tag,
		SNI: "example.com", NormalizedSNI: "example.com", Host: "example.com",
		NormalizedHost: "example.com", Port: 443, Dest: "example.com:443",
		DestinationMatchesSNI: true,
	}
}

func healthyTLS(latency int) TLSObservation {
	validFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	validTo := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	return TLSObservation{
		HandshakeSucceeded: true, Authorized: true, Protocol: "TLSv1.3", ALPN: "h2",
		CertificateValidFrom: &validFrom, CertificateValidTo: &validTo,
		CertificateNames: []string{"example.com", "*.example.com"}, LatencyMS: &latency,
	}
}

func healthyAdapters() ProbeAdapters {
	return ProbeAdapters{
		Resolve: func(context.Context, string) ([]ResolvedAddress, error) {
			return []ResolvedAddress{{Address: "93.184.216.34", Family: 4}}, nil
		},
		TCP: func(context.Context, string, int, time.Duration) error { return nil },
		TLS: func(context.Context, string, string, int, time.Duration) TLSObservation {
			return healthyTLS(40)
		},
		Xray: func(context.Context, string, string, int) XrayResult {
			postQuantum := false
			return XrayResult{XrayEvidence: XrayEvidence{
				HandshakeSucceeded: true, TLSVersion: "TLS 1.3", KeyExchange: "X25519",
				PostQuantum: &postQuantum, CertificateDomains: []string{"example.com", "*.example.com"},
			}}
		},
	}
}

func TestProbeProducesUsableResultAndThreeSamples(t *testing.T) {
	t.Parallel()

	engine := NewProbeEngine(healthyAdapters(), DefaultProbeLimits(), func() time.Time {
		return time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	})
	result := engine.Probe(context.Background(), healthyTarget())
	if result.Verdict != VerdictUsable || !result.Healthy || len(result.LatencySamples) != 3 {
		t.Fatalf("result = %+v", result)
	}
	for _, check := range result.Checks {
		if check.Status == StatusFail {
			t.Fatalf("unexpected failing check: %+v", check)
		}
	}
}

func TestProbeClassifiesPolicyCapabilityAndIndependentFailures(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC) }
	privateAdapters := healthyAdapters()
	privateAdapters.Resolve = func(context.Context, string) ([]ResolvedAddress, error) {
		return []ResolvedAddress{{Address: "192.168.1.10", Family: 4}}, nil
	}
	private := NewProbeEngine(privateAdapters, DefaultProbeLimits(), now).Probe(context.Background(), healthyTarget())
	if private.Verdict != VerdictUnusable || checkStatus(private, "public-address") != StatusFail {
		t.Fatalf("private result = %+v", private)
	}

	oldXrayAdapters := healthyAdapters()
	oldXrayAdapters.Xray = func(context.Context, string, string, int) XrayResult {
		return failedXrayResult("rw-core tls ping is unavailable", false)
	}
	oldXray := NewProbeEngine(oldXrayAdapters, DefaultProbeLimits(), now).Probe(context.Background(), healthyTarget())
	if oldXray.Verdict != VerdictUnverified || checkStatus(oldXray, "xray-capability") != StatusUnavailable {
		t.Fatalf("old rw-core result = %+v", oldXray)
	}

	mismatch := healthyTarget()
	mismatch.DestinationMatchesSNI = false
	mismatched := NewProbeEngine(healthyAdapters(), DefaultProbeLimits(), now).Probe(context.Background(), mismatch)
	if mismatched.Verdict != VerdictUnusable || checkStatus(mismatched, "destination-sni-match") != StatusFail {
		t.Fatalf("mismatch result = %+v", mismatched)
	}
}

func TestProbeBoundsAddressesConcurrencyAndOverallDeadline(t *testing.T) {
	t.Parallel()

	adapters := healthyAdapters()
	addresses := make([]ResolvedAddress, 12)
	for index := range addresses {
		addresses[index] = ResolvedAddress{Address: "1.1.1." + string(rune('1'+index)), Family: 4}
	}
	adapters.Resolve = func(context.Context, string) ([]ResolvedAddress, error) { return addresses, nil }
	var current int32
	var maximum int32
	var calls int32
	adapters.TCP = func(ctx context.Context, _ string, _ int, _ time.Duration) error {
		value := atomic.AddInt32(&current, 1)
		defer atomic.AddInt32(&current, -1)
		atomic.AddInt32(&calls, 1)
		for {
			old := atomic.LoadInt32(&maximum)
			if value <= old || atomic.CompareAndSwapInt32(&maximum, old, value) {
				break
			}
		}
		select {
		case <-time.After(5 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	result := NewProbeEngine(adapters, DefaultProbeLimits(), time.Now).Probe(context.Background(), healthyTarget())
	if len(result.Addresses) != 8 || calls != 8 || maximum > 4 || checkStatus(result, "address-coverage") != StatusWarning {
		t.Fatalf("addresses=%d calls=%d max=%d result=%+v", len(result.Addresses), calls, maximum, result)
	}

	blocked := healthyAdapters()
	var cancelled bool
	var mutex sync.Mutex
	blocked.TCP = func(ctx context.Context, _ string, _ int, _ time.Duration) error {
		<-ctx.Done()
		mutex.Lock()
		cancelled = true
		mutex.Unlock()
		return ctx.Err()
	}
	limits := DefaultProbeLimits()
	limits.OverallTimeout = 20 * time.Millisecond
	timedOut := NewProbeEngine(blocked, limits, time.Now).Probe(context.Background(), healthyTarget())
	mutex.Lock()
	wasCancelled := cancelled
	mutex.Unlock()
	if timedOut.Verdict != VerdictUnverified || checkStatus(timedOut, "probe-timeout") != StatusUnavailable || !wasCancelled {
		t.Fatalf("timeout result=%+v cancelled=%v", timedOut, wasCancelled)
	}
}

func TestProbeReportsDNSFailure(t *testing.T) {
	t.Parallel()

	adapters := healthyAdapters()
	adapters.Resolve = func(context.Context, string) ([]ResolvedAddress, error) {
		return nil, errors.New("resolver unavailable")
	}
	result := NewProbeEngine(adapters, DefaultProbeLimits(), time.Now).Probe(context.Background(), healthyTarget())
	if result.Verdict != VerdictUnusable || checkStatus(result, "dns-resolution") != StatusFail {
		t.Fatalf("result = %+v", result)
	}
}

func TestNativeTLSProbeHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	result := nativeTLSProbe(ctx, "203.0.113.1", "example.com", 443, 5*time.Second)
	if result.HandshakeSucceeded || result.Error == "" || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("cancelled TLS result = %+v after %s", result, time.Since(started))
	}
}

func checkStatus(result Result, id string) CheckStatus {
	for _, check := range result.Checks {
		if check.ID == id {
			return check.Status
		}
	}
	return ""
}
