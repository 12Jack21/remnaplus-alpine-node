package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// maxLogSize triggers rotation per log file; one ".1" backup is kept,
	// so worst-case disk usage is ~2x maxLogSize per file.
	maxLogSize       = 10 << 20
	logCheckInterval = 10 * time.Minute
	// A malformed source line must not hold an unbounded amount of data in
	// memory while waiting for a newline. Flushing this chunk preserves bytes;
	// the reader can still report the resulting record as oversized.
	maxPendingLineBytes = 1 << 20
)

var rotatedLogFiles = []string{"xray.out.log", "xray.err.log"}

// StartLogRotation is retained for lifecycle compatibility. Managed writers
// rotate at complete-line boundaries as bytes arrive, so no timer mutates an
// active Xray destination.
func (m *Manager) StartLogRotation(ctx context.Context) {
	<-ctx.Done()
}

func (m *Manager) rotateLogs() {}

type segmentedLogWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	maxSize  int64
	pending  []byte
	segment  uint64
	openDate string
}

func openSegmentedLogWriter(path string, maxSize int64) (*segmentedLogWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := recoverSegmentRotation(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	segmentPrefix := filepath.Base(path) + ".segment-"
	var generation uint64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, segmentPrefix) || !strings.HasSuffix(name, ".sealed") {
			continue
		}
		generation++
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &segmentedLogWriter{
		file:     file,
		maxSize:  maxSize,
		path:     path,
		size:     info.Size(),
		openDate: time.Now().UTC().Format("20060102"),
		segment:  generation,
	}, nil
}

func (w *segmentedLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index >= maxPendingLineBytes {
			if err := w.writeLine(w.pending[:maxPendingLineBytes]); err != nil {
				return 0, err
			}
			w.pending = w.pending[maxPendingLineBytes:]
			continue
		}
		if index < 0 {
			if len(w.pending) >= maxPendingLineBytes {
				if err := w.writeLine(w.pending); err != nil {
					return 0, err
				}
				w.pending = w.pending[:0]
			}
			return len(p), nil
		}
		line := w.pending[:index+1]
		if err := w.writeLine(line); err != nil {
			return 0, err
		}
		w.pending = w.pending[index+1:]
	}
}

func (w *segmentedLogWriter) writeLine(line []byte) error {
	today := time.Now().UTC().Format("20060102")
	if (w.maxSize > 0 && w.size > 0 && w.size+int64(len(line)) > w.maxSize) || today != w.openDate {
		if err := w.seal(today); err != nil {
			return err
		}
	}
	n, err := w.file.Write(line)
	w.size += int64(n)
	return err
}

func (w *segmentedLogWriter) seal(today string) error {
	if err := w.file.Sync(); err != nil {
		return err
	}
	sealed := ""
	for {
		w.segment++
		candidate := fmt.Sprintf("%s.segment-%s-%06d.sealed", w.path, w.openDate, w.segment)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			sealed = candidate
			break
		} else if err != nil {
			return err
		}
	}
	journalPath := w.path + ".rotation.json"
	journalBytes, err := json.Marshal(struct {
		Active string `json:"active"`
		Sealed string `json:"sealed"`
	}{Active: w.path, Sealed: sealed})
	if err != nil {
		return err
	}
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = journal.Write(journalBytes); err == nil {
		err = journal.Sync()
	}
	closeErr := journal.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := syncDirectory(filepath.Dir(w.path)); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	if err := os.Rename(w.path, sealed); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(w.path)); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(w.path)); err != nil {
		return err
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.file = file
	w.size = 0
	w.openDate = today
	return nil
}

func (w *segmentedLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		if _, err := w.file.Write(w.pending); err != nil {
			return err
		}
		w.pending = nil
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	return w.file.Close()
}

func recoverSegmentRotation(active string) error {
	journalPath := active + ".rotation.json"
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal struct {
		Active string `json:"active"`
		Sealed string `json:"sealed"`
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("invalid rotation journal %s: %w", journalPath, err)
	}
	sealedPrefix := filepath.Base(active) + ".segment-"
	if journal.Active != active || filepath.Dir(journal.Sealed) != filepath.Dir(active) ||
		!strings.HasPrefix(filepath.Base(journal.Sealed), sealedPrefix) ||
		!strings.HasSuffix(journal.Sealed, ".sealed") {
		return fmt.Errorf("invalid rotation journal paths in %s", journalPath)
	}
	_, activeErr := os.Stat(active)
	_, sealedErr := os.Stat(journal.Sealed)
	activeExists := activeErr == nil
	sealedExists := sealedErr == nil
	if activeErr != nil && !errors.Is(activeErr, os.ErrNotExist) {
		return activeErr
	}
	if sealedErr != nil && !errors.Is(sealedErr, os.ErrNotExist) {
		return sealedErr
	}
	if activeExists && sealedExists {
		return fmt.Errorf("rotation recovery collision: both %s and %s exist", active, journal.Sealed)
	}
	if activeExists {
		if err := os.Rename(active, journal.Sealed); err != nil {
			return err
		}
	} else if !sealedExists {
		return fmt.Errorf("rotation recovery lost both active and sealed log: %s", active)
	}
	if err := syncDirectory(filepath.Dir(active)); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(active))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
