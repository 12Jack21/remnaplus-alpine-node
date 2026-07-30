package tcpstats

import (
	"context"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const listeningPortsTimeout = 2 * time.Second

var trailingPortPattern = regexp.MustCompile(`:(\d+)$`)

type ListeningPort struct {
	Networks []string `json:"networks"`
	Port     int      `json:"port"`
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func ParseListeningPorts(output string) []ListeningPort {
	networksByPort := make(map[int]map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		columns := strings.Fields(strings.TrimSpace(line))
		if len(columns) < 5 || (columns[0] != "tcp" && columns[0] != "udp") {
			continue
		}
		matches := trailingPortPattern.FindStringSubmatch(columns[4])
		if len(matches) != 2 {
			continue
		}
		port, err := strconv.Atoi(matches[1])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if networksByPort[port] == nil {
			networksByPort[port] = make(map[string]struct{})
		}
		networksByPort[port][columns[0]] = struct{}{}
	}

	ports := make([]int, 0, len(networksByPort))
	for port := range networksByPort {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	result := make([]ListeningPort, 0, len(ports))
	for _, port := range ports {
		networks := make([]string, 0, len(networksByPort[port]))
		for network := range networksByPort[port] {
			networks = append(networks, network)
		}
		sort.Strings(networks)
		result = append(result, ListeningPort{Networks: networks, Port: port})
	}
	return result
}

func GetListeningPorts(ctx context.Context) []ListeningPort {
	return collectListeningPorts(ctx, runCommand)
}

func collectListeningPorts(ctx context.Context, runner commandRunner) []ListeningPort {
	bounded, cancel := context.WithTimeout(ctx, listeningPortsTimeout)
	defer cancel()
	raw, err := runner(bounded, "ss", "-H", "-lnut")
	if err != nil {
		return nil
	}
	return ParseListeningPorts(string(raw))
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
