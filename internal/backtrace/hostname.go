package backtrace

import (
	"fmt"
	"regexp"
	"strings"
)

// Mirror of the dashboard/contract target policy: never traceroute private,
// loopback, link-local, shared or documentation ranges from a node.
var reservedIPv4Ranges = [][2]uint32{
	{0x00000000, 0x00ffffff}, // 0.0.0.0/8
	{0x0a000000, 0x0affffff}, // 10.0.0.0/8
	{0x64400000, 0x647fffff}, // 100.64.0.0/10
	{0x7f000000, 0x7fffffff}, // 127.0.0.0/8
	{0xa9fe0000, 0xa9feffff}, // 169.254.0.0/16
	{0xac100000, 0xac1fffff}, // 172.16.0.0/12
	{0xc0000000, 0xc00000ff}, // 192.0.0.0/24
	{0xc0000200, 0xc00002ff}, // 192.0.2.0/24
	{0xc0a80000, 0xc0a8ffff}, // 192.168.0.0/16
	{0xc6120000, 0xc613ffff}, // 198.18.0.0/15
	{0xc6336400, 0xc63364ff}, // 198.51.100.0/24
	{0xcb007100, 0xcb0071ff}, // 203.0.113.0/24
	{0xe0000000, 0xefffffff}, // 224.0.0.0/4 multicast
	{0xf0000000, 0xffffffff}, // 240.0.0.0/4 reserved + broadcast
}

var reservedHostSuffixes = []string{".local", ".localhost", ".internal", ".home.arpa"}

var ipv4Pattern = regexp.MustCompile(`^\d{1,3}(?:\.\d{1,3}){3}$`)
var hostnameLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var forbiddenHostCharacters = regexp.MustCompile(`[\s/:@*?#]`)

// NormalizeTarget validates and canonicalises one checkpoint host.
func NormalizeTarget(value string) (string, error) {
	normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")

	if normalized == "" ||
		len(normalized) > 253 ||
		forbiddenHostCharacters.MatchString(normalized) ||
		strings.HasPrefix(normalized, "-") {
		return "", fmt.Errorf("Target must be a public DNS hostname or IPv4 address.")
	}

	if ipv4Pattern.MatchString(normalized) {
		return normalizeIPv4(normalized)
	}

	labels := strings.Split(normalized, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("Target must be a public DNS hostname or IPv4 address.")
	}
	for _, label := range labels {
		if !hostnameLabelPattern.MatchString(label) {
			return "", fmt.Errorf("Target must be a public DNS hostname or IPv4 address.")
		}
	}

	// A numeric final label is never a real TLD; these are legacy
	// inet_aton forms (127.1, 0x7f.1, 10.1) resolvers expand into private
	// addresses.
	if isNumeric(labels[len(labels)-1]) {
		return "", fmt.Errorf("Target must be a valid IPv4 address.")
	}

	for _, suffix := range reservedHostSuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return "", fmt.Errorf("Target must not use a private-use hostname suffix.")
		}
	}

	return normalized, nil
}

func normalizeIPv4(value string) (string, error) {
	parts := strings.Split(value, ".")
	numeric := uint32(0)
	for _, part := range parts {
		octet, ok := parseOctet(part)
		if !ok {
			return "", fmt.Errorf("Target must be a valid IPv4 address.")
		}
		numeric = numeric<<8 | uint32(octet)
	}
	for _, reserved := range reservedIPv4Ranges {
		if numeric >= reserved[0] && numeric <= reserved[1] {
			return "", fmt.Errorf("Target must not be a private or reserved IPv4 address.")
		}
	}
	return value, nil
}

func parseOctet(value string) (int, bool) {
	if value == "" || len(value) > 3 {
		return 0, false
	}
	parsed := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
		parsed = parsed*10 + int(character-'0')
	}
	return parsed, parsed <= 255
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
