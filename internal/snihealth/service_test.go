package snihealth

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type fakeConfigProvider struct {
	config map[string]any
	tags   []string
}

func (f *fakeConfigProvider) CurrentConfig() map[string]any { return f.config }
func (f *fakeConfigProvider) InboundTags() []string         { return f.tags }

func TestServiceCachesOnlyActiveSnapshots(t *testing.T) {
	t.Parallel()

	provider := &fakeConfigProvider{
		config: realityConfig("A", "a.example.com"),
		tags:   []string{"A"},
	}
	probed := make([]Target, 0)
	service := NewService(provider, func(_ context.Context, target Target) Result {
		probed = append(probed, target)
		return Result{CorrelationID: target.CorrelationID, InboundTag: target.InboundTag, SNI: target.SNI, Dest: target.Dest, Hostname: target.NormalizedSNI, Port: target.Port, Verdict: VerdictUsable, Healthy: true, Checks: []Check{}, Addresses: []Address{}, LatencySamples: []int{}, CheckedAt: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)}
	}, time.Now)

	active := service.RunActive(context.Background())
	if len(active) != 1 || active[0].InboundTag == nil || *active[0].InboundTag != "A" {
		t.Fatalf("active results = %+v", active)
	}
	before := service.Results()
	candidates, err := service.RunCandidates(context.Background(), []CandidateTarget{{CorrelationID: "candidate-1", Hostname: "Example.COM.", Port: 8443}})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].CorrelationID == nil || *candidates[0].CorrelationID != "candidate-1" || candidates[0].InboundTag != nil {
		t.Fatalf("candidate results = %+v", candidates)
	}
	if !reflect.DeepEqual(service.Results(), before) {
		t.Fatal("candidate probe changed active cache")
	}

	provider.tags = nil
	if results := service.RunActive(context.Background()); len(results) != 0 || len(service.Results()) != 0 {
		t.Fatalf("empty active results were not published: %+v / %+v", results, service.Results())
	}
}

func TestServiceRejectsInvalidCandidatesAndAggregates(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeConfigProvider{}, func(_ context.Context, target Target) Result {
		verdict := VerdictUsable
		if target.Port == 444 {
			verdict = VerdictWarning
		}
		return Result{CorrelationID: target.CorrelationID, SNI: target.SNI, Dest: target.Dest, Hostname: target.NormalizedSNI, Port: target.Port, Verdict: verdict, Healthy: verdict == VerdictUsable, Checks: []Check{}, Addresses: []Address{}, LatencySamples: []int{}, CheckedAt: time.Now()}
	}, time.Now)

	if _, err := service.RunCandidates(context.Background(), nil); err == nil {
		t.Fatal("empty candidate request succeeded")
	}
	if _, err := service.RunCandidates(context.Background(), []CandidateTarget{{CorrelationID: "bad id", Hostname: "example.com", Port: 443}}); err == nil {
		t.Fatal("invalid correlation ID succeeded")
	}
	results, err := service.RunCandidates(context.Background(), []CandidateTarget{
		{CorrelationID: "one", Hostname: "example.com", Port: 443},
		{CorrelationID: "two", Hostname: "example.com", Port: 444},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := BuildProbeResponse("candidates", results, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))
	if response.AggregateVerdict != VerdictWarning || response.Counts.Usable != 1 || response.Counts.Warning != 1 || response.UnhealthyCount != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func realityConfig(tag, serverName string) map[string]any {
	return map[string]any{"inbounds": []any{map[string]any{
		"tag": tag,
		"streamSettings": map[string]any{"realitySettings": map[string]any{
			"dest": serverName + ":443", "serverNames": []any{serverName},
		}},
	}}}
}
