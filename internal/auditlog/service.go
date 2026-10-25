package auditlog

import (
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
	SegmentID        string
}

type Chunk struct {
	FileInode       *string  `json:"fileInode"`
	Lines           []string `json:"lines"`
	LineOffsets     []int64  `json:"lineOffsets"`
	LineOffsetsText []string `json:"lineOffsetsText"`
	NextOffset      int64    `json:"nextOffset"`
	NextOffsetText  string   `json:"nextOffsetText"`
	ReadStart       int64    `json:"readStart"`
	ReadStartText   string   `json:"readStartText"`
	Source          Source   `json:"source"`
	TotalSize       int64    `json:"totalSize"`
	TotalSizeText   string   `json:"totalSizeText"`
}

type SourceMetadata struct {
	EarliestTimestamp *time.Time `json:"earliestTimestamp"`
	Inode             string     `json:"inode"`
	LatestTimestamp   *time.Time `json:"latestTimestamp"`
	Source            Source     `json:"source"`
	TotalSize         int64      `json:"totalSize"`
}

type Segment struct {
	ID     string `json:"id"`
	Inode  string `json:"inode"`
	Size   int64  `json:"size"`
	Sealed bool   `json:"sealed"`
	Source Source `json:"source"`
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
	if request.SegmentID != "" {
		path, err = s.pathForSegment(request.Source, request.SegmentID)
		if err != nil {
			return Chunk{}, err
		}
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
		FileInode:       &inode,
		Lines:           []string{},
		LineOffsets:     []int64{},
		LineOffsetsText: []string{},
		NextOffset:      snapshotEnd,
		NextOffsetText:  strconv.FormatInt(snapshotEnd, 10),
		ReadStart:       readStart,
		ReadStartText:   strconv.FormatInt(readStart, 10),
		Source:          request.Source,
		TotalSize:       snapshotEnd,
		TotalSizeText:   strconv.FormatInt(snapshotEnd, 10),
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
	lines, lineOffsets, nextOffset := splitCompleteLinesWithOffsets(text, readStart)
	if request.FromTimestamp != nil {
		filtered := make([]string, 0, len(lines))
		filteredOffsets := make([]int64, 0, len(lines))
		for index, line := range lines {
			timestamp := parseTimestamp(line)
			if timestamp == nil || !timestamp.Before(*request.FromTimestamp) {
				filtered = append(filtered, line)
				filteredOffsets = append(filteredOffsets, lineOffsets[index])
			}
		}
		lines = filtered
		lineOffsets = filteredOffsets
	}
	result.Lines = lines
	result.LineOffsets = lineOffsets
	result.LineOffsetsText = make([]string, len(lineOffsets))
	for index, offset := range lineOffsets {
		result.LineOffsetsText[index] = strconv.FormatInt(offset, 10)
	}
	result.ReadStart = readStart
	result.ReadStartText = strconv.FormatInt(readStart, 10)
	result.NextOffset = nextOffset
	result.NextOffsetText = strconv.FormatInt(nextOffset, 10)
	result.TotalSize = snapshotEnd
	result.TotalSizeText = strconv.FormatInt(snapshotEnd, 10)
	return result, nil
}

func (s *Service) pathForSegment(source Source, id string) (string, error) {
	active, err := s.pathFor(source)
	if err != nil {
		return "", err
	}
	if id == filepath.Base(active) {
		return active, nil
	}
	base := filepath.Base(active)
	if filepath.Base(id) != id || !strings.HasPrefix(id, base+".segment-") || !strings.HasSuffix(id, ".sealed") {
		return "", errors.New("invalid audit log segment id")
	}
	path := filepath.Join(filepath.Dir(active), id)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("audit log segment is not a regular file")
	}
	return path, nil
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

func (s *Service) Catalog(source Source) ([]Segment, error) {
	path, err := s.pathFor(source)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(path + ".segment-*.sealed")
	if err != nil {
		return nil, err
	}
	paths = append(paths, path)
	segments := make([]Segment, 0, len(paths))
	for _, candidate := range paths {
		info, statErr := os.Stat(candidate)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return nil, statErr
		}
		segments = append(segments, Segment{
			ID: filepath.Base(candidate), Inode: inodeString(info), Size: info.Size(),
			Sealed: candidate != path, Source: source,
		})
	}
	return segments, nil
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
		item, err := cleanSealedSegments(path, cutoff)
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

// cleanSealedSegments deliberately leaves the active destination untouched.
// Xray keeps that file open, and replacing or truncating it would lose bytes
// after the writer has opened the old inode. Legacy active files are therefore
// retained until a managed writer seals them into a removable segment.
func cleanSealedSegments(activePath string, cutoff time.Time) (CleanResult, error) {
	entries, err := os.ReadDir(filepath.Dir(activePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CleanResult{}, nil
		}
		return CleanResult{}, err
	}
	prefix := filepath.Base(activePath) + ".segment-"
	result := CleanResult{}
	removed := false
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".sealed") {
			continue
		}
		match := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".sealed")
		if len(match) < len("20060102-") || match[8] != '-' {
			continue
		}
		day, parseErr := time.Parse("20060102", match[:8])
		if parseErr != nil || !day.Before(cutoff) {
			continue
		}
		segmentPath := filepath.Join(filepath.Dir(activePath), name)
		info, statErr := os.Stat(segmentPath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return CleanResult{}, statErr
		}
		if err := os.Remove(segmentPath); err != nil {
			return CleanResult{}, err
		}
		result.Files++
		result.ReclaimedBytes += int(info.Size())
		removed = true
	}
	if removed {
		if err := syncDirectory(filepath.Dir(activePath)); err != nil {
			return CleanResult{}, err
		}
	}
	return result, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
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
	lines, _, next := splitCompleteLinesWithOffsets(text, readStart)
	return lines, next
}

func splitCompleteLinesWithOffsets(text string, readStart int64) ([]string, []int64, int64) {
	lastNewline := strings.LastIndexByte(text, '\n')
	if lastNewline < 0 {
		return []string{}, []int64{}, readStart
	}
	complete := text[:lastNewline+1]
	parts := strings.Split(complete, "\n")
	lines := make([]string, 0, len(parts))
	offsets := make([]int64, 0, len(parts))
	position := readStart
	for _, raw := range parts {
		line := strings.TrimSuffix(raw, "\r")
		if line != "" {
			lines = append(lines, line)
			offsets = append(offsets, position)
		}
		position += int64(len(raw) + 1)
	}
	return lines, offsets, readStart + int64(len(complete))
}

func readWindow(file *os.File, offset, length int64) ([]byte, error) {
	buffer := make([]byte, length)
	n, err := file.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buffer[:n], nil
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
