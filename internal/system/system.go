package system

import (
	"bufio"
	"context"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/tcpstats"
	"github.com/12Jack21/remnaplus-alpine-node/internal/vnstat"
)

var processStartedAt = time.Now()

type NetworkInterface struct {
	Interface     string `json:"interface"`
	RxBytesPerSec int64  `json:"rxBytesPerSec"`
	TxBytesPerSec int64  `json:"txBytesPerSec"`
	RxTotal       int64  `json:"rxTotal"`
	TxTotal       int64  `json:"txTotal"`
}

type Info struct {
	Arch              string   `json:"arch"`
	CPUs              int      `json:"cpus"`
	CPUModel          string   `json:"cpuModel"`
	MemoryTotal       uint64   `json:"memoryTotal"`
	Hostname          string   `json:"hostname"`
	Platform          string   `json:"platform"`
	Release           string   `json:"release"`
	Type              string   `json:"type"`
	Version           string   `json:"version"`
	NetworkInterfaces []string `json:"networkInterfaces"`
	OsName            *string  `json:"osName"`
	Virtualization    *string  `json:"virtualization"`
	SwapTotal         *uint64  `json:"swapTotal,omitempty"`
}

type Stats struct {
	ResourceStats

	MemoryFree       uint64              `json:"memoryFree"`
	MemoryUsed       uint64              `json:"memoryUsed"`
	SwapTotal        *uint64             `json:"swapTotal"`
	SwapFree         *uint64             `json:"swapFree"`
	SwapUsed         *uint64             `json:"swapUsed"`
	ProcessCount     *int                `json:"processCount"`
	UDPConnections   *int                `json:"udpConnections"`
	Uptime           float64             `json:"uptime"`
	LoadAvg          []float64           `json:"loadAvg"`
	Interface        *NetworkInterface   `json:"interface"`
	TCP              tcpstats.Stats      `json:"tcp"`
	VnstatDaily      []vnstat.DailyEntry `json:"vnstatDaily"`
	VnstatError      *vnstat.ErrorCode   `json:"vnstatError"`
	VnstatTotalBytes *float64            `json:"vnstatTotalBytes"`
}

type Snapshot struct {
	Info  Info  `json:"info"`
	Stats Stats `json:"stats"`
}

func GetInfo() Info {
	hostname, _ := os.Hostname()
	memory := readMemoryInfo()
	return Info{
		Arch:              runtime.GOARCH,
		CPUs:              runtime.NumCPU(),
		CPUModel:          cpuModel(),
		MemoryTotal:       memory.total,
		Hostname:          hostname,
		Platform:          runtime.GOOS,
		Release:           kernelRelease(),
		Type:              runtime.GOOS,
		Version:           runtime.Version(),
		NetworkInterfaces: networkInterfaces(),
		OsName:            osReleaseName(),
		Virtualization:    virtualization(),
		SwapTotal:         memory.swapTotal,
	}
}

func GetStats() Stats {
	return GetStatsContext(context.Background())
}

func GetStatsContext(ctx context.Context) Stats {
	memory := readMemoryInfo()
	free, total := memory.free, memory.total
	used := uint64(0)
	if total > free {
		used = total - free
	}
	var swapUsed *uint64
	if memory.swapTotal != nil && memory.swapFree != nil && *memory.swapTotal >= *memory.swapFree {
		value := *memory.swapTotal - *memory.swapFree
		swapUsed = &value
	}
	interfaceStats := defaultMonitor.GetDefaultInterface()
	defaultInterface := resolveDefaultInterface()
	if interfaceStats != nil && interfaceStats.Interface != "" {
		defaultInterface = interfaceStats.Interface
	}
	vnstatResult := vnstat.DefaultSnapshot(ctx, defaultInterface)
	var daily []vnstat.DailyEntry
	var totalBytes *float64
	if vnstatResult.Snapshot != nil {
		daily = vnstatResult.Snapshot.Daily
		value := vnstatResult.Snapshot.TotalBytes
		totalBytes = &value
	}
	return Stats{
		ResourceStats:    ResourcesSnapshot(),
		MemoryFree:       free,
		MemoryUsed:       used,
		SwapTotal:        memory.swapTotal,
		SwapFree:         memory.swapFree,
		SwapUsed:         swapUsed,
		ProcessCount:     processCount(),
		UDPConnections:   udpConnections(),
		Uptime:           uptime(),
		LoadAvg:          loadAvg(),
		Interface:        interfaceStats,
		TCP:              tcpstats.GetStats(),
		VnstatDaily:      daily,
		VnstatError:      vnstatResult.Error,
		VnstatTotalBytes: totalBytes,
	}
}

func GetSnapshot() Snapshot {
	return Snapshot{
		Info:  GetInfo(),
		Stats: GetStats(),
	}
}

func networkInterfaces() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		names = append(names, iface.Name)
	}
	return names
}

type memoryInfo struct {
	free      uint64
	total     uint64
	swapFree  *uint64
	swapTotal *uint64
}

func readMemoryInfo() memoryInfo {
	info := memoryInfo{}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		info.total = mem.Sys
		return info
	}
	defer file.Close()

	var free, available, total, swapFree, swapTotal uint64
	var hasSwap bool
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		value *= 1024
		switch fields[0] {
		case "MemTotal:":
			total = value
		case "MemFree:":
			free = value
		case "MemAvailable:":
			available = value
		case "SwapTotal:":
			swapTotal = value
			hasSwap = true
		case "SwapFree:":
			swapFree = value
		}
	}
	if available > 0 {
		free = available
	}
	info.free = free
	info.total = total
	if hasSwap {
		info.swapTotal = &swapTotal
		info.swapFree = &swapFree
	}
	return info
}

func processCount() *int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err == nil {
			count++
		}
	}
	return &count
}

func udpConnections() *int {
	count := tcpstats.GetUDPConnectionCount()
	return &count
}

func kernelRelease() string {
	if raw, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if value := strings.TrimSpace(string(raw)); value != "" {
			return value
		}
	}
	return runtime.GOOS
}

func osReleaseName() *string {
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(line, "PRETTY_NAME=") {
				continue
			}
			value := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			if value != "" {
				return &value
			}
		}
	}
	return nil
}

var hypervisorDMI = regexp.MustCompile(`(?i)kvm|qemu|vmware|virtualbox|bochs|xen|hyper-v|virtual machine|droplet|openstack|amazon ec2|bhyve|parallels|google compute`)

func virtualization() *string {
	hypervisorFlag := false
	if raw, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "flags") && strings.Contains(line, "hypervisor") {
				hypervisorFlag = true
				break
			}
		}
	}
	for _, path := range []string{"/sys/class/dmi/id/product_name", "/sys/class/dmi/id/sys_vendor"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(raw))
		if value != "" && hypervisorDMI.MatchString(value) {
			return &value
		}
	}
	value := "none"
	if hypervisorFlag {
		value = "virtual"
	}
	return &value
}

func loadAvg() []float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return []float64{0, 0, 0}
	}
	fields := strings.Fields(string(raw))
	values := []float64{0, 0, 0}
	for i := 0; i < len(values) && i < len(fields); i++ {
		if parsed, err := strconv.ParseFloat(fields[i], 64); err == nil {
			values[i] = parsed
		}
	}
	return values
}

func uptime() float64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Since(processStartedAt).Seconds()
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return time.Since(processStartedAt).Seconds()
	}
	parsed, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Since(processStartedAt).Seconds()
	}
	return parsed
}

func cpuModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}
