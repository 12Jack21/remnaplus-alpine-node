package tcpstats

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

var procNetTCPFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

const (
	tcpStateEstablished = "01"
	tcpStateListen      = "0A"
)

type Stats struct {
	Established int `json:"established"`
	Total       int `json:"total"`
}

type PeerConnection struct {
	Established int    `json:"established"`
	SourceIP    string `json:"sourceIp"`
	Total       int    `json:"total"`
}

type procRow struct {
	localPort string
	remoteIP  string
	state     string
}

func DecodeHexIPAddress(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) == 8 {
		bytes, err := hex.DecodeString(value)
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("%d.%d.%d.%d", bytes[3], bytes[2], bytes[1], bytes[0]), true
	}
	if len(value) != 32 {
		return "", false
	}
	if (strings.HasPrefix(value, strings.Repeat("0", 20)+"FFFF") ||
		strings.HasPrefix(value, strings.Repeat("0", 16)+"FFFF0000")) && len(value) >= 8 {
		return DecodeHexIPAddress(value[len(value)-8:])
	}

	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != net.IPv6len {
		return "", false
	}
	for offset := 0; offset < len(raw); offset += 4 {
		raw[offset], raw[offset+3] = raw[offset+3], raw[offset]
		raw[offset+1], raw[offset+2] = raw[offset+2], raw[offset+1]
	}
	groups := make([]string, 0, 8)
	for offset := 0; offset < len(raw); offset += 2 {
		groups = append(groups, strconv.FormatUint(uint64(raw[offset])<<8|uint64(raw[offset+1]), 16))
	}
	return strings.Join(groups, ":"), true
}

func ParseProcNetTCPStats(content string) Stats {
	var result Stats
	for _, row := range parseProcRows(content) {
		result.Total++
		if row.state == tcpStateEstablished {
			result.Established++
		}
	}
	return result
}

func ParsePeerConnections(contents []string) []PeerConnection {
	rows := make([]procRow, 0)
	for _, content := range contents {
		rows = append(rows, parseProcRows(content)...)
	}
	listeners := make(map[string]struct{})
	for _, row := range rows {
		if row.state == tcpStateListen && row.localPort != "" {
			listeners[strings.ToUpper(row.localPort)] = struct{}{}
		}
	}

	counts := make(map[string]PeerConnection)
	order := make([]string, 0)
	for _, row := range rows {
		if _, ok := listeners[strings.ToUpper(row.localPort)]; !ok {
			continue
		}
		sourceIP, ok := DecodeHexIPAddress(row.remoteIP)
		if !ok || isLoopbackOrUnspecified(sourceIP) {
			continue
		}
		current, exists := counts[sourceIP]
		if !exists {
			current.SourceIP = sourceIP
			order = append(order, sourceIP)
		}
		current.Total++
		if row.state == tcpStateEstablished {
			current.Established++
		}
		counts[sourceIP] = current
	}

	result := make([]PeerConnection, 0, len(counts))
	for _, sourceIP := range order {
		result = append(result, counts[sourceIP])
	}
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].Established > result[right].Established
	})
	return result
}

func ReadPeerConnections(paths []string) []PeerConnection {
	contents := readProcFiles(paths)
	if len(contents) == 0 {
		return []PeerConnection{}
	}
	return ParsePeerConnections(contents)
}

func GetPeerConnections() []PeerConnection {
	return ReadPeerConnections(procNetTCPFiles)
}

func ReadStats(paths []string) Stats {
	var result Stats
	for _, content := range readProcFiles(paths) {
		stats := ParseProcNetTCPStats(content)
		result.Established += stats.Established
		result.Total += stats.Total
	}
	return result
}

func GetStats() Stats {
	return ReadStats(procNetTCPFiles)
}

func parseProcRows(content string) []procRow {
	rows := make([]procRow, 0)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		local := strings.SplitN(fields[1], ":", 2)
		remote := strings.SplitN(fields[2], ":", 2)
		if len(local) != 2 || len(remote) != 2 {
			continue
		}
		if _, err := strconv.ParseUint(local[1], 16, 16); err != nil {
			continue
		}
		rows = append(rows, procRow{
			localPort: strings.ToUpper(local[1]),
			remoteIP:  strings.ToUpper(remote[0]),
			state:     strings.ToUpper(fields[3]),
		})
	}
	return rows
}

func readProcFiles(paths []string) []string {
	contents := make([]string, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err == nil {
			contents = append(contents, string(raw))
		}
	}
	return contents
}

func isLoopbackOrUnspecified(value string) bool {
	ip := net.ParseIP(value)
	return ip == nil || ip.IsLoopback() || ip.IsUnspecified()
}
