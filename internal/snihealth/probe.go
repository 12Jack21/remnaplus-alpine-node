package snihealth

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type ResolvedAddress struct {
	Address string
	Family  int
}

type TLSObservation struct {
	HandshakeSucceeded   bool
	Authorized           bool
	AuthorizationError   string
	Protocol             string
	ALPN                 string
	CertificateValidFrom *time.Time
	CertificateValidTo   *time.Time
	CertificateNames     []string
	LatencyMS            *int
	Error                string
}

type ProbeAdapters struct {
	Resolve func(context.Context, string) ([]ResolvedAddress, error)
	TCP     func(context.Context, string, int, time.Duration) error
	TLS     func(context.Context, string, string, int, time.Duration) TLSObservation
	Xray    func(context.Context, string, string, int) XrayResult
}

type ProbeLimits struct {
	AddressLimit       int
	AddressConcurrency int
	NativeTimeout      time.Duration
	OverallTimeout     time.Duration
	LatencyWarningMS   int
	JitterWarningMS    int
}

func DefaultProbeLimits() ProbeLimits {
	return ProbeLimits{
		AddressLimit: 8, AddressConcurrency: 4, NativeTimeout: 5 * time.Second,
		OverallTimeout: 45 * time.Second, LatencyWarningMS: 1000, JitterWarningMS: 500,
	}
}

type ProbeEngine struct {
	adapters ProbeAdapters
	limits   ProbeLimits
	now      func() time.Time
}

func NewProbeEngine(adapters ProbeAdapters, limits ProbeLimits, now func() time.Time) *ProbeEngine {
	if now == nil {
		now = time.Now
	}
	return &ProbeEngine{adapters: adapters, limits: limits, now: now}
}

func (e *ProbeEngine) Probe(parent context.Context, target Target) Result {
	ctx, cancel := context.WithTimeout(parent, e.limits.OverallTimeout)
	defer cancel()
	resultChannel := make(chan Result, 1)
	go func() { resultChannel <- e.probe(ctx, target) }()
	select {
	case result := <-resultChannel:
		if ctx.Err() == context.DeadlineExceeded {
			return e.timeoutResult(target)
		}
		return result
	case <-ctx.Done():
		// Bounded adapters observe cancellation. Waiting here prevents leaked probes from
		// surviving beyond the response that reports the composite timeout.
		<-resultChannel
		return e.timeoutResult(target)
	}
}

type addressProbe struct {
	resolved ResolvedAddress
	address  Address
	tcpOK    bool
	xray     *XrayResult
	tls      *TLSObservation
}

func (e *ProbeEngine) probe(ctx context.Context, target Target) Result {
	checks := []Check{newCheck(
		"destination-sni-match", boolStatus(target.DestinationMatchesSNI), target.DestinationMatchesSNI,
		map[bool]string{true: "Destination hostname matches the configured SNI.", false: "Destination hostname and configured SNI differ."}[target.DestinationMatchesSNI],
	)}
	if !target.DestinationMatchesSNI || target.NormalizedSNI == "" || target.NormalizedHost == "" {
		return e.finish(target, checks, nil, nil, nil)
	}

	resolved, err := e.adapters.Resolve(ctx, target.Host)
	if err != nil {
		checks = append(checks, newCheck("dns-resolution", StatusFail, nil, err.Error()))
		return e.finish(target, checks, nil, nil, nil)
	}
	unique := deduplicateAddresses(resolved)
	addressValues := make([]string, len(unique))
	for index, item := range unique {
		addressValues[index] = item.Address
	}
	dnsStatus := StatusPass
	reason := fmt.Sprintf("Resolved %d address(es).", len(unique))
	if len(unique) == 0 {
		dnsStatus = StatusFail
		reason = "DNS returned no addresses."
	}
	checks = append(checks, newCheck("dns-resolution", dnsStatus, addressValues, reason))

	inspected := unique
	if len(inspected) > e.limits.AddressLimit {
		inspected = inspected[:e.limits.AddressLimit]
	}
	omitted := len(unique) - len(inspected)
	probes := e.probeAddresses(ctx, target, inspected)
	addresses := make([]Address, len(probes))
	publicValues := make([]string, 0, len(probes))
	for index, probe := range probes {
		addresses[index] = probe.address
		if probe.address.IsPublic {
			publicValues = append(publicValues, probe.resolved.Address)
		}
	}
	if len(publicValues) == 0 {
		checks = append(checks, newCheck("public-address", StatusFail, publicValues, "No resolved destination address is public."))
		return e.finish(target, checks, addresses, nil, nil)
	}
	checks = append(checks, newCheck("public-address", StatusPass, publicValues, "At least one resolved destination address is public."))

	reachable := make([]string, 0)
	xraySuccess := 0
	xrayUnavailable := false
	tlsVersions := make([]string, 0)
	alpns := make([]string, 0)
	var selected *addressProbe
	var selectedXray *XrayResult
	var selectedTLS *TLSObservation
	for index := range probes {
		probe := &probes[index]
		if probe.tcpOK {
			reachable = append(reachable, probe.resolved.Address)
		}
		if probe.xray != nil {
			if probe.xray.HandshakeSucceeded {
				xraySuccess++
			}
			if probe.xray.CommandError != "" {
				xrayUnavailable = true
			}
			if selectedXray == nil {
				selectedXray = probe.xray
			}
		}
		if probe.tls != nil {
			if probe.tls.HandshakeSucceeded && probe.tls.Protocol != "" {
				tlsVersions = append(tlsVersions, probe.tls.Protocol)
			}
			if probe.tls.HandshakeSucceeded && probe.tls.ALPN != "" {
				alpns = append(alpns, probe.tls.ALPN)
			}
			if selectedTLS == nil {
				selectedTLS = probe.tls
			}
		}
		if selected == nil && probe.tcpOK && probe.xray != nil && probe.xray.HandshakeSucceeded && probe.tls != nil && probe.tls.HandshakeSucceeded && probe.tls.Authorized && probe.tls.Protocol == "TLSv1.3" {
			selected = probe
			selectedXray = probe.xray
			selectedTLS = probe.tls
		}
	}
	checks = append(checks, newCheck("tcp-reachability", passWhen(len(reachable) > 0), reachable, conditionalReason(len(reachable) > 0, "At least one public destination address accepted TCP connections.", "No public destination address accepted a TCP connection.")))
	xrayStatus := passWhen(xraySuccess > 0)
	if xraySuccess == 0 && xrayUnavailable {
		xrayStatus = StatusUnavailable
	}
	checks = append(checks,
		newCheck("xray-capability", xrayStatus, xraySuccess, conditionalReason(xraySuccess > 0, "Xray tls ping completed successfully.", firstXrayError(probes))),
		newCheck("xray-handshake", xrayStatus, xraySuccess, conditionalReason(xraySuccess > 0, "Xray completed the SNI handshake.", "Xray did not complete the SNI handshake.")),
		newCheck("tls-version", passWhen(contains(tlsVersions, "TLSv1.3")), tlsVersions, conditionalReason(contains(tlsVersions, "TLSv1.3"), "TLS 1.3 was negotiated.", "TLS 1.3 was not negotiated.")),
	)
	alpnOK := len(alpns) == 0 || contains(alpns, "h2") || contains(alpns, "http/1.1")
	alpnStatus := StatusPass
	if !alpnOK {
		alpnStatus = StatusWarning
	}
	checks = append(checks, newCheck("alpn", alpnStatus, alpns, conditionalReason(alpnOK, "An accepted or empty ALPN was negotiated.", "The negotiated ALPN is non-standard for this probe.")))
	checks = append(checks, e.securityChecks(target, selectedXray, selectedTLS)...)

	latencies := make([]int, 0, 3)
	if selected != nil && selected.tls.LatencyMS != nil {
		latencies = append(latencies, *selected.tls.LatencyMS)
		for sample := 0; sample < 2; sample++ {
			observation := e.adapters.TLS(ctx, selected.resolved.Address, target.NormalizedSNI, target.Port, e.limits.NativeTimeout)
			if observation.HandshakeSucceeded && observation.LatencyMS != nil {
				latencies = append(latencies, *observation.LatencyMS)
			}
		}
	}
	median := medianInt(latencies)
	jitter := rangeInt(latencies)
	checks = append(checks,
		newCheck("tls-samples", passWhen(len(latencies) == 3), len(latencies), fmt.Sprintf("Collected %d complete TLS sample(s).", len(latencies))),
		newCheck("latency", metricStatus(len(latencies), median, e.limits.LatencyWarningMS), nullableInt(median), metricReason("Median TLS handshake latency", median)),
		newCheck("jitter", metricStatus(len(latencies), jitter, e.limits.JitterWarningMS), nullableInt(jitter), metricReason("TLS jitter", jitter)),
	)
	coverageWarning := omitted > 0
	for _, address := range addresses {
		coverageWarning = coverageWarning || address.Status != StatusPass
	}
	coverageStatus := StatusPass
	if coverageWarning {
		coverageStatus = StatusWarning
	}
	checks = append(checks, newCheck("address-coverage", coverageStatus, omitted, fmt.Sprintf("Address coverage completed with %d omitted.", omitted)))
	return e.finish(target, checks, addresses, latencies, median)
}

func (e *ProbeEngine) probeAddresses(ctx context.Context, target Target, values []ResolvedAddress) []addressProbe {
	result := make([]addressProbe, len(values))
	jobs := make(chan int)
	workers := e.limits.AddressConcurrency
	if workers > len(values) {
		workers = len(values)
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				result[index] = e.probeAddress(ctx, target, values[index])
			}
		}()
	}
	for index := range values {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	return result
}

func (e *ProbeEngine) probeAddress(ctx context.Context, target Target, resolved ResolvedAddress) addressProbe {
	probe := addressProbe{resolved: resolved, address: Address{Address: resolved.Address, Family: resolved.Family}}
	probe.address.IsPublic = IsPublicAddress(resolved.Address)
	if !probe.address.IsPublic {
		probe.address.Status = StatusFail
		probe.address.Error = textPtr("Resolved address is private or reserved.")
		return probe
	}
	if err := e.adapters.TCP(ctx, resolved.Address, target.Port, e.limits.NativeTimeout); err != nil {
		probe.address.Status = StatusFail
		probe.address.Error = textPtr(err.Error())
		return probe
	}
	probe.tcpOK = true
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		value := e.adapters.Xray(ctx, resolved.Address, target.NormalizedSNI, target.Port)
		probe.xray = &value
	}()
	go func() {
		defer wait.Done()
		value := e.adapters.TLS(ctx, resolved.Address, target.NormalizedSNI, target.Port, e.limits.NativeTimeout)
		probe.tls = &value
	}()
	wait.Wait()
	passed := probe.xray.HandshakeSucceeded && probe.tls.HandshakeSucceeded
	probe.address.Status = passWhen(passed)
	if !passed {
		message := firstNonempty(probe.xray.CommandError, probe.tls.Error, "TLS probe failed.")
		probe.address.Error = &message
	}
	return probe
}

func (e *ProbeEngine) securityChecks(target Target, xray *XrayResult, observation *TLSObservation) []Check {
	keyExchange := ""
	if xray != nil {
		keyExchange = xray.KeyExchange
	}
	keyStatus := StatusUnavailable
	if keyExchange == "X25519" || keyExchange == "X25519MLKEM768" {
		keyStatus = StatusPass
	} else if keyExchange == "RSA Exchange" {
		keyStatus = StatusFail
	}
	authorized := observation != nil && observation.Authorized
	valid := observation != nil && observation.CertificateValidFrom != nil && observation.CertificateValidTo != nil && !e.now().Before(*observation.CertificateValidFrom) && !e.now().After(*observation.CertificateValidTo)
	covered := observation != nil && certificateCovers(observation.CertificateNames, target.NormalizedSNI)
	return []Check{
		newCheck("key-exchange", keyStatus, nullableString(keyExchange), conditionalReason(keyStatus == StatusPass, "A supported key exchange was negotiated.", "A supported key exchange was not reported.")),
		newCheck("certificate-trust", passWhen(authorized), authorizationValue(observation), conditionalReason(authorized, "The certificate chains to the node trust store.", "The certificate is not trusted.")),
		newCheck("certificate-validity", passWhen(valid), certificateWindow(observation), conditionalReason(valid, "The certificate is currently within its validity dates.", "The certificate is expired, not yet valid, or missing validity dates.")),
		newCheck("certificate-san", passWhen(covered), certificateNames(observation), conditionalReason(covered, "The certificate SAN covers the configured SNI.", "The certificate SAN does not cover the configured SNI.")),
	}
}

func (e *ProbeEngine) finish(target Target, checks []Check, addresses []Address, latencies []int, latency *int) Result {
	if addresses == nil {
		addresses = []Address{}
	}
	if latencies == nil {
		latencies = []int{}
	}
	unavailable := false
	independentFailure := false
	warning := false
	var firstProblem *string
	for _, item := range checks {
		if item.Status == StatusUnavailable {
			unavailable = true
		}
		if item.Status == StatusFail && item.ID != "jitter" && item.ID != "latency" && item.ID != "tls-samples" {
			independentFailure = true
		}
		if item.Status == StatusWarning && item.ID != "alpn" {
			warning = true
		}
		if firstProblem == nil && (item.Status == StatusFail || (item.Status == StatusWarning && item.ID != "alpn")) {
			value := item.Reason
			firstProblem = &value
		}
	}
	verdict := VerdictUsable
	if unavailable && !independentFailure {
		verdict = VerdictUnverified
	} else if hasStatus(checks, StatusFail) {
		verdict = VerdictUnusable
	} else if warning {
		verdict = VerdictWarning
	}
	return Result{
		CorrelationID: target.CorrelationID, InboundTag: target.InboundTag, SNI: target.SNI, Dest: target.Dest,
		Hostname: firstNonempty(target.NormalizedSNI, target.SNI), Port: target.Port, Verdict: verdict, Healthy: verdict == VerdictUsable,
		LatencyMS: latency, Error: firstProblem, Checks: checks, Addresses: addresses, LatencySamples: latencies,
		JitterMS: rangeInt(latencies), CheckedAt: e.now(),
	}
}

func (e *ProbeEngine) timeoutResult(target Target) Result {
	return e.finish(target, []Check{newCheck("probe-timeout", StatusUnavailable, nil, "The composite probe exceeded its 45-second deadline.")}, nil, nil, nil)
}

func NativeProbeAdapters(xray *XrayPinger) ProbeAdapters {
	return ProbeAdapters{
		Resolve: func(ctx context.Context, hostname string) ([]ResolvedAddress, error) {
			values, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
			result := make([]ResolvedAddress, 0, len(values))
			for _, value := range values {
				family := 6
				if value.IP.To4() != nil {
					family = 4
				}
				result = append(result, ResolvedAddress{Address: value.IP.String(), Family: family})
			}
			return result, err
		},
		TCP: func(ctx context.Context, address string, port int, timeout time.Duration) error {
			connection, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(address, fmt.Sprint(port)))
			if err == nil {
				_ = connection.Close()
			}
			return err
		},
		TLS:  nativeTLSProbe,
		Xray: xray.Ping,
	}
}

func nativeTLSProbe(ctx context.Context, address, hostname string, port int, timeout time.Duration) TLSObservation {
	started := time.Now()
	dialer := &net.Dialer{Timeout: timeout}
	rawConnection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, fmt.Sprint(port)))
	if err != nil {
		return TLSObservation{Error: err.Error(), AuthorizationError: err.Error()}
	}
	connection := tls.Client(rawConnection, &tls.Config{
		ServerName: hostname, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2", "http/1.1"},
	})
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(timeout)); err != nil {
		return TLSObservation{Error: err.Error(), AuthorizationError: err.Error()}
	}
	if err := connection.HandshakeContext(ctx); err != nil {
		return TLSObservation{Error: err.Error(), AuthorizationError: err.Error()}
	}
	state := connection.ConnectionState()
	latency := int(time.Since(started).Round(time.Millisecond) / time.Millisecond)
	observation := TLSObservation{HandshakeSucceeded: true, Authorized: len(state.VerifiedChains) > 0, Protocol: tlsVersionName(state.Version), ALPN: state.NegotiatedProtocol, LatencyMS: &latency, CertificateNames: []string{}}
	if len(state.PeerCertificates) > 0 {
		certificate := state.PeerCertificates[0]
		observation.CertificateValidFrom = &certificate.NotBefore
		observation.CertificateValidTo = &certificate.NotAfter
		observation.CertificateNames = append(observation.CertificateNames, certificate.DNSNames...)
	}
	return observation
}

func tlsVersionName(version uint16) string {
	if version == tls.VersionTLS13 {
		return "TLSv1.3"
	}
	return fmt.Sprintf("0x%x", version)
}
func newCheck(id string, status CheckStatus, value any, reason string) Check {
	return Check{ID: id, Status: status, ObservedValue: value, Reason: reason, Hint: textPtr(hintFor(id))}
}
func hintFor(id string) string          { return "Review the target configuration and retry this check." }
func boolStatus(value bool) CheckStatus { return passWhen(value) }
func passWhen(value bool) CheckStatus {
	if value {
		return StatusPass
	}
	return StatusFail
}
func conditionalReason(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func deduplicateAddresses(values []ResolvedAddress) []ResolvedAddress {
	seen := map[string]struct{}{}
	result := make([]ResolvedAddress, 0, len(values))
	for _, value := range values {
		key := fmt.Sprintf("%d:%s", value.Family, value.Address)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}
func firstXrayError(probes []addressProbe) string {
	for _, probe := range probes {
		if probe.xray != nil && probe.xray.CommandError != "" {
			return probe.xray.CommandError
		}
	}
	return "Xray tls ping did not complete a handshake."
}
func medianInt(values []int) *int {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	value := sorted[len(sorted)/2]
	return &value
}
func rangeInt(values []int) *int {
	if len(values) == 0 {
		return nil
	}
	minimum, maximum := values[0], values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	result := maximum - minimum
	return &result
}
func metricStatus(samples int, value *int, warning int) CheckStatus {
	if samples < 3 || value == nil {
		return StatusFail
	}
	if *value > warning {
		return StatusWarning
	}
	return StatusPass
}
func metricReason(label string, value *int) string {
	if value == nil {
		return label + " was unavailable."
	}
	return fmt.Sprintf("%s was %d ms.", label, *value)
}
func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func authorizationValue(value *TLSObservation) any {
	if value == nil || value.AuthorizationError == "" {
		return nil
	}
	return value.AuthorizationError
}
func certificateWindow(value *TLSObservation) any {
	if value == nil || value.CertificateValidFrom == nil || value.CertificateValidTo == nil {
		return nil
	}
	return value.CertificateValidFrom.Format(time.RFC3339) + " - " + value.CertificateValidTo.Format(time.RFC3339)
}
func certificateNames(value *TLSObservation) any {
	if value == nil {
		return nil
	}
	return value.CertificateNames
}
func certificateCovers(names []string, hostname string) bool {
	hostname = strings.ToLower(hostname)
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == hostname {
			return true
		}
		if strings.HasPrefix(name, "*.") {
			suffix := name[1:]
			if strings.HasSuffix(hostname, suffix) {
				prefix := strings.TrimSuffix(hostname, suffix)
				if prefix != "" && !strings.Contains(prefix, ".") {
					return true
				}
			}
		}
	}
	return false
}
func hasStatus(checks []Check, status CheckStatus) bool {
	for _, check := range checks {
		if check.Status == status {
			return true
		}
	}
	return false
}
func textPtr(value string) *string { return &value }
