package system

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type DiskStats struct {
	TotalBytes     string  `json:"totalBytes"`
	UsedBytes      string  `json:"usedBytes"`
	AvailableBytes string  `json:"availableBytes"`
	UsedPercent    float64 `json:"usedPercent"`
	SampledAt      string  `json:"sampledAt"`
}

type ResourceStats struct {
	CPUUsagePercent         *float64   `json:"cpuUsagePercent"`
	CPUSampledAt            *string    `json:"cpuSampledAt"`
	Disk                    *DiskStats `json:"disk"`
	ResourceTelemetryErrors []string   `json:"resourceTelemetryErrors"`
}

type cpuCounters struct{ total, idle uint64 }

func parseCPUCounters(raw string) (cpuCounters, error) {
	fields := strings.Fields(strings.SplitN(raw, "\n", 2)[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuCounters{}, fmt.Errorf("invalid CPU counters")
	}
	var counters cpuCounters
	for i, field := range fields[1:] {
		if i >= 8 {
			break
		} // guest and guest_nice are included in user/nice.
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil || ^uint64(0)-counters.total < value {
			return cpuCounters{}, fmt.Errorf("invalid CPU counter")
		}
		counters.total += value
		if i == 3 || i == 4 {
			counters.idle += value
		}
	}
	return counters, nil
}

func cpuPercent(previous, current cpuCounters) *float64 {
	if current.total <= previous.total || current.idle < previous.idle {
		return nil
	}
	total, idle := current.total-previous.total, current.idle-previous.idle
	if idle > total {
		return nil
	}
	percent := float64(total-idle) / float64(total) * 100
	return &percent
}

func diskStats(blocks, free, available, blockSize uint64, sampledAt string) (*DiskStats, error) {
	if blocks == 0 || blockSize == 0 || free > blocks || available > free {
		return nil, fmt.Errorf("invalid filesystem statistics")
	}
	bytes := func(value uint64) string {
		return new(big.Int).Mul(new(big.Int).SetUint64(value), new(big.Int).SetUint64(blockSize)).String()
	}
	return &DiskStats{
		TotalBytes: bytes(blocks), UsedBytes: bytes(blocks - free), AvailableBytes: bytes(available),
		UsedPercent: float64(blocks-free) / float64(blocks) * 100, SampledAt: sampledAt,
	}, nil
}

type resourceCollector struct {
	mu         sync.RWMutex
	previous   *cpuCounters
	nextDiskAt time.Time
	snapshot   ResourceStats
}

var resources = resourceCollector{snapshot: ResourceStats{ResourceTelemetryErrors: []string{"cpu-warming-up", "disk-unavailable"}}}
var resourcesOnce sync.Once

// StartResources starts one sampler for the server lifecycle; requests only read its cache.
func StartResources(ctx context.Context) {
	resourcesOnce.Do(func() {
		resources.sample(time.Now())
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					resources.sample(now)
				}
			}
		}()
	})
}

func ResourcesSnapshot() ResourceStats {
	resources.mu.RLock()
	defer resources.mu.RUnlock()
	value := resources.snapshot
	value.ResourceTelemetryErrors = append([]string{}, value.ResourceTelemetryErrors...)
	return value
}

func (collector *resourceCollector) sample(now time.Time) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	sampledAt := now.UTC().Format(time.RFC3339Nano)
	errors := []string{}
	raw, err := os.ReadFile("/proc/stat")
	current, parseErr := parseCPUCounters(string(raw))
	collector.snapshot.CPUUsagePercent = nil
	collector.snapshot.CPUSampledAt = nil
	if err != nil || parseErr != nil {
		collector.previous = nil
		errors = append(errors, "cpu-unavailable")
	} else {
		collector.snapshot.CPUSampledAt = &sampledAt
		if collector.previous != nil {
			collector.snapshot.CPUUsagePercent = cpuPercent(*collector.previous, current)
		}
		collector.previous = &current
		if collector.snapshot.CPUUsagePercent == nil {
			errors = append(errors, "cpu-warming-up")
		}
	}
	if !now.Before(collector.nextDiskAt) {
		// Match the CPU cadence so the dashboard's root filesystem tile does
		// not sit unchanged between 30-second snapshots.
		collector.nextDiskAt = now.Add(5 * time.Second)
		var stat unix.Statfs_t
		collector.snapshot.Disk = nil
		if err := unix.Statfs("/", &stat); err != nil || stat.Bsize <= 0 {
			errors = append(errors, "disk-unavailable")
		} else {
			disk, err := diskStats(stat.Blocks, stat.Bfree, stat.Bavail, uint64(stat.Bsize), sampledAt)
			if err != nil {
				errors = append(errors, "disk-unavailable")
			} else {
				collector.snapshot.Disk = disk
			}
		}
	} else {
		for _, code := range collector.snapshot.ResourceTelemetryErrors {
			if strings.HasPrefix(code, "disk-") {
				errors = append(errors, code)
			}
		}
	}
	collector.snapshot.ResourceTelemetryErrors = errors
}
