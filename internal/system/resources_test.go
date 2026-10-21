package system

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCPUResourceDeltas(t *testing.T) {
	previous, err := parseCPUCounters("cpu 100 0 100 800 0 0 0 0 500 50\ncpu0 0 0 0 0")
	if err != nil || previous.total != 1000 {
		t.Fatalf("guest counters: %+v %v", previous, err)
	}
	current, _ := parseCPUCounters("cpu 150 0 150 900 0 0 0 0 1000 100")
	value := cpuPercent(previous, current)
	if value == nil || *value != 50 {
		t.Fatalf("CPU percentage: %v", value)
	}
	if cpuPercent(current, previous) != nil || cpuPercent(previous, previous) != nil {
		t.Fatal("reset or empty interval must be unavailable")
	}
	if _, err := parseCPUCounters("cpu nope 0 0 0"); err == nil {
		t.Fatal("malformed counters accepted")
	}
}

func TestDiskResources(t *testing.T) {
	disk, err := diskStats(100, 25, 20, 4096, "now")
	if err != nil || disk.UsedPercent != 75 || disk.TotalBytes != "409600" || disk.UsedBytes != "307200" || disk.AvailableBytes != "81920" {
		t.Fatalf("disk: %+v %v", disk, err)
	}
	if _, err := diskStats(0, 0, 0, 4096, "now"); err == nil {
		t.Fatal("zero filesystem accepted")
	}
	if _, err := diskStats(10, 11, 0, 4096, "now"); err == nil {
		t.Fatal("invalid free blocks accepted")
	}
	if _, err := diskStats(10, 5, 6, 4096, "now"); err == nil {
		t.Fatal("invalid available blocks accepted")
	}
	disk, err = diskStats(1<<60, 0, 0, 4096, "now")
	if err != nil || disk.TotalBytes != "4722366482869645213696" {
		t.Fatalf("large disk precision: %+v %v", disk, err)
	}
}

func TestDiskIoStatsParsing(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"   8       0 sda 1000 0 2000 0 500 0 4000 0 0 0 0",
		"   8       1 sda1 900 0 1800 0 400 0 3600 0 0 0 0",
		"   7       0 loop0 10 0 20 0 0 0 0 0 0 0 0",
		" 253       0 dm-0 400 0 800 0 200 0 1600 0 0 0 0",
	}, "\n")
	counters := parseDiskstats(content, map[string]struct{}{
		"sda": {}, "loop0": {}, "dm-0": {},
	})
	if len(counters) != 1 {
		t.Fatalf("counters = %+v, want only the whole physical disk", counters)
	}
	if counters["sda"].readBytes != 2000*diskSectorBytes || counters["sda"].writeBytes != 4000*diskSectorBytes {
		t.Fatalf("sda counters = %+v", counters["sda"])
	}
}

func TestResourcesConcurrentSnapshots(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			resources.sample(time.Now())
		}
	}()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot := ResourcesSnapshot()
			snapshot.ResourceTelemetryErrors = append(snapshot.ResourceTelemetryErrors, "test")
		}()
	}
	wg.Wait()
	for _, code := range ResourcesSnapshot().ResourceTelemetryErrors {
		if code == "test" {
			t.Fatal("snapshot mutated collector")
		}
	}
}
