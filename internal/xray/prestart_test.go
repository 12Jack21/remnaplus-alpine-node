package xray

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

type preStartProvider struct {
	enabled bool
	files   []string
}

func (p preStartProvider) TorrentBlockerEnabled() bool             { return false }
func (p preStartProvider) TorrentBlockerIncludeRuleTags() []string { return nil }
func (p preStartProvider) PreStartCleanupSockets() (bool, []string) {
	return p.enabled, p.files
}

func TestPreStartRemovesSocketsButNotFilesOrSymlinks(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "rnl-prestart-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "stale.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	regularPath := filepath.Join(dir, "regular.sock")
	if err := os.WriteFile(regularPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(dir, "link.sock")
	if err := os.Symlink(socketPath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(Options{XrayBin: "missing", GeoDir: t.TempDir(), LogDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetTorrentBlockerProvider(preStartProvider{enabled: true, files: []string{filepath.Join(dir, "*.sock")}})
	response := manager.Start(t.Context(), StartRequest{XrayConfig: map[string]any{}})
	if response.IsStarted {
		t.Fatal("missing test core unexpectedly started")
	}
	if _, err := os.Lstat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("stale socket remains: %v", err)
	}
	if got, err := os.ReadFile(regularPath); err != nil || string(got) != "keep" {
		t.Fatalf("regular file changed: %q, %v", got, err)
	}
	if _, err := os.Lstat(symlinkPath); err != nil {
		t.Fatalf("symlink was removed: %v", err)
	}
}

func TestPreStartDisabledLeavesSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "rnl-prestart-off-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "keep.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(Options{XrayBin: "missing", GeoDir: t.TempDir(), LogDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetTorrentBlockerProvider(preStartProvider{enabled: false, files: []string{path}})
	manager.runPreStart()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("disabled cleanup removed socket: %v", err)
	}
}

func TestResolveSocketPatternsHasGlobalCapAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < maxPreStartMatches+20; index++ {
		path := filepath.Join(dir, fmt.Sprintf("socket-%03d.sock", index))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pattern := filepath.Join(dir, "*.sock")
	matches := resolveSocketPatterns([]string{pattern, pattern})
	if len(matches) != maxPreStartMatches {
		t.Fatalf("matches = %d, want %d", len(matches), maxPreStartMatches)
	}
	seen := make(map[string]bool, len(matches))
	for _, match := range matches {
		if seen[match] {
			t.Fatalf("duplicate match %s", match)
		}
		seen[match] = true
	}
}
