package auditlog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadChunkSupportsInitialTailOffsetsAndTimestampFiltering(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "access.log")
	content := strings.Join([]string{
		"2026/07/17 00:00:00 old line",
		"2026/07/18 00:00:00 retained line",
		"unparseable diagnostic line",
		"2026-07-19T01:00:00.000Z latest line",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, filepath.Join(directory, "error.log"))
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	inode := inodeString(stat)

	initial, err := service.ReadChunk(ChunkRequest{
		Source:           SourceAccess,
		InitialTailBytes: 70,
		MaxReadBytes:     200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if initial.FileInode == nil || *initial.FileInode != inode {
		t.Fatalf("inode = %v, want %s", initial.FileInode, inode)
	}
	if initial.ReadStart <= 0 || len(initial.Lines) == 0 || strings.HasPrefix(initial.Lines[0], "2026/") && strings.Contains(initial.Lines[0], "old line") {
		t.Fatalf("initial tail = %+v, want complete tail lines without a partial first line", initial)
	}

	from := time.Date(2026, time.July, 18, 0, 0, 0, 0, time.UTC)
	filtered, err := service.ReadChunk(ChunkRequest{
		Source:        SourceAccess,
		Offset:        int64Ptr(0),
		EndOffset:     int64Ptr(int64(len(content))),
		FromTimestamp: &from,
		MaxReadBytes:  int64(len(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filtered.Lines, []string{
		"2026/07/18 00:00:00 retained line",
		"unparseable diagnostic line",
		"2026-07-19T01:00:00.000Z latest line",
	}) {
		t.Fatalf("filtered lines = %#v", filtered.Lines)
	}
	if filtered.ReadStart != 0 || filtered.NextOffset != int64(len(content)) {
		t.Fatalf("offsets = readStart %d nextOffset %d", filtered.ReadStart, filtered.NextOffset)
	}
}

func TestReadChunkRejectsChangedInodeAndHonorsEndOffset(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "access.log")
	if err := os.WriteFile(path, []byte("2026/07/18 00:00:00 one\n2026/07/18 00:00:01 two\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, filepath.Join(directory, "error.log"))
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReadChunk(ChunkRequest{Source: SourceAccess, ExpectedInode: "changed"})
	if err != ErrInodeChanged {
		t.Fatalf("error = %v, want ErrInodeChanged", err)
	}

	end := int64(len("2026/07/18 00:00:00 one\n"))
	chunk, err := service.ReadChunk(ChunkRequest{
		Source:        SourceAccess,
		Offset:        int64Ptr(0),
		EndOffset:     &end,
		ExpectedInode: inodeString(stat),
		MaxReadBytes:  end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chunk.Lines, []string{"2026/07/18 00:00:00 one"}) || chunk.TotalSize != end || chunk.NextOffset != end {
		t.Fatalf("chunk = %+v", chunk)
	}
}

func TestDropPartialFirstLinePreservesLeadingLineFeedOffset(t *testing.T) {
	t.Parallel()

	text, readStart := dropPartialFirstLine("\n2026/07/18 00:00:00 complete\n", 20)
	if text != "\n2026/07/18 00:00:00 complete\n" || readStart != 20 {
		t.Fatalf("normalized text = %q, readStart = %d", text, readStart)
	}
}

func TestSourceMetadataAndAbsentFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	accessPath := filepath.Join(directory, "access.log")
	if err := os.WriteFile(accessPath, []byte(strings.Join([]string{
		"2026/07/18 00:00:00 first",
		"noise",
		"2026-07-19T01:00:00.000Z last",
	}, "\n")+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := NewService(accessPath, filepath.Join(directory, "error.log"))
	metadata, err := service.SourceMetadata(SourceAccess)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.EarliestTimestamp == nil || metadata.EarliestTimestamp.Format(time.RFC3339Nano) != "2026-07-18T00:00:00Z" {
		t.Fatalf("earliest timestamp = %v", metadata.EarliestTimestamp)
	}
	if metadata.LatestTimestamp == nil || metadata.LatestTimestamp.Format(time.RFC3339Nano) != "2026-07-19T01:00:00Z" {
		t.Fatalf("latest timestamp = %v", metadata.LatestTimestamp)
	}

	absent := NewService(filepath.Join(directory, "missing-access.log"), filepath.Join(directory, "missing-error.log"))
	chunk, err := absent.ReadChunk(ChunkRequest{Source: SourceError})
	if err != nil || chunk.FileInode != nil || chunk.Lines == nil || len(chunk.Lines) != 0 {
		t.Fatalf("absent chunk = %+v, error = %v", chunk, err)
	}
	missingMetadata, err := absent.SourceMetadata(SourceError)
	if err != nil || missingMetadata.Inode != "" || missingMetadata.EarliestTimestamp != nil || missingMetadata.LatestTimestamp != nil {
		t.Fatalf("absent metadata = %+v, error = %v", missingMetadata, err)
	}
}

func TestCleanLogsStreamsRetentionAndPreservesUnparseableLines(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	accessPath := filepath.Join(directory, "access.log")
	errorPath := filepath.Join(directory, "error.log")
	old := "2026/07/17 00:00:00 old"
	newLine := "2026/07/19 00:00:00 new"
	noise := "unparseable must survive"
	if err := os.WriteFile(accessPath, []byte(strings.Join([]string{old, newLine, noise}, "\n")+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errorPath, []byte(strings.Join([]string{old, newLine}, "\n")+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := NewService(accessPath, errorPath)
	cutoff := time.Date(2026, time.July, 18, 0, 0, 0, 0, time.UTC)
	result, err := service.CleanLogsAt(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 || result.RemovedLines != 2 || result.KeptLines != 3 {
		t.Fatalf("cleanup = %+v", result)
	}
	cleaned, err := os.ReadFile(accessPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(cleaned) != newLine+"\n"+noise+"\n" {
		t.Fatalf("cleaned access log = %q", cleaned)
	}
	mode, err := os.Stat(accessPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", mode.Mode().Perm())
	}
}

func TestCleanLogsPreservesLargeUnparseableLine(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	accessPath := filepath.Join(directory, "access.log")
	largeLine := strings.Repeat("x", 1_100_000)
	if err := os.WriteFile(accessPath, []byte(largeLine+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := NewService(accessPath, filepath.Join(directory, "error.log"))
	result, err := service.CleanLogsAt(time.Date(2026, time.July, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.KeptLines != 1 || result.RemovedLines != 0 {
		t.Fatalf("cleanup = %+v", result)
	}
}

func int64Ptr(value int64) *int64 { return &value }
