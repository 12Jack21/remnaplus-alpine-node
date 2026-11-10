package xray

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// CoreVersions reports the selectable cores and the running core version.
func (m *Manager) CoreVersions() CoreVersionsResponse {
	m.mu.RLock()
	current := m.xrayVersion
	m.mu.RUnlock()
	available := m.coreLoader.availableCoreVersions()
	if available == nil {
		available = []string{}
	}
	return CoreVersionsResponse{Available: available, Current: current}
}

// SwitchCoreVersion makes `version` the active core and restarts Xray with the
// dashboard's current configuration. Both cores must accept that configuration
// first. If the new core does not come up, the previous core and selection are
// restored and Xray is started again on it.
func (m *Manager) SwitchCoreVersion(ctx context.Context, version string, req StartRequest) (CoreSwitchResponse, error) {
	if !m.coreSwitchMu.TryLock() {
		return CoreSwitchResponse{}, errors.New("an Xray version switch is already in progress")
	}
	defer m.coreSwitchMu.Unlock()
	loader := m.coreLoader
	if !loader.versionsEnabled() {
		return CoreSwitchResponse{}, errCoreSwitchUnsupported
	}
	if !isSupportedCoreVersion(version) || coreArchiveSHA256[version][loader.arch] == "" {
		return CoreSwitchResponse{}, fmt.Errorf("xray %s is not available on this node", version)
	}
	if req.XrayConfig == nil {
		return CoreSwitchResponse{}, errors.New("an applied configuration is required")
	}
	if _, present, _ := parseCoreSpec(req.XrayConfig["geodata"]); present {
		return CoreSwitchResponse{}, errors.New("the configuration installs a custom core (geodata.core); remove it before selecting a version")
	}

	m.mu.RLock()
	previousVersion := ""
	if m.xrayVersion != nil {
		previousVersion = *m.xrayVersion
	}
	online := m.xrayOnline
	m.mu.RUnlock()

	if err := loader.ensureVersion(ctx, version); err != nil {
		return CoreSwitchResponse{}, err
	}
	configJSON := marshalConfigJSON(generateAPIConfig(req.XrayConfig, m.xtlsSocket, m.torrentBlockerOptions(), m.warpProbePort))
	if err := testCoreConfig(ctx, loader.versionBinary(version), configJSON); err != nil {
		return CoreSwitchResponse{}, fmt.Errorf("xray %s rejected the configuration; the current core was not changed: %w", version, err)
	}
	previousTarget := loader.activeTarget()
	if online && previousTarget != "" {
		if err := testCoreConfig(ctx, previousTarget, configJSON); err != nil {
			return CoreSwitchResponse{}, fmt.Errorf("the current core rejected the configuration, so a rollback could not be guaranteed: %w", err)
		}
	}
	previousSelection, hadSelection := loader.readSelection()

	if err := loader.selectVersion(version); err != nil {
		return CoreSwitchResponse{}, err
	}
	switchReq := req
	switchReq.Internals.ForceRestart = true
	started := m.Start(ctx, switchReq)
	m.mu.RLock()
	running := ""
	if m.xrayVersion != nil {
		running = *m.xrayVersion
	}
	m.mu.RUnlock()
	if started.IsStarted && running == version {
		log.Printf("xray core switched: %s -> %s", previousVersion, version)
		return CoreSwitchResponse{Current: version, Previous: previousVersion, Available: loader.availableCoreVersions()}, nil
	}

	reason := "selected core did not become healthy"
	if started.Error != nil {
		reason = *started.Error
	} else if started.IsStarted {
		reason = fmt.Sprintf("running core reports %q instead of %s", running, version)
	}
	log.Printf("xray core switch to %s failed (%s); restoring previous core", version, reason)
	if err := loader.restoreLink(previousTarget, previousSelection, hadSelection); err != nil {
		return CoreSwitchResponse{}, fmt.Errorf("switch failed (%s) and the previous core could not be restored: %w; operator recovery is required", reason, err)
	}
	if online {
		rollback := m.Start(ctx, switchReq)
		if !rollback.IsStarted {
			return CoreSwitchResponse{}, fmt.Errorf("switch failed (%s) and the previous core did not restart; operator recovery is required", reason)
		}
	}
	return CoreSwitchResponse{}, fmt.Errorf("xray switch failed: %s; the previous core was restored", reason)
}
