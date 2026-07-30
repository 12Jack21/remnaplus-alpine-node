package snihealth

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

type ConfigProvider interface {
	CurrentConfig() map[string]any
	InboundTags() []string
}

type ProbeFunc func(context.Context, Target) Result

type Service struct {
	provider ConfigProvider
	probe    ProbeFunc
	now      func() time.Time

	mu      sync.RWMutex
	results []Result
}

func NewService(provider ConfigProvider, probe ProbeFunc, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{provider: provider, probe: probe, now: now, results: []Result{}}
}

func (s *Service) RunActive(ctx context.Context) []Result {
	targets := ExtractRealityTargets(s.provider.CurrentConfig(), s.provider.InboundTags())
	results := s.probeTargets(ctx, targets)
	s.mu.Lock()
	s.results = cloneResults(results)
	s.mu.Unlock()
	return results
}

var correlationIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

func (s *Service) RunCandidates(ctx context.Context, candidates []CandidateTarget) ([]Result, error) {
	if len(candidates) == 0 || len(candidates) > 64 {
		return nil, fmt.Errorf("candidate probes require between 1 and 64 targets")
	}
	targets := make([]Target, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.CorrelationID) == 0 || len(candidate.CorrelationID) > 128 || !correlationIDPattern.MatchString(candidate.CorrelationID) {
			return nil, fmt.Errorf("invalid candidate correlationId")
		}
		hostname, err := NormalizeHostname(candidate.Hostname)
		if err != nil || candidate.Port < 1 || candidate.Port > 65535 {
			return nil, fmt.Errorf("invalid candidate target")
		}
		correlationID := candidate.CorrelationID
		targets = append(targets, Target{
			Identity: "candidate\x00" + correlationID, CorrelationID: &correlationID,
			SNI: hostname, NormalizedSNI: hostname, Host: hostname, NormalizedHost: hostname,
			Port: candidate.Port, Dest: fmt.Sprintf("%s:%d", hostname, candidate.Port), DestinationMatchesSNI: true,
		})
	}
	return s.probeTargets(ctx, targets), nil
}

func (s *Service) probeTargets(ctx context.Context, targets []Target) []Result {
	results := make([]Result, len(targets))
	var wait sync.WaitGroup
	for index, target := range targets {
		wait.Add(1)
		go func() { defer wait.Done(); results[index] = s.probe(ctx, target) }()
	}
	wait.Wait()
	return results
}

func (s *Service) Results() []Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneResults(s.results)
}

func (s *Service) Now() time.Time { return s.now() }

func (s *Service) Status() StatusResponse {
	results := s.Results()
	if len(results) == 0 {
		results = s.RunActive(context.Background())
	}
	unhealthy := 0
	for _, result := range results {
		if !result.Healthy {
			unhealthy++
		}
	}
	return StatusResponse{Results: results, UnhealthyCount: unhealthy, CheckedAt: s.now()}
}

func BuildProbeResponse(mode string, results []Result, checkedAt time.Time) ProbeResponse {
	response := ProbeResponse{Mode: mode, Results: results, AggregateVerdict: VerdictUnverified, CheckedAt: checkedAt}
	for _, result := range results {
		switch result.Verdict {
		case VerdictUsable:
			response.Counts.Usable++
		case VerdictWarning:
			response.Counts.Warning++
		case VerdictUnusable:
			response.Counts.Unusable++
		case VerdictUnverified:
			response.Counts.Unverified++
		}
		if result.Verdict != VerdictUsable {
			response.UnhealthyCount++
		}
	}
	for _, verdict := range []Verdict{VerdictUnusable, VerdictUnverified, VerdictWarning, VerdictUsable} {
		for _, result := range results {
			if result.Verdict == verdict {
				response.AggregateVerdict = verdict
				return response
			}
		}
	}
	return response
}

func cloneResults(values []Result) []Result {
	result := make([]Result, len(values))
	copy(result, values)
	for index := range result {
		result[index].Checks = append([]Check(nil), values[index].Checks...)
		result[index].Addresses = append([]Address(nil), values[index].Addresses...)
		result[index].LatencySamples = append([]int(nil), values[index].LatencySamples...)
	}
	return result
}
