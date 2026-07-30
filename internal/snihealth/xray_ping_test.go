package snihealth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseXrayTLSPingGoldenFixtures(t *testing.T) {
	t.Parallel()

	success := parseFixture(t, "success.txt")
	if !success.HandshakeSucceeded || success.TLSVersion != "TLS 1.3" || success.KeyExchange != "X25519MLKEM768" || success.PostQuantum == nil || !*success.PostQuantum {
		t.Fatalf("success evidence = %+v", success)
	}
	if !reflect.DeepEqual(success.CertificateDomains, []string{"example.com", "*.example.com"}) {
		t.Fatalf("certificate domains = %#v", success.CertificateDomains)
	}
	failure := parseFixture(t, "handshake-failure-exit-zero.txt")
	if failure.HandshakeSucceeded || !strings.Contains(strings.ToLower(failure.Diagnostic), "handshake failure") {
		t.Fatalf("failure evidence = %+v", failure)
	}
	rsa := parseFixture(t, "rsa-fallback.txt")
	if !rsa.HandshakeSucceeded || rsa.KeyExchange != "RSA Exchange" || rsa.PostQuantum == nil || *rsa.PostQuantum {
		t.Fatalf("RSA evidence = %+v", rsa)
	}
	sanitized := SanitizeDiagnostic(strings.Repeat("x", 5000) + "\x00\nsecret-token")
	if len(sanitized) > 2048 || strings.ContainsRune(sanitized, '\x00') {
		t.Fatalf("diagnostic was not bounded and sanitized")
	}
}

func TestXrayPingerUsesPinnedCommandAndSurfacesErrors(t *testing.T) {
	t.Parallel()

	var command string
	var args []string
	pinger := NewXrayPinger("/usr/local/bin/rw-core", func(_ context.Context, name string, values ...string) ([]byte, error) {
		command = name
		args = append([]string(nil), values...)
		return []byte("Pinging with SNI\nHandshake succeeded\nTLS Version: TLS 1.3\nTLS Post-Quantum key exchange: false (X25519)\n"), nil
	})
	result := pinger.Ping(context.Background(), "93.184.216.34", "example.com", 443)
	if !result.HandshakeSucceeded || command != "/usr/local/bin/rw-core" || !reflect.DeepEqual(args, []string{"tls", "ping", "-ip", "93.184.216.34", "example.com:443"}) {
		t.Fatalf("result=%+v command=%s args=%v", result, command, args)
	}

	failed := NewXrayPinger("/usr/local/bin/rw-core", func(context.Context, string, ...string) ([]byte, error) {
		return []byte("command failed\x00"), errors.New("exit status 1")
	}).Ping(context.Background(), "93.184.216.34", "example.com", 443)
	if failed.HandshakeSucceeded || failed.CommandError == "" || strings.ContainsRune(failed.CommandError, '\x00') {
		t.Fatalf("failed result = %+v", failed)
	}
}

func parseFixture(t *testing.T, name string) XrayEvidence {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return ParseXrayTLSPingOutput(string(raw))
}
