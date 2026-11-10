package xray

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/artifact"
)

// fakeCore writes a shell script that answers `version` and `run -test` like Xray.
func fakeCore(t *testing.T, path, version string, rejectConfig bool) {
	t.Helper()
	exitCode := "0"
	if rejectConfig {
		exitCode = "23"
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = version ]; then echo 'Xray " + version + " (Xray, Penetrates Everything.)'; exit 0; fi\n" +
		"if [ \"$1\" = run ] && [ \"$2\" = -test ]; then echo 'config check'; exit " + exitCode + "; fi\n" +
		"exit 1\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func managedLoader(t *testing.T) (*coreLoader, string) {
	t.Helper()
	bin := t.TempDir()
	data := t.TempDir()
	fakeCore(t, filepath.Join(bin, "xray"), "26.6.27", false)
	if err := os.Symlink(filepath.Join(bin, "xray"), filepath.Join(bin, "rw-core")); err != nil {
		t.Fatal(err)
	}
	loader := newCoreLoader(filepath.Join(bin, "rw-core"), data)
	loader.arch = "amd64"
	return loader, bin
}

func zipWithCore(t *testing.T, path, version string) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "xray")
	fakeCore(t, script, version, false)
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: "xray", Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(body); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"geoip.dat", "LICENSE"} {
		other, _ := writer.Create(name)
		_, _ = other.Write([]byte(name))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAvailableCoreVersionsNeedsManagedLinkAndDataDir(t *testing.T) {
	loader, _ := managedLoader(t)
	if got := loader.availableCoreVersions(); strings.Join(got, ",") != "26.3.27,26.10.31" {
		t.Fatalf("available = %v", got)
	}
	if got := newCoreLoader("/usr/local/bin/xray", t.TempDir()).availableCoreVersions(); got != nil {
		t.Fatalf("unmanaged XRAY_BIN must not offer versions, got %v", got)
	}
	if got := newCoreLoader("/usr/local/bin/rw-core").availableCoreVersions(); got != nil {
		t.Fatalf("no data dir must not offer versions, got %v", got)
	}
	unknownArch := newCoreLoader("/usr/local/bin/rw-core", t.TempDir())
	unknownArch.arch = "riscv64"
	if got := unknownArch.availableCoreVersions(); got != nil {
		t.Fatalf("unpinned architecture must not offer versions, got %v", got)
	}
}

func TestEnsureVersionDownloadsPinnedArchiveAndExtractsCore(t *testing.T) {
	loader, _ := managedLoader(t)
	var requested string
	var options artifact.Options
	loader.download = func(_ context.Context, url, path string, opts artifact.Options) (artifact.Result, error) {
		requested, options = url, opts
		zipWithCore(t, path, "26.10.31")
		return artifact.Result{SHA256: opts.ExpectedSHA256}, nil
	}
	if err := loader.ensureVersion(context.Background(), "26.10.31"); err != nil {
		t.Fatal(err)
	}
	if requested != "https://github.com/XTLS/Xray-core/releases/download/v26.10.31/Xray-linux-64.zip" {
		t.Fatalf("url = %s", requested)
	}
	if options.ExpectedSHA256 != coreArchiveSHA256["26.10.31"]["amd64"] {
		t.Fatalf("download must pin the reviewed SHA-256, got %q", options.ExpectedSHA256)
	}
	if !loader.matchesVersion(context.Background(), loader.versionBinary("26.10.31"), "26.10.31") {
		t.Fatal("installed core does not report 26.10.31")
	}
	// Already installed: no second download.
	loader.download = func(context.Context, string, string, artifact.Options) (artifact.Result, error) {
		t.Fatal("unexpected re-download")
		return artifact.Result{}, nil
	}
	if err := loader.ensureVersion(context.Background(), "26.10.31"); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureVersionRejectsArchiveWithWrongCore(t *testing.T) {
	loader, _ := managedLoader(t)
	loader.download = func(_ context.Context, _ string, path string, _ artifact.Options) (artifact.Result, error) {
		zipWithCore(t, path, "26.6.27")
		return artifact.Result{}, nil
	}
	err := loader.ensureVersion(context.Background(), "26.3.27")
	if err == nil || !strings.Contains(err.Error(), "does not report version 26.3.27") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(loader.versionBinary("26.3.27")); !os.IsNotExist(statErr) {
		t.Fatal("a mismatched core must not be installed")
	}
}

func TestSelectedVersionSurvivesStartWithoutCustomCore(t *testing.T) {
	loader, bin := managedLoader(t)
	fakeCore(t, loader.versionBinary("26.3.27"), "26.3.27", false)
	if err := loader.selectVersion("26.3.27"); err != nil {
		t.Fatal(err)
	}
	// A normal xray/start without geodata.core used to restore the stock core.
	if err := loader.prepare(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := loader.activeTarget(); got != loader.versionBinary("26.3.27") {
		t.Fatalf("rw-core -> %s, want selected core", got)
	}
	// Without a valid selection, stock is restored.
	if err := os.Remove(loader.selectionPath()); err != nil {
		t.Fatal(err)
	}
	if err := loader.prepare(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := loader.activeTarget(); got != filepath.Join(bin, "xray") {
		t.Fatalf("rw-core -> %s, want stock core", got)
	}
}

func TestRestoreLinkPutsBackPreviousTargetAndSelection(t *testing.T) {
	loader, bin := managedLoader(t)
	stock := filepath.Join(bin, "xray")
	fakeCore(t, loader.versionBinary("26.10.31"), "26.10.31", false)
	if err := loader.selectVersion("26.10.31"); err != nil {
		t.Fatal(err)
	}
	if err := loader.restoreLink(stock, "", false); err != nil {
		t.Fatal(err)
	}
	if loader.activeTarget() != stock {
		t.Fatal("previous rw-core target not restored")
	}
	if _, ok := loader.readSelection(); ok {
		t.Fatal("selection must be cleared when there was none before")
	}
}

func switchManager(t *testing.T) (*Manager, *coreLoader) {
	t.Helper()
	loader, _ := managedLoader(t)
	manager, err := NewManager(Options{XrayBin: loader.paths.active, GeoDir: t.TempDir(), LogDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manager.coreLoader = loader
	return manager, loader
}

func TestSwitchRejectsUnsupportedAndCustomCoreRequests(t *testing.T) {
	manager, _ := switchManager(t)
	request := StartRequest{XrayConfig: map[string]any{"inbounds": []any{}}}
	if _, err := manager.SwitchCoreVersion(context.Background(), "26.6.27", request); err == nil {
		t.Fatal("unsupported version must be rejected")
	}
	custom := StartRequest{XrayConfig: map[string]any{"geodata": map[string]any{"core": map[string]any{
		"url": "https://example.com/xray", "sha256": strings.Repeat("a", 64),
	}}}}
	if _, err := manager.SwitchCoreVersion(context.Background(), "26.3.27", custom); err == nil ||
		!strings.Contains(err.Error(), "custom core") {
		t.Fatalf("custom core config must be rejected, got %v", err)
	}
	unmanaged, err := NewManager(Options{XrayBin: "/usr/local/bin/xray", GeoDir: t.TempDir(), LogDir: t.TempDir(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unmanaged.SwitchCoreVersion(context.Background(), "26.3.27", request); err != errCoreSwitchUnsupported {
		t.Fatalf("unmanaged node err = %v", err)
	}
}

func TestSwitchKeepsCurrentCoreWhenCandidateRejectsConfig(t *testing.T) {
	manager, loader := switchManager(t)
	before := loader.activeTarget()
	fakeCore(t, loader.versionBinary("26.3.27"), "26.3.27", true)
	_, err := manager.SwitchCoreVersion(context.Background(), "26.3.27", StartRequest{XrayConfig: map[string]any{"inbounds": []any{}}})
	if err == nil || !strings.Contains(err.Error(), "rejected the configuration") {
		t.Fatalf("err = %v", err)
	}
	if loader.activeTarget() != before {
		t.Fatal("a rejected candidate must not change rw-core")
	}
	if _, ok := loader.readSelection(); ok {
		t.Fatal("a rejected candidate must not be selected")
	}
}

func TestSwitchRestoresPreviousCoreWhenCandidateDoesNotStart(t *testing.T) {
	manager, loader := switchManager(t)
	manager.lowMemory = false
	before := loader.activeTarget()
	fakeCore(t, loader.versionBinary("26.3.27"), "26.3.27", false)
	deadline := time.Now()
	_, err := manager.SwitchCoreVersion(context.Background(), "26.3.27", StartRequest{XrayConfig: map[string]any{"inbounds": []any{}}})
	if err == nil || !strings.Contains(err.Error(), "previous core was restored") {
		t.Fatalf("err = %v (after %s)", err, time.Since(deadline))
	}
	if loader.activeTarget() != before {
		t.Fatalf("rw-core -> %s, want previous %s", loader.activeTarget(), before)
	}
	if _, ok := loader.readSelection(); ok {
		t.Fatal("failed switch must not leave a selection behind")
	}
	raw, _ := json.Marshal(manager.CoreVersions())
	if !strings.Contains(string(raw), `"available":["26.3.27","26.10.31"]`) {
		t.Fatalf("core versions = %s", raw)
	}
}
