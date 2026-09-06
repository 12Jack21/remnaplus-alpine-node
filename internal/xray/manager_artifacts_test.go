package xray

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/12Jack21/remnaplus-alpine-node/internal/artifact"
)

func TestStartPreparesArtifactsBeforeSpawningCore(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManager(Options{XrayBin: "definitely-missing-rw-core", GeoDir: dir, LogDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var requested bool
	manager.geodataLoader.download = func(_ context.Context, url, path string, _ artifact.Options) (artifact.Result, error) {
		requested = true
		if url != "https://example.com/custom.dat" {
			t.Fatalf("unexpected URL: %s", url)
		}
		return artifact.Result{Size: 3}, os.WriteFile(path, []byte("geo"), 0o600)
	}
	response := manager.Start(context.Background(), StartRequest{XrayConfig: map[string]any{
		"geodata": map[string]any{"assets": []any{map[string]any{"url": "https://example.com/custom.dat", "file": "custom.dat"}}},
	}})
	if !requested || response.IsStarted || response.Error == nil || !strings.Contains(*response.Error, "start rw-core") {
		t.Fatalf("artifact/spawn ordering failed: requested=%v response=%+v", requested, response)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "custom.dat")); err != nil || string(got) != "geo" {
		t.Fatalf("geodata=%q err=%v", got, err)
	}
}

func TestStartRejectsWhileLifecycleIsBusy(t *testing.T) {
	manager, err := NewManager(Options{XrayBin: "missing-core", GeoDir: t.TempDir(), LogDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manager.lifecycleMu.Lock()
	response := manager.Start(context.Background(), StartRequest{XrayConfig: map[string]any{}})
	manager.lifecycleMu.Unlock()
	if response.Error == nil || *response.Error != "Request already in progress" {
		t.Fatalf("response=%+v", response)
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.startProcessing {
		t.Fatal("rejected start left startProcessing set")
	}
}

func TestCoreLoaderKeepsRegularFileInstallation(t *testing.T) {
	active := filepath.Join(t.TempDir(), "rw-core")
	if err := os.WriteFile(active, []byte("existing installation"), 0o755); err != nil {
		t.Fatal(err)
	}
	loader := newCoreLoader(active)
	loader.download = func(context.Context, string, string, artifact.Options) (artifact.Result, error) {
		t.Fatal("must not download a replacement for an unmanaged regular file")
		return artifact.Result{}, nil
	}
	if err := loader.prepare(context.Background(), map[string]any{"core": map[string]any{"url": "https://example.com/core", "sha256": strings.Repeat("a", 64)}}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(active); err != nil || string(got) != "existing installation" {
		t.Fatalf("regular installation changed: %q err=%v", got, err)
	}
}

func TestCoreVersionFailurePreservesPreviouslyActiveCore(t *testing.T) {
	dir := t.TempDir()
	stock := filepath.Join(dir, "xray")
	active := filepath.Join(dir, "rw-core")
	if err := os.WriteFile(stock, []byte("stock"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stock, active); err != nil {
		t.Fatal(err)
	}
	loader := newCoreLoader(active)
	loader.download = func(_ context.Context, _, path string, _ artifact.Options) (artifact.Result, error) {
		return artifact.Result{Size: 3}, os.WriteFile(path, []byte("bad"), 0o600)
	}
	loader.readVersion = func(context.Context, string) (string, error) { return "", errors.New("invalid executable") }
	if err := loader.prepare(context.Background(), map[string]any{"core": map[string]any{"url": "https://example.com/core", "sha256": strings.Repeat("a", 64)}}); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(active); err != nil || target != stock {
		t.Fatalf("active core changed: %s err=%v", target, err)
	}
	if _, err := os.Stat(loader.paths.staged); !os.IsNotExist(err) {
		t.Fatalf("staged file remains: %v", err)
	}
}
