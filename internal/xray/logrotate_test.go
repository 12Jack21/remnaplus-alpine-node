package xray

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSegmentedLogWriterBuffersPartialLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray.out.log")
	writer, err := openSegmentedLogWriter(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("partial line visible: info=%v err=%v", info, err)
	}
	if _, err := writer.Write([]byte(" line\nsecond\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first line\nsecond\n" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestSegmentedLogWriterRotatesCompleteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray.err.log")
	writer, err := openSegmentedLogWriter(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("1234\n5678\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var sealed string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".sealed") {
			sealed = filepath.Join(dir, entry.Name())
		}
	}
	if sealed == "" {
		t.Fatal("expected a sealed segment")
	}
	data, err := os.ReadFile(sealed)
	if err != nil || string(data) != "1234\n" {
		t.Fatalf("sealed=%q err=%v", data, err)
	}
	active, err := os.ReadFile(path)
	if err != nil || string(active) != "5678\n" {
		t.Fatalf("active=%q err=%v", active, err)
	}
}

func TestSegmentedLogWriterReopensExistingActiveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray.out.log")
	first, err := openSegmentedLogWriter(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = first.Write([]byte("before\n"))
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := openSegmentedLogWriter(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = second.Write([]byte("after\n"))
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "before\nafter\n" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestSegmentedLogWriterDoesNotReplaceExistingSealedGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray.out.log")
	existing := path + ".segment-" + time.Now().UTC().Format("20060102") + "-000001.sealed"
	if err := os.WriteFile(existing, []byte("preserve\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer, err := openSegmentedLogWriter(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("new\nmore\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "preserve\n" {
		t.Fatalf("existing sealed segment changed to %q err=%v", data, err)
	}
}

func TestSegmentedLogWriterRecoversRotationJournal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray.out.log")
	sealed := path + ".segment-20261025-000001.sealed"
	if err := os.WriteFile(path, []byte("recover me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".rotation.json", []byte(`{"active":"`+path+`","sealed":"`+sealed+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writer, err := openSegmentedLogWriter(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sealed)
	if err != nil || string(data) != "recover me\n" {
		t.Fatalf("sealed=%q err=%v", data, err)
	}
	if _, err := os.Stat(path + ".rotation.json"); !os.IsNotExist(err) {
		t.Fatalf("rotation journal remains: %v", err)
	}
}

func TestSegmentedLogWriterRejectsRotationJournalCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray.err.log")
	sealed := path + ".segment-20261025-000001.sealed"
	if err := os.WriteFile(path, []byte("active\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealed, []byte("sealed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".rotation.json", []byte(`{"active":"`+path+`","sealed":"`+sealed+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openSegmentedLogWriter(path, 1024); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("expected recovery collision, got %v", err)
	}
}

func TestSegmentedLogWriterNamesMidnightSegmentByItsContentsDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray.out.log")
	writer, err := openSegmentedLogWriter(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("previous day\n")); err != nil {
		t.Fatal(err)
	}
	writer.openDate = "20261024"
	if _, err := writer.Write([]byte("current day\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sealed := path + ".segment-20261024-000001.sealed"
	data, err := os.ReadFile(sealed)
	if err != nil || string(data) != "previous day\n" {
		t.Fatalf("sealed=%q err=%v", data, err)
	}
	active, err := os.ReadFile(path)
	if err != nil || string(active) != "current day\n" {
		t.Fatalf("active=%q err=%v", active, err)
	}
}
