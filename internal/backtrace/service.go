package backtrace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/snihealth"
)

const (
	defaultBinary  = "/usr/local/bin/nexttrace-tiny"
	commandTimeout = 90 * time.Second
	dnsTimeout     = 5 * time.Second
	maxOutputBytes = 4 * 1024 * 1024
	rawOutputLimit = 1_000_000
	maxHops        = 64
	MaxTargets     = 3
)

// nexttrace-tiny reports the stop reason as "destination_reached" or
// "max_hops". Older builds omit Error information, so anything other than a
// confirmed destination hit is treated as an incomplete trace.
var destinationReachedPattern = regexp.MustCompile(`(?i)destination_reached|reached destination`)

// Runner executes one nexttrace-tiny invocation.
type Runner func(ctx context.Context, binary string, args ...string) RunResult

// Resolver resolves a checkpoint hostname.
type Resolver func(ctx context.Context, host string) ([]string, error)

// RunResult is the raw outcome of one nexttrace-tiny invocation.
type RunResult struct {
	Stdout   string
	Stderr   string
	Err      error
	TimedOut bool
}

// Service runs three-network backtrace probes for the dashboard.
type Service struct {
	binary    string
	run       Runner
	resolve   Resolver
	semaphore chan struct{}
}

// NewService builds the production service.
func NewService(binary string) *Service {
	if strings.TrimSpace(binary) == "" {
		binary = defaultBinary
	}
	return &Service{
		binary:    binary,
		run:       runCommand,
		resolve:   lookupHost,
		semaphore: make(chan struct{}, 1),
	}
}

func newServiceWithDeps(binary string, runner Runner, resolver Resolver) *Service {
	service := NewService(binary)
	if runner != nil {
		service.run = runner
	}
	if resolver != nil {
		service.resolve = resolver
	}
	return service
}

// ValidateTargets enforces the dashboard contract before any probe runs.
func ValidateTargets(targets []Target) ([]Target, error) {
	if len(targets) == 0 || len(targets) > MaxTargets {
		return nil, fmt.Errorf("Targets must contain between 1 and %d checkpoints.", MaxTargets)
	}

	validated := make([]Target, 0, len(targets))
	for _, target := range targets {
		switch target.ISP {
		case IspTelecom, IspUnicom, IspMobile:
		default:
			return nil, fmt.Errorf("ISP must be telecom, unicom or mobile.")
		}
		host, err := NormalizeTarget(target.Host)
		if err != nil {
			return nil, err
		}
		validated = append(validated, Target{ISP: target.ISP, Host: host})
	}
	return validated, nil
}

// Run traces every target, one checkpoint batch at a time.
func (s *Service) Run(ctx context.Context, targets []Target) ([]Probe, error) {
	validated, err := ValidateTargets(targets)
	if err != nil {
		return nil, err
	}

	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	probes := make([]Probe, len(validated))
	var waitGroup sync.WaitGroup
	for index, target := range validated {
		waitGroup.Add(1)
		go func(index int, target Target) {
			defer waitGroup.Done()
			probes[index] = s.trace(ctx, target)
		}(index, target)
	}
	waitGroup.Wait()
	return probes, nil
}

func (s *Service) trace(ctx context.Context, target Target) Probe {
	startedAt := time.Now()
	probe := Probe{
		ISP:        target.ISP,
		Checkpoint: target.Host,
		Hops:       []Hop{},
		RawOutput:  "",
	}

	address, resolutionErr := s.resolveCheckpoint(ctx, target.Host)
	if resolutionErr != nil {
		probe.DurationMs = time.Since(startedAt).Milliseconds()
		probe.Error = stringPointer(resolutionErr.Error())
		return probe
	}

	// -M keeps nexttrace from calling api.nxtrace.org for the tracemap:
	// when that call fails it prints the error instead of JSON, which would
	// fail every probe on a node with restricted egress. The validated
	// address (not the hostname) is traced so nexttrace cannot re-resolve it
	// into a private address (DNS rebinding).
	commonArgs := []string{"-4", "-k", "-n", "-q", "1", "-m", "30", "-M", "--timeout", "1000"}

	// The dashboard's raw section is the literal output of nexttrace-tiny
	// run without --json, so the human-readable pass runs alongside the JSON
	// pass. Both stay under the per-command timeout.
	var structured, display RunResult
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		structured = s.run(ctx, s.binary, append(commonArgs, "-j", address)...)
	}()
	go func() {
		defer waitGroup.Done()
		display = s.run(ctx, s.binary, append(commonArgs, "-C", address)...)
	}()
	waitGroup.Wait()

	probe.DurationMs = time.Since(startedAt).Milliseconds()
	probe.RawOutput = truncate(strings.TrimSpace(display.Stdout), rawOutputLimit)

	parsed, parseErr := decodeJSON([]byte(structured.Stdout))
	hops := parsed.Hops
	if hops == nil {
		hops = []Hop{}
	}
	probe.Hops = hops

	hasUsableHops := false
	var lastRespondingHop *Hop
	for index := range hops {
		if hops[index].Success && hops[index].IP != nil {
			hasUsableHops = true
			lastRespondingHop = &hops[index]
		}
	}
	reachedDestination := destinationReachedPattern.MatchString(parsed.StopReason) ||
		(lastRespondingHop != nil && lastRespondingHop.IP != nil && *lastRespondingHop.IP == address)

	missingBinary := errors.Is(structured.Err, fs.ErrNotExist)
	var errorMessage *string

	switch {
	case missingBinary:
		errorMessage = stringPointer("nexttrace-tiny is not installed on this node.")
	case structured.TimedOut:
		errorMessage = stringPointer("nexttrace-tiny timed out.")
	case !hasUsableHops:
		switch {
		case structured.Err != nil:
			fallback := "nexttrace-tiny failed."
			if parseErr != nil {
				fallback = parseErr.Error()
			} else if structured.Err.Error() != "" {
				fallback = structured.Err.Error()
			}
			// A non-zero exit carries the actionable cause on stderr
			// (missing capability, bad flag, resolver failure); the parse
			// error only says that stdout was not JSON.
			errorMessage = stringPointer(sanitizeError(structured.Stderr, fallback))
		case parseErr != nil:
			errorMessage = stringPointer(parseErr.Error())
		default:
			errorMessage = stringPointer("nexttrace-tiny did not receive any route responses.")
		}
	case !reachedDestination:
		// Partial routes must not be presented as a confident verdict: the
		// dashboard caches the result for six hours.
		reason := parsed.StopReason
		if reason == "" {
			reason = "unknown stop reason"
		}
		errorMessage = stringPointer(fmt.Sprintf(
			"nexttrace-tiny stopped before reaching the destination (%s).",
			truncate(reason, 128),
		))
	}

	probe.Success = hasUsableHops && errorMessage == nil
	probe.Error = errorMessage
	return probe
}

func (s *Service) resolveCheckpoint(ctx context.Context, host string) (string, error) {
	addresses, err := s.resolve(ctx, host)
	if err != nil {
		message := err.Error()
		if message == "" {
			message = "Checkpoint could not be resolved."
		}
		return "", fmt.Errorf("%s", sanitizeError("", message))
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("Checkpoint could not be resolved.")
	}

	for _, address := range addresses {
		if !snihealth.IsPublicAddress(address) {
			return "", fmt.Errorf("Checkpoint resolves to a private or reserved address.")
		}
	}

	for _, address := range addresses {
		parsed := net.ParseIP(address)
		if parsed != nil && parsed.To4() != nil {
			return parsed.To4().String(), nil
		}
	}
	return "", fmt.Errorf("Checkpoint has no public IPv4 address.")
}

func lookupHost(ctx context.Context, host string) ([]string, error) {
	lookupContext, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	addresses, err := net.DefaultResolver.LookupIPAddr(lookupContext, host)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, address.IP.String())
	}
	return result, nil
}

func runCommand(ctx context.Context, binary string, args ...string) RunResult {
	runContext, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	command := exec.CommandContext(runContext, binary, args...)
	stdout := &cappedBuffer{limit: maxOutputBytes}
	stderr := &cappedBuffer{limit: maxOutputBytes}
	command.Stdout = stdout
	command.Stderr = stderr

	err := command.Run()
	return RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
		TimedOut: errors.Is(runContext.Err(), context.DeadlineExceeded),
	}
}

type cappedBuffer struct {
	buffer   []byte
	limit    int
	overflow bool
}

func (buffer *cappedBuffer) Write(payload []byte) (int, error) {
	if len(buffer.buffer) >= buffer.limit {
		buffer.overflow = true
		return len(payload), nil
	}
	remaining := buffer.limit - len(buffer.buffer)
	if len(payload) > remaining {
		buffer.buffer = append(buffer.buffer, payload[:remaining]...)
		buffer.overflow = true
		return len(payload), nil
	}
	buffer.buffer = append(buffer.buffer, payload...)
	return len(payload), nil
}

func (buffer *cappedBuffer) String() string {
	return string(buffer.buffer)
}

func stringPointer(value string) *string {
	return &value
}
