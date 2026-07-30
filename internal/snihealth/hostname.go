package snihealth

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

func NormalizeHostname(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	for _, character := range trimmed {
		if unicode.IsSpace(character) || strings.ContainsRune("/:@*", character) {
			return "", fmt.Errorf("SNI hostname must be a public DNS hostname without a port or path")
		}
	}
	trimmed = strings.TrimSuffix(trimmed, ".")
	normalized, err := idna.Lookup.ToASCII(trimmed)
	if err != nil {
		return "", fmt.Errorf("normalize SNI hostname: %w", err)
	}
	normalized = strings.ToLower(normalized)
	if normalized == "" || len(normalized) > 253 || net.ParseIP(normalized) != nil {
		return "", fmt.Errorf("SNI hostname must be a public DNS hostname without a port or path")
	}
	for _, label := range strings.Split(normalized, ".") {
		if !validHostnameLabel(label) {
			return "", fmt.Errorf("SNI hostname must be a public DNS hostname without a port or path")
		}
	}
	return normalized, nil
}

func validHostnameLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, character := range label {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func ParseDestination(value string) (Destination, bool) {
	dest := strings.TrimSpace(value)
	if dest == "" || strings.ContainsAny(dest, " \t\r\n/") || strings.Contains(dest, "://") {
		return Destination{}, false
	}
	host := dest
	port := 443
	if strings.HasPrefix(dest, "[") {
		closing := strings.IndexByte(dest, ']')
		if closing < 2 {
			return Destination{}, false
		}
		host = dest[1:closing]
		remainder := dest[closing+1:]
		if remainder != "" {
			if !strings.HasPrefix(remainder, ":") || len(remainder) == 1 {
				return Destination{}, false
			}
			parsed, err := strconv.Atoi(remainder[1:])
			if err != nil {
				return Destination{}, false
			}
			port = parsed
		}
	} else if strings.Count(dest, ":") == 1 {
		separator := strings.LastIndexByte(dest, ':')
		host = dest[:separator]
		parsed, err := strconv.Atoi(dest[separator+1:])
		if err != nil {
			return Destination{}, false
		}
		port = parsed
	}
	if host == "" || port < 1 || port > 65535 {
		return Destination{}, false
	}
	return Destination{Host: host, Port: port, Dest: net.JoinHostPort(host, strconv.Itoa(port))}, true
}

func splitServerNames(value any) []string {
	var raw []any
	switch values := value.(type) {
	case []any:
		raw = values
	case []string:
		raw = make([]any, len(values))
		for index, item := range values {
			raw[index] = item
		}
	default:
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		for _, part := range strings.Split(text, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if normalized, err := NormalizeHostname(part); err == nil {
				part = normalized
			}
			result = append(result, part)
		}
	}
	return result
}

func ExtractRealityTargets(config map[string]any, activeInboundTags []string) []Target {
	active := make(map[string]struct{}, len(activeInboundTags))
	for _, tag := range activeInboundTags {
		active[tag] = struct{}{}
	}
	inbounds, ok := config["inbounds"].([]any)
	if !ok {
		return []Target{}
	}
	identities := make(map[string]struct{})
	result := make([]Target, 0)
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]any)
		if !ok {
			continue
		}
		tag, ok := inbound["tag"].(string)
		if !ok {
			continue
		}
		if _, ok := active[tag]; !ok {
			continue
		}
		stream, ok := inbound["streamSettings"].(map[string]any)
		if !ok {
			continue
		}
		reality, ok := stream["realitySettings"].(map[string]any)
		if !ok {
			continue
		}
		rawDest, ok := reality["dest"].(string)
		if !ok {
			continue
		}
		dest, ok := ParseDestination(rawDest)
		if !ok {
			continue
		}
		normalizedHost, _ := NormalizeHostname(dest.Host)
		for _, rawSNI := range splitServerNames(reality["serverNames"]) {
			normalizedSNI, _ := NormalizeHostname(rawSNI)
			identitySNI := normalizedSNI
			if identitySNI == "" {
				identitySNI = strings.ToLower(rawSNI)
			}
			identityHost := normalizedHost
			if identityHost == "" {
				identityHost = strings.ToLower(dest.Host)
			}
			identity := fmt.Sprintf("%s\x00%s\x00%s\x00%d", tag, identitySNI, identityHost, dest.Port)
			if _, exists := identities[identity]; exists {
				continue
			}
			identities[identity] = struct{}{}
			tagCopy := tag
			result = append(result, Target{
				Identity: identity, InboundTag: &tagCopy, SNI: firstNonempty(normalizedSNI, rawSNI),
				NormalizedSNI: normalizedSNI, Host: dest.Host, NormalizedHost: normalizedHost,
				Port: dest.Port, Dest: dest.Dest,
				DestinationMatchesSNI: normalizedSNI != "" && normalizedHost != "" && normalizedSNI == normalizedHost,
			})
		}
	}
	return result
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
