package xray

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const disabledWarpEndpoint = "warp-is-disabled-on-this-remnanode.invalid:9"

func guardWarpOutbounds(outbounds []any) ([]any, bool, error) {
	guarded := make([]any, len(outbounds))
	copy(guarded, outbounds)
	applied := false
	warpCount := 0
	for index, value := range outbounds {
		outbound, ok := value.(map[string]any)
		if !ok || outbound["tag"] != "WARP" {
			continue
		}
		warpCount++
		if warpCount > 1 || outbound["protocol"] != "wireguard" {
			return nil, false, fmt.Errorf("conflicting WARP outbound")
		}
		settings, ok := outbound["settings"].(map[string]any)
		if !ok {
			continue
		}
		peers, ok := settings["peers"].([]any)
		if !ok || len(peers) != 1 {
			return nil, false, fmt.Errorf("managed WARP requires exactly one peer")
		}
		peer, ok := peers[0].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("managed WARP requires exactly one peer")
		}
		endpoint, _ := peer["endpoint"].(string)
		if endpoint != disabledWarpEndpoint && !validIPv4Endpoint(endpoint) && !validHostnameEndpoint(endpoint) {
			return nil, false, fmt.Errorf("managed WARP requires an IPv4 or hostname peer endpoint")
		}
		addresses, _ := settings["address"].([]any)
		ipv4Address := ""
		for _, value := range addresses {
			address, _ := value.(string)
			if host := strings.SplitN(address, "/", 2)[0]; net.ParseIP(host) != nil && net.ParseIP(host).To4() != nil {
				ipv4Address = address
				break
			}
		}
		if ipv4Address == "" {
			return nil, false, fmt.Errorf("managed WARP requires an IPv4 tunnel address")
		}
		newSettings := cloneMap(settings)
		newSettings["address"] = []any{ipv4Address}
		newSettings["domainStrategy"] = "ForceIPv4"
		newPeer := cloneMap(peer)
		newPeer["allowedIPs"] = []any{"0.0.0.0/0"}
		newSettings["peers"] = []any{newPeer}
		if endpoint != disabledWarpEndpoint {
			if _, exists := settings["reserved"]; !exists {
				if clientID := strings.TrimSpace(os.Getenv("WARP_CLIENT_ID")); clientID != "" {
					decoded, err := base64.StdEncoding.DecodeString(clientID)
					if err != nil || len(decoded) != 3 || len(clientID) != 4 {
						return nil, false, fmt.Errorf("WARP_CLIENT_ID must encode exactly three bytes as base64")
					}
					reserved := make([]any, len(decoded))
					for i, value := range decoded {
						reserved[i] = int(value)
					}
					newSettings["reserved"] = reserved
				}
			}
		}
		newOutbound := cloneMap(outbound)
		newOutbound["settings"] = newSettings
		newOutbound["targetStrategy"] = "ForceIPv4"
		if streamSettings, ok := outbound["streamSettings"].(map[string]any); ok {
			if sockopt, ok := streamSettings["sockopt"].(map[string]any); ok {
				newStream := cloneMap(streamSettings)
				newSockopt := cloneMap(sockopt)
				newSockopt["domainStrategy"] = "ForceIPv4"
				newStream["sockopt"] = newSockopt
				newOutbound["streamSettings"] = newStream
			}
		}
		guarded[index] = newOutbound
		applied = true
	}
	return guarded, applied, nil
}

func validIPv4Endpoint(endpoint string) bool {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil || net.ParseIP(host).To4() == nil {
		return false
	}
	return validPort(port)
}

func validHostnameEndpoint(endpoint string) bool {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" || net.ParseIP(host) != nil || len(host) > 253 || !validPort(port) {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`).MatchString(label) {
			return false
		}
	}
	return true
}

func validPort(value string) bool {
	port, err := strconv.Atoi(value)
	return err == nil && port >= 1 && port <= 65535
}
