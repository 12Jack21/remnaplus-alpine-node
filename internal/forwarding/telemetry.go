package forwarding

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultConfigPath = "/etc/middle-forward/mappings.tsv"
	DefaultSocketPath = "/run/middle-forward/haproxy.sock"
	maxResponseBytes  = 4 * 1024 * 1024
	maxRequested      = 100
	socketTimeout     = 3 * time.Second
)

type ForwardedTCPConnection struct {
	ConnectionCount int    `json:"connectionCount"`
	MappingName     string `json:"mappingName"`
	SourceIP        string `json:"sourceIp"`
}

type ConfiguredProxyMapping struct {
	Enabled      bool   `json:"enabled"`
	ListenPort   int    `json:"listenPort"`
	MappingName  string `json:"mappingName"`
	Protocol     string `json:"protocol"`
	SourceIPMode string `json:"sourceIpMode"`
	TableName    string `json:"tableName"`
	TargetIP     string `json:"targetIp"`
	TargetPort   int    `json:"targetPort"`
}

type Diagnostic struct {
	Code        string `json:"code"`
	MappingName string `json:"mappingName"`
	Message     string `json:"message"`
}

type Result struct {
	Connections []ForwardedTCPConnection `json:"connections"`
	Diagnostics []Diagnostic             `json:"diagnostics"`
	Mappings    []ConfiguredProxyMapping `json:"mappings"`
}

var mappingNamePattern = regexp.MustCompile(`^[A-Za-z0-9$][A-Za-z0-9_$-]{0,31}$`)

func ParseConfiguredProxyMappings(content string) map[string]ConfiguredProxyMapping {
	mappings := make(map[string]ConfiguredProxyMapping)
	scanner := bufio.NewScanner(strings.NewReader(content))
	first := true
	for scanner.Scan() {
		row := scanner.Text()
		if first {
			first = false
			continue
		}
		if strings.TrimSpace(row) == "" {
			continue
		}
		fields := strings.Split(row, "\t")
		if len(fields) < 8 {
			continue
		}
		listenPort, listenErr := strconv.Atoi(fields[4])
		targetPort, targetErr := strconv.Atoi(fields[6])
		if fields[7] != "1" || fields[2] != "tcp" || fields[3] != "proxy_protocol" ||
			!mappingNamePattern.MatchString(fields[1]) || net.ParseIP(fields[5]) == nil ||
			listenErr != nil || listenPort < 1 || listenPort > 65535 ||
			targetErr != nil || targetPort < 1 || targetPort > 65535 {
			continue
		}
		mappings[fields[1]] = ConfiguredProxyMapping{
			Enabled:      true,
			ListenPort:   listenPort,
			MappingName:  fields[1],
			Protocol:     "tcp",
			SourceIPMode: "proxy_protocol",
			TableName:    "mf_" + strings.NewReplacer("-", "_", "$", "_").Replace(fields[1]),
			TargetIP:     fields[5],
			TargetPort:   targetPort,
		}
	}
	return mappings
}

func ParseHAProxyStickTable(mappingName, content string) ([]ForwardedTCPConnection, error) {
	connections := make([]ForwardedTCPConnection, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "key=") {
			continue
		}
		fields := strings.Fields(line)
		var sourceIP string
		var countText string
		for _, field := range fields {
			if strings.HasPrefix(field, "key=") {
				sourceIP = strings.TrimPrefix(field, "key=")
			}
			if strings.HasPrefix(field, "conn_cur=") {
				countText = strings.TrimPrefix(field, "conn_cur=")
			}
		}
		if sourceIP == "" || countText == "" {
			return nil, fmt.Errorf("invalid HAProxy stick-table row for %s", mappingName)
		}
		if net.ParseIP(sourceIP) == nil {
			return nil, fmt.Errorf("invalid source IP in HAProxy table for %s", mappingName)
		}
		if _, duplicate := seen[sourceIP]; duplicate {
			return nil, fmt.Errorf("duplicate source IP in HAProxy table for %s", mappingName)
		}
		count, err := strconv.ParseUint(countText, 10, 31)
		if err != nil {
			return nil, fmt.Errorf("invalid connection count in HAProxy table for %s", mappingName)
		}
		seen[sourceIP] = struct{}{}
		connections = append(connections, ForwardedTCPConnection{
			ConnectionCount: int(count),
			MappingName:     mappingName,
			SourceIP:        sourceIP,
		})
	}
	return connections, nil
}

func Get(requestedMappings []string, configPath, socketPath string) Result {
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	requested := uniqueMappings(requestedMappings)
	content, err := os.ReadFile(configPath)
	if err != nil {
		diagnostics := make([]Diagnostic, 0, len(requested))
		for _, name := range requested {
			diagnostics = append(diagnostics, Diagnostic{
				Code:        "mapping-not-configured",
				MappingName: name,
				Message:     mappingUnavailableMessage(configPath, err),
			})
		}
		return Result{Connections: []ForwardedTCPConnection{}, Diagnostics: diagnostics, Mappings: []ConfiguredProxyMapping{}}
	}
	configured := ParseConfiguredProxyMappings(string(content))
	result := Result{
		Connections: []ForwardedTCPConnection{},
		Diagnostics: []Diagnostic{},
		Mappings:    []ConfiguredProxyMapping{},
	}
	for _, name := range requested {
		mapping, ok := configured[name]
		if !ok {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:        "mapping-not-configured",
				MappingName: name,
				Message:     "The requested mapping is not an enabled TCP PROXY-protocol mapping.",
			})
			continue
		}
		result.Mappings = append(result.Mappings, mapping)
		response, err := querySocket(socketPath, mapping.TableName)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:        "socket-unavailable",
				MappingName: name,
				Message:     err.Error(),
			})
			continue
		}
		if strings.Contains(strings.ToLower(response), "no such table") || strings.Contains(strings.ToLower(response), "unknown table") {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:        "table-unavailable",
				MappingName: name,
				Message:     "The HAProxy stick table is unavailable.",
			})
			continue
		}
		connections, err := ParseHAProxyStickTable(name, response)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:        "invalid-response",
				MappingName: name,
				Message:     err.Error(),
			})
			continue
		}
		result.Connections = append(result.Connections, connections...)
	}
	return result
}

func uniqueMappings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == maxRequested {
			break
		}
	}
	return result
}

func mappingUnavailableMessage(path string, err error) string {
	return fmt.Sprintf("Cannot read %s (%s). Configure forwarding Remnanode with environment MIDDLE_FORWARD_TELEMETRY=1, MIDDLE_FORWARD_ETC_MOUNT=/etc/middle-forward, MIDDLE_FORWARD_RUN_MOUNT=/run/middle-forward and read-only volumes /etc/middle-forward:/etc/middle-forward:ro, /run/middle-forward:/run/middle-forward:ro.", path, errorCode(err))
}

func errorCode(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "ENOENT"
	}
	if errors.Is(err, os.ErrPermission) {
		return "EACCES"
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	return "UNKNOWN"
}

func querySocket(socketPath, tableName string) (string, error) {
	connection, err := net.DialTimeout("unix", socketPath, socketTimeout)
	if err != nil {
		return "", fmt.Errorf("HAProxy socket is unavailable: %w", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(socketTimeout))
	if _, err := io.WriteString(connection, "show table "+tableName+"\n"); err != nil {
		return "", fmt.Errorf("HAProxy socket write failed: %w", err)
	}
	reader := bufio.NewReader(io.LimitReader(connection, maxResponseBytes+1))
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("HAProxy socket read failed: %w", err)
	}
	if len(data) > maxResponseBytes {
		return "", errors.New("HAProxy stick-table response is too large.")
	}
	return string(data), nil
}
