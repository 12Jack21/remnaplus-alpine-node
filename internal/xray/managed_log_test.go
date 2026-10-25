package xray

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureManagedLogPathsOnlyRewritesRecognizedDestinations(t *testing.T) {
	dir := t.TempDir()
	config := map[string]any{"log": map[string]any{
		"access": filepath.Join(dir, "access.log"),
		"error":  "/custom/error.log",
	}}
	pumps := configureManagedLogPaths(config, dir)
	if len(pumps) != 1 {
		t.Fatalf("pumps=%d, want 1", len(pumps))
	}
	logConfig := config["log"].(map[string]any)
	if logConfig["access"] != filepath.Join(dir, ".access.log.pipe") {
		t.Fatalf("managed access path=%v", logConfig["access"])
	}
	if logConfig["error"] != "/custom/error.log" {
		t.Fatalf("custom error path was rewritten: %v", logConfig["error"])
	}
}

func TestManagedLogPumpDrainsIntoActiveFile(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, ".access.log.pipe")
	pump := &managedLogPump{pipePath: pipe, stop: make(chan struct{}), done: make(chan struct{})}
	if err := pump.start(filepath.Join(dir, "access.log")); err != nil {
		t.Fatal(err)
	}
	fd, err := os.OpenFile(pipe, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write([]byte("managed line\n")); err != nil {
		t.Fatal(err)
	}
	_ = fd.Close()
	pump.close()
	data, err := os.ReadFile(filepath.Join(dir, "access.log"))
	if err != nil || string(data) != "managed line\n" {
		t.Fatalf("active data=%q err=%v", data, err)
	}
}
