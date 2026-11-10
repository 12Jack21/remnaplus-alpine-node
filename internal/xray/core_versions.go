package xray

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/artifact"
)

// SupportedCoreVersions are the Xray cores the dashboard can select. They match
// the Docker Remnanode image (node/src/modules/xray-core/xray-core-versions.ts).
var SupportedCoreVersions = []string{"26.3.27", "26.10.31"}

// coreArchiveSHA256 pins the official XTLS release archives, taken from the
// release .dgst files. Only pinned archives can be installed.
var coreArchiveSHA256 = map[string]map[string]string{
	"26.3.27": {
		"amd64": "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae",
		"arm64": "4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c",
	},
	"26.10.31": {
		"amd64": "e4bd9119f50ae7c41658b1abd0679ae06f98bd864a2907ff33f2f2cf7081b214",
		"arm64": "1811c8f1da0c3b6dc707ebaf1c8390e987ba39859a8f0a7aadd8526157f2ec8d",
	},
}

var coreArchiveName = map[string]string{"amd64": "Xray-linux-64.zip", "arm64": "Xray-linux-arm64-v8a.zip"}

const (
	coreArchiveBaseURL      = "https://github.com/XTLS/Xray-core/releases/download"
	coreArchiveTimeout      = 90 * time.Second
	coreArchiveMaxSize      = 128 << 20
	coreConfigTestTimeout   = 15 * time.Second
	coreSelectionMarkerName = "xray-core-selection.json"
)

var errCoreSwitchUnsupported = errors.New("this node does not manage rw-core; core version switching is unavailable")

type coreSelection struct {
	Version    string `json:"version"`
	SelectedAt string `json:"selectedAt"`
}

// CoreVersionsResponse mirrors the Docker node's /node/xray/core-versions body.
type CoreVersionsResponse struct {
	Available []string `json:"available"`
	Current   *string  `json:"current"`
}

// CoreSwitchResponse mirrors the Docker node's /node/xray/core-version body.
type CoreSwitchResponse struct {
	Current   string   `json:"current"`
	Previous  string   `json:"previous"`
	Available []string `json:"available"`
}

// CoreSwitchRequest is the body of POST /node/xray/core-version.
type CoreSwitchRequest struct {
	Version string        `json:"version"`
	Request *StartRequest `json:"request"`
}

func isSupportedCoreVersion(version string) bool {
	for _, candidate := range SupportedCoreVersions {
		if candidate == version {
			return true
		}
	}
	return false
}

// availableCoreVersions lists the selectable versions on this host. Versions are
// downloaded on demand, so every pinned version for this architecture counts.
func (l *coreLoader) availableCoreVersions() []string {
	if !l.versionsEnabled() {
		return nil
	}
	available := make([]string, 0, len(SupportedCoreVersions))
	for _, version := range SupportedCoreVersions {
		if coreArchiveSHA256[version][l.arch] != "" {
			available = append(available, version)
		}
	}
	return available
}

func (l *coreLoader) versionsEnabled() bool {
	return l != nil && l.managed && l.versionsDir != "" && coreArchiveName[l.arch] != ""
}

func (l *coreLoader) versionBinary(version string) string {
	return filepath.Join(l.versionsDir, version, "xray")
}

func (l *coreLoader) selectionPath() string {
	return filepath.Join(filepath.Dir(l.versionsDir), coreSelectionMarkerName)
}

func (l *coreLoader) readSelection() (string, bool) {
	if !l.versionsEnabled() {
		return "", false
	}
	raw, err := os.ReadFile(l.selectionPath())
	if err != nil {
		return "", false
	}
	var selection coreSelection
	if json.Unmarshal(raw, &selection) != nil || !isSupportedCoreVersion(selection.Version) {
		return "", false
	}
	info, err := os.Stat(l.versionBinary(selection.Version))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	return selection.Version, true
}

// ensureVersion downloads, verifies and unpacks a pinned core if it is missing.
func (l *coreLoader) ensureVersion(ctx context.Context, version string) error {
	target := l.versionBinary(version)
	if l.matchesVersion(ctx, target, version) {
		return nil
	}
	sha := coreArchiveSHA256[version][l.arch]
	if sha == "" {
		return fmt.Errorf("xray %s has no pinned archive for %s", version, l.arch)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create core directory: %w", err)
	}
	archive := filepath.Join(filepath.Dir(target), ".archive.zip")
	defer os.Remove(archive)
	url := fmt.Sprintf("%s/v%s/%s", coreArchiveBaseURL, version, coreArchiveName[l.arch])
	if _, err := l.download(ctx, url, archive, artifact.Options{
		TotalTimeout:   coreArchiveTimeout,
		MaxSize:        coreArchiveMaxSize,
		ExpectedSHA256: sha,
	}); err != nil {
		return fmt.Errorf("download xray %s: %w", version, err)
	}
	staged := target + ".staged"
	defer os.Remove(staged)
	if err := extractCoreBinary(archive, staged); err != nil {
		return fmt.Errorf("unpack xray %s: %w", version, err)
	}
	if !l.matchesVersion(ctx, staged, version) {
		return fmt.Errorf("downloaded core does not report version %s", version)
	}
	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("install xray %s: %w", version, err)
	}
	return nil
}

func (l *coreLoader) matchesVersion(ctx context.Context, path, version string) bool {
	line, err := l.readVersion(ctx, path)
	return err == nil && extractSemver(line) == version
}

func extractCoreBinary(archivePath, target string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "xray" {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		defer source.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(source, coreArchiveMaxSize)); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return errors.New("archive has no xray binary")
}

// testConfig runs `xray run -test` so a core never becomes active with a
// configuration it rejects.
func testCoreConfig(ctx context.Context, binary string, configJSON []byte) error {
	dir, err := os.MkdirTemp("", "remnanode-core-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, configJSON, 0o600); err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, coreConfigTestTimeout)
	defer cancel()
	output, err := exec.CommandContext(testCtx, binary, "run", "-test", "-config", path).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		return fmt.Errorf("%s: %s", err, lines[len(lines)-1])
	}
	return nil
}

// selectVersion points rw-core at a versioned core and remembers the choice so
// later starts keep it.
func (l *coreLoader) selectVersion(version string) error {
	if err := l.ensureActiveCanBeManaged(); err != nil {
		return err
	}
	if err := l.pointLinkAt(l.versionBinary(version)); err != nil {
		return fmt.Errorf("activate xray %s: %w", version, err)
	}
	return writeJSONAtomic(l.selectionPath(), coreSelection{
		Version:    version,
		SelectedAt: l.now().UTC().Format(time.RFC3339Nano),
	}, 0o600)
}

// restoreLink puts back an earlier rw-core target and selection after a failed switch.
func (l *coreLoader) restoreLink(target string, selection string, hadSelection bool) error {
	if target != "" {
		if err := l.pointLinkAt(target); err != nil {
			return err
		}
	}
	if hadSelection {
		return writeJSONAtomic(l.selectionPath(), coreSelection{
			Version:    selection,
			SelectedAt: l.now().UTC().Format(time.RFC3339Nano),
		}, 0o600)
	}
	if err := os.Remove(l.selectionPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *coreLoader) activeTarget() string {
	target, err := os.Readlink(l.paths.active)
	if err != nil {
		return ""
	}
	return resolveLink(l.paths.active, target)
}

func runtimeCoreArch() string {
	return runtime.GOARCH
}
