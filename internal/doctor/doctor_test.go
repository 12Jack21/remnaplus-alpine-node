package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/12Jack21/remnaplus-alpine-node/internal/config"
)

func TestRunMissingEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.env")
	if code := Run([]string{"--env", missing}); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCheckSecret(t *testing.T) {
	t.Parallel()
	if r := checkSecret(config.Config{SecretKey: "x"}); r[0].level != "OK" {
		t.Fatalf("expected OK, got %#v", r)
	}
	if r := checkSecret(config.Config{}); r[0].level != "ERROR" {
		t.Fatalf("expected ERROR, got %#v", r)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	t.Parallel()
	envPath := filepath.Join(t.TempDir(), "node.env")
	if err := os.WriteFile(envPath, []byte("SECRET_KEY=abc\nNODE_PORT=2222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretKey != "abc" || cfg.NodePort != 2222 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestCheckServiceManagerSystemd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	systemdPath := filepath.Join(dir, "remnawave-node.service")
	if err := os.WriteFile(systemdPath, []byte("[Service]\nExecStart=/usr/local/bin/remnanode-lite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := checkServiceManager(filepath.Join(dir, "missing-openrc"), systemdPath)
	if result.level != "OK" || result.title != "systemd service" {
		t.Fatalf("unexpected systemd result: %#v", result)
	}
}

func TestCheckServiceManagerOpenRC(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	openRCPath := filepath.Join(dir, "remnawave-node")
	if err := os.WriteFile(openRCPath, []byte("#!/sbin/openrc-run\ncommand=/usr/local/bin/remnawave-node-run\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := checkServiceManager(openRCPath, filepath.Join(dir, "missing-systemd"))
	if result.level != "OK" || result.title != "OpenRC service" {
		t.Fatalf("unexpected OpenRC result: %#v", result)
	}
}
