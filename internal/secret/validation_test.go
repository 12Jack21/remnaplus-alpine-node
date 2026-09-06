package secret

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidationRejectsUntrustedOrAmbiguousMaterial(t *testing.T) {
	valid := validPayload(t)
	unrelated := validPayload(t)
	for _, test := range []struct {
		name   string
		mutate func(*Payload)
		want   string
	}{
		{"different-ca", func(p *Payload) { p.CACertPEM = unrelated.CACertPEM }, "not signed by caCertPem"},
		{"trailing-ca", func(p *Payload) { p.CACertPEM += unrelated.CACertPEM }, "trailing PEM data"},
		{"trailing-node-key", func(p *Payload) { p.NodeKeyPEM += unrelated.NodeKeyPEM }, "trailing PEM data"},
		{"trailing-jwt", func(p *Payload) { p.JWTPublicKey += unrelated.JWTPublicKey }, "trailing PEM data"},
		{"truncated-node-cert", func(p *Payload) { p.NodeCertPEM = "-----BEGIN CERTIFICATE-----" }, "nodeCertPem"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.mutate(&payload)
			if err := payload.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %s error, got %v", test.want, err)
			}
		})
	}
}

func TestValidationRejectsFutureCertificates(t *testing.T) {
	payload := validPayloadAt(t, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))
	if err := payload.Validate(); err == nil || !strings.Contains(err.Error(), "not valid before") {
		t.Fatalf("future certificate accepted: %v", err)
	}
}

func TestValidationRejectsNonRSAJWTKey(t *testing.T) {
	payload := validPayload(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	payload.JWTPublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if err := payload.Validate(); err == nil || !strings.Contains(err.Error(), "must be RSA") {
		t.Fatalf("non-RSA JWT key accepted: %v", err)
	}
}

func TestParseAcceptsUnpaddedBase64(t *testing.T) {
	encoded := strings.TrimRight(encodePayload(t, validPayload(t)), "=")
	if _, err := Parse(encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(base64.StdEncoding.EncodeToString([]byte("not JSON"))); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestDashboardGeneratedSecret(t *testing.T) {
	path := os.Getenv("RNL_DASHBOARD_SECRET_FIXTURE")
	if path == "" {
		t.Skip("set RNL_DASHBOARD_SECRET_FIXTURE to a disposable dashboard-generated payload")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(string(raw)); err != nil {
		t.Fatalf("dashboard-generated payload rejected: %v", err)
	}
}
