package xray

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

const maxPreStartMatches = 256

func (m *Manager) runPreStart() {
	m.mu.RLock()
	provider := m.torrentBlocker
	m.mu.RUnlock()
	if provider == nil {
		return
	}
	enabled, patterns := provider.PreStartCleanupSockets()
	if !enabled || len(patterns) == 0 {
		return
	}

	removed := 0
	for _, path := range resolveSocketPatterns(patterns) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("warning: pre-start inspect socket %s: %v", path, err)
			continue
		}
		if info.Mode()&os.ModeSocket == 0 {
			log.Printf("pre-start socket cleanup skipped non-socket %s", path)
			continue
		}
		if err := os.Remove(path); err != nil {
			log.Printf("warning: pre-start remove socket %s: %v", path, err)
			continue
		}
		removed++
		log.Printf("pre-start removed stale socket %s", path)
	}
	log.Printf("pre-start socket cleanup completed: %d removed", removed)
}

func resolveSocketPatterns(patterns []string) []string {
	result := make([]string, 0, min(len(patterns), maxPreStartMatches))
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		var matches []string
		if strings.ContainsAny(pattern, "*?[") {
			var err error
			matches, err = filepath.Glob(pattern)
			if err != nil {
				log.Printf("warning: pre-start invalid socket glob %q: %v", pattern, err)
				continue
			}
		} else {
			matches = []string{pattern}
		}
		for _, match := range matches {
			if _, duplicate := seen[match]; duplicate {
				continue
			}
			if len(result) == maxPreStartMatches {
				log.Printf("warning: pre-start socket cleanup exceeded %d unique matches; remaining entries skipped", maxPreStartMatches)
				return result
			}
			seen[match] = struct{}{}
			result = append(result, match)
		}
	}
	return result
}
