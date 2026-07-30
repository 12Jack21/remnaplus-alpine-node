package auditlog

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultInitialTailBytes = int64(1_000_000)
	defaultMaxReadBytes     = int64(5_000_000)
	metadataWindowBytes     = int64(1_000_000)
)

type Source string

const (
	SourceAccess Source = "access"
	SourceError  Source = "error"
)

var (
	ErrInodeChanged  = errors.New("AUDIT_LOG_INODE_CHANGED")
	ErrInvalidSource = errors.New("invalid audit log source")
)

type ChunkRequest struct {
	EndOffset        *int64
	ExpectedInode    string
	FromTimestamp    *time.Time
	InitialTailBytes int64
	MaxReadBytes     int64
	Offset           *int64
	Source           Source
}

type Chunk struct {
	FileInode  *string  `json:"fileInode"`
	Lines      []string `json:"lines"`
	NextOffset int64    `json:"nextOffset"`
	ReadStart  int64    `json:"readStart"`
	Source     Source   `json:"source"`
	TotalSize  int64    `json:"totalSize"`
}

type SourceMetadata struct {
	EarliestTimestamp *time.Time `json:"earliestTimestamp"`
	Inode             string     `json:"inode"`
	LatestTimestamp   *time.Time `json:"latestTimestamp"`
	Source            Source     `json:"source"`
	TotalSize         int64      `json:"totalSize"`
}

type CleanResult struct {
	Files          int `json:"files"`
	KeptLines      int `json:"keptLines"`
	RemovedLines   int `json:"removedLines"`
	ReclaimedBytes int `json:"reclaimedBytes"`
}

type Service struct {
	accessPath string
	errorPath  string
}

func NewService(accessPath, errorPath string) *Service {
	return &Service{accessPath: accessPath, errorPath: errorPath}
}

func NewServiceForLogDir(logDir string) *Service {
	return NewService(filepath.Join(logDir, "access.log"), filepath.Join(logDir, "error.log"))
}

func (s *Service) ReadChunk(request ChunkRequest) (Chunk, error) {
	path, err := s.pathFor(request.Source)
	if err != nil {
		return Chunk{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Chunk{Lines: []string{}, Source: request.Source}, nil
		}
		return Chunk{}, err
	}
	inode := inodeString(info)
	if request.ExpectedInode != "" && request.ExpectedInode != inode {
		return Chunk{}, ErrInodeChanged
	}
	initialTailBytes := request.InitialTailBytes
	if initialTailBytes <= 0 {
		initialTailBytes = defaultInitialTailBytes
	}
	maxReadBytes := request.MaxReadBytes
	if maxReadBytes <= 0 {
		maxReadBytes = defaultMaxReadBytes
	}
	requestedOffset := int64(0)
	isInitialTail := request.Offset == nil || *request.Offset > info.Size()
	if isInitialTail {
		requestedOffset = info.Size() - initialTailBytes
		if requestedOffset < 0 {
			requestedOffset = 0
		}
	} else {
		requestedOffset = *request.Offset
	}
	snapshotEnd := info.Size()
	if request.EndOffset != nil && *request.EndOffset < snapshotEnd {
		snapshotEnd = *request.EndOffset
	}
	if snapshotEnd < 0 {
		snapshotEnd = 0
	}
	if requestedOffset < 0 {
		requestedOffset = 0
	}
	readStart := requestedOffset
	readEnd := snapshotEnd
	if candidate := readStart + maxReadBytes; candidate < readEnd {
		readEnd = candidate
	}
	result := Chunk{
		FileInode:  &inode,
		Lines:      []string{},
		NextOffset: snapshotEnd,
		ReadStart:  readStart,
		Source:     request.Source,
		TotalSize:  snapshotEnd,
	}
	if readStart >= readEnd {
		return result, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return Chunk{}, err
	}
	defer file.Close()
	buffer := make([]byte, readEnd-readStart)
	n, readErr := file.ReadAt(buffer, readStart)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return Chunk{}, readErr
	}
	text := string(buffer[:n])
	if isInitialTail && readStart > 0 {
		text, readStart = dropPartialFirstLine(text, readStart)
	}
	lines, nextOffset := splitCompleteLines(text, readStart)
	if request.FromTimestamp != nil {
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			timestamp := parseTimestamp(line)
			if timestamp == nil || !timestamp.Before(*request.FromTimestamp) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	result.Lines = lines
	result.ReadStart = readStart
	result.NextOffset = nextOffset
	return result, nil
}

func (s *Service) SourceMetadata(source Source) (SourceMetadata, error) {
	path, err := s.pathFor(source)
	if err != nil {
		return SourceMetadata{}, err
	}
	metadata := SourceMetadata{Source: source}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return metadata, nil
		}
		return SourceMetadata{}, err
	}
	metadata.Inode = inodeString(info)
	metadata.TotalSize = info.Size()
	window := info.Size()
	if window > metadataWindowBytes {
		window = metadataWindowBytes
	}
	if window == 0 {
		return metadata, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return SourceMetadata{}, err
	}
	defer file.Close()
	head, err := readWindow(file, 0, window)
	if err != nil {
		return SourceMetadata{}, err
	}
	tail, err := readWindow(file, info.Size()-window, window)
	if err != nil {
		return SourceMetadata{}, err
	}
	metadata.EarliestTimestamp = firstTimestamp(string(head))
	metadata.LatestTimestamp = lastTimestamp(string(tail))
	return metadata, nil
}

func (s *Service) CleanLogs(retentionDays int) (CleanResult, error) {
	if retentionDays <= 0 {
		return CleanResult{}, fmt.Errorf("retentionDays must be positive")
	}
	return s.CleanLogsAt(time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour))
}

func (s *Service) CleanLogsAt(cutoff time.Time) (CleanResult, error) {
	paths := []string{s.accessPath, s.errorPath}
	result := CleanResult{}
	for _, path := range paths {
		item, err := compactFile(path, cutoff)
		if err != nil {
			return CleanResult{}, err
		}
		result.Files += item.Files
		result.KeptLines += item.KeptLines
		result.RemovedLines += item.RemovedLines
		result.ReclaimedBytes += item.ReclaimedBytes
	}
	return result, nil
}

func (s *Service) pathFor(source Source) (string, error) {
	switch source {
	case SourceAccess:
		return s.accessPath, nil
	case SourceError:
		return s.errorPath, nil
	default:
		return "", ErrInvalidSource
	}
}

func dropPartialFirstLine(text string, readStart int64) (string, int64) {
	if strings.HasPrefix(text, "\n") || strings.HasPrefix(text, "\r\n") {
		return text, readStart
	}
	newline := strings.IndexByte(text, '\n')
	if newline < 0 {
		return "", readStart
	}
	skipped := int64(newline + 1)
	return text[newline+1:], readStart + skipped
}

func splitCompleteLines(text string, readStart int64) ([]string, int64) {
	lastNewline := strings.LastIndexByte(text, '\n')
	if lastNewline < 0 {
		return []string{}, readStart
	}
	complete := text[:lastNewline+1]
	parts := strings.Split(complete, "\n")
	lines := make([]string, 0, len(parts))
	for _, line := range parts {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, readStart + int64(len(complete))
}

func readWindow(file *os.File, offset, length int64) ([]byte, error) {
	buffer := make([]byte, length)
	n, err := file.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buffer[:n], nil
}

func compactFile(path string, cutoff time.Time) (CleanResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CleanResult{}, nil
		}
		return CleanResult{}, err
	}
	input, err := os.Open(path)
	if err != nil {
		return CleanResult{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-clean-")
	if err != nil {
		input.Close()
		return CleanResult{}, err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		input.Close()
		temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return CleanResult{}, err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), int(defaultMaxReadBytes))
	writer := bufio.NewWriter(temporary)
	result := CleanResult{Files: 1}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		timestamp := parseTimestamp(line)
		if timestamp != nil && timestamp.Before(cutoff) {
			result.RemovedLines++
			continue
		}
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return CleanResult{}, err
		}
		result.KeptLines++
	}
	if err := scanner.Err(); err != nil {
		return CleanResult{}, err
	}
	if err := writer.Flush(); err != nil {
		return CleanResult{}, err
	}
	if err := temporary.Sync(); err != nil {
		return CleanResult{}, err
	}
	if err := temporary.Close(); err != nil {
		return CleanResult{}, err
	}
	if err := input.Close(); err != nil {
		return CleanResult{}, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return CleanResult{}, err
	}
	removeTemporary = false
	after, err := os.Stat(path)
	if err != nil {
		return CleanResult{}, err
	}
	result.ReclaimedBytes = int(info.Size() - after.Size())
	return result, nil
}

func parseTimestamp(line string) *time.Time {
	if len(line) >= len("2006/01/02 15:04:05") {
		if parsed, err := time.Parse("2006/01/02 15:04:05", line[:len("2006/01/02 15:04:05")]); err == nil {
			return &parsed
		}
	}
	const isoMillisecondsLayout = "2006-01-02T15:04:05.000Z"
	if len(line) >= len(isoMillisecondsLayout) {
		if parsed, err := time.Parse(isoMillisecondsLayout, line[:len(isoMillisecondsLayout)]); err == nil {
			return &parsed
		}
	}
	if len(line) >= len(time.RFC3339) {
		if parsed, err := time.Parse(time.RFC3339, line[:len(time.RFC3339)]); err == nil {
			return &parsed
		}
	}
	return nil
}

func firstTimestamp(text string) *time.Time {
	for _, line := range strings.Split(text, "\n") {
		if parsed := parseTimestamp(strings.TrimSuffix(line, "\r")); parsed != nil {
			return parsed
		}
	}
	return nil
}

func lastTimestamp(text string) *time.Time {
	var result *time.Time
	for _, line := range strings.Split(text, "\n") {
		if parsed := parseTimestamp(strings.TrimSuffix(line, "\r")); parsed != nil {
			result = parsed
		}
	}
	return result
}

func inodeString(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return strconv.FormatUint(uint64(stat.Ino), 10)
	}
	return ""
}
