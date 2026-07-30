package snihealth

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type XrayEvidence struct {
	HandshakeSucceeded bool
	TLSVersion         string
	KeyExchange        string
	PostQuantum        *bool
	CertificateDomains []string
	Diagnostic         string
}

type XrayResult struct {
	XrayEvidence
	CommandError string
	TimedOut     bool
}

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type XrayPinger struct {
	binary string
	runner CommandRunner
}

func NewXrayPinger(binary string, runner CommandRunner) *XrayPinger {
	if runner == nil {
		runner = runBoundedCommand
	}
	return &XrayPinger{binary: binary, runner: runner}
}

func (p *XrayPinger) Ping(ctx context.Context, address, hostname string, port int) XrayResult {
	normalized, err := NormalizeHostname(hostname)
	if err != nil || !IsPublicAddress(address) || port < 1 || port > 65535 {
		return failedXrayResult("Invalid public Xray TLS ping target.", false)
	}
	commandContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, commandErr := p.runner(commandContext, p.binary, "tls", "ping", "-ip", address, normalized+":"+strconv.Itoa(port))
	evidence := ParseXrayTLSPingOutput(string(output))
	if commandErr == nil {
		return XrayResult{XrayEvidence: evidence}
	}
	diagnostic := SanitizeDiagnostic(string(output))
	if diagnostic == "" {
		diagnostic = SanitizeDiagnostic(commandErr.Error())
	}
	if diagnostic == "" {
		diagnostic = "Xray TLS ping failed."
	}
	evidence.HandshakeSucceeded = false
	if evidence.Diagnostic == "" {
		evidence.Diagnostic = diagnostic
	}
	return XrayResult{XrayEvidence: evidence, CommandError: diagnostic, TimedOut: commandContext.Err() == context.DeadlineExceeded}
}

func failedXrayResult(message string, timedOut bool) XrayResult {
	return XrayResult{XrayEvidence: XrayEvidence{CertificateDomains: []string{}, Diagnostic: message}, CommandError: message, TimedOut: timedOut}
}

func runBoundedCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	buffer := &boundedBuffer{limit: 64 * 1024}
	command.Stdout = buffer
	command.Stderr = buffer
	err := command.Run()
	return buffer.Bytes(), err
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buffer.Write(value)
	}
	return written, nil
}

func (b *boundedBuffer) Bytes() []byte { return b.buffer.Bytes() }

func ParseXrayTLSPingOutput(output string) XrayEvidence {
	section := ""
	if start := strings.Index(strings.ToLower(output), "pinging with sni"); start >= 0 {
		section = output[start+len("Pinging with SNI"):]
		lower := strings.ToLower(section)
		end := len(section)
		for _, marker := range []string{"\n---", "\ntls ping finished"} {
			if index := strings.Index(lower, marker); index >= 0 && index < end {
				end = index
			}
		}
		section = section[:end]
	}
	evidence := XrayEvidence{CertificateDomains: []string{}}
	evidence.HandshakeSucceeded = regexp.MustCompile(`(?i)handshake succeeded`).MatchString(section)
	evidence.TLSVersion = captureLine(section, `(?i)TLS Version:\s*([^\r\n]+)`)
	keyExchange := captureLine(section, `(?i)TLS Post-Quantum key exchange:\s*[^\r\n]*\(([^)]+)\)`)
	switch keyExchange {
	case "X25519", "X25519MLKEM768", "RSA Exchange":
		evidence.KeyExchange = keyExchange
	}
	postQuantum := captureLine(section, `(?i)TLS Post-Quantum key exchange:\s*(true|false)`)
	if postQuantum != "" {
		value := strings.EqualFold(postQuantum, "true")
		evidence.PostQuantum = &value
	}
	domains := captureLine(section, `(?i)Cert's allowed domains:\s*\[([^\]]*)\]`)
	if domains != "" {
		evidence.CertificateDomains = strings.Fields(domains)
		if len(evidence.CertificateDomains) > 64 {
			evidence.CertificateDomains = evidence.CertificateDomains[:64]
		}
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "handshake succeeded") || strings.HasPrefix(lower, "tls version:") || strings.HasPrefix(lower, "tls post-quantum") || strings.HasPrefix(lower, "cert'") {
			continue
		}
		lines = append(lines, line)
	}
	evidence.Diagnostic = SanitizeDiagnostic(strings.Join(lines, "\n"))
	return evidence
}

func captureLine(value, pattern string) string {
	match := regexp.MustCompile(pattern).FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func SanitizeDiagnostic(value string) string {
	var result strings.Builder
	for _, character := range strings.ReplaceAll(value, "\r", "") {
		if (character >= 0 && character <= 8) || character == 11 || character == 12 || (character >= 14 && character <= 31) || character == 127 {
			continue
		}
		if result.Len()+len(string(character)) > 2048 {
			break
		}
		result.WriteRune(character)
	}
	return strings.TrimSpace(result.String())
}
