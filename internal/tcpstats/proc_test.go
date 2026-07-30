package tcpstats

import (
	"path/filepath"
	"testing"
)

const procHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"

func TestDecodeHexIPAddress(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"852C1468":                         "104.20.44.133",
		"00000000000000000000FFFF010200C0": "192.0.2.1",
		"B80D0120000000000000000001000000": "2001:db8:0:0:0:0:0:1",
	}
	for encoded, expected := range tests {
		actual, ok := DecodeHexIPAddress(encoded)
		if !ok {
			t.Fatalf("DecodeHexIPAddress(%q) rejected valid address", encoded)
		}
		if actual != expected {
			t.Fatalf("DecodeHexIPAddress(%q) = %q, want %q", encoded, actual, expected)
		}
	}

	if _, ok := DecodeHexIPAddress("not-an-address"); ok {
		t.Fatal("invalid hex address was accepted")
	}
}

func TestParsePeerConnectionsFiltersAndAggregates(t *testing.T) {
	t.Parallel()

	ipv4 := []string{
		procHeader,
		"0: 1643BACD:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0",
		"1: 1643BACD:01BB 1A70528C:C552 01 00000000:00000000 00:00000000 00000000 0",
		"2: 1643BACD:01BB 1A70528C:C553 01 00000000:00000000 00:00000000 00000000 0",
		"3: 1643BACD:01BB 1A70528C:C554 06 00000000:00000000 00:00000000 00000000 0",
		"4: 1643BACD:A770 E42F1568:01BB 01 00000000:00000000 00:00000000 00000000 0",
		"5: 1643BACD:01BB 0100007F:620E 01 00000000:00000000 00:00000000 00000000 0",
	}
	ipv6 := []string{
		procHeader,
		"0: 00000000000000000000000000000000:01BB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 0",
		"1: 00000000000000000000000000000000:01BB B80D0120000000000000000001000000:C552 01 00000000:00000000 00:00000000 00000000 0",
		"2: 00000000000000000000000000000000:01BB 00000000000000000000FFFF010200C0:C553 01 00000000:00000000 00:00000000 00000000 0",
		"3: 00000000000000000000000000000000:01BB 00000000000000000000000001000000:C554 01 00000000:00000000 00:00000000 00000000 0",
		"4: 00000000000000000000000000000000:01BB 00000000000000000000FFFF0100007F:C555 01 00000000:00000000 00:00000000 00000000 0",
	}

	connections := ParsePeerConnections([]string{joinLines(ipv4), joinLines(ipv6)})
	byIP := make(map[string]PeerConnection, len(connections))
	for _, connection := range connections {
		byIP[connection.SourceIP] = connection
	}

	expected := map[string]PeerConnection{
		"140.82.112.26":        {Established: 2, SourceIP: "140.82.112.26", Total: 3},
		"192.0.2.1":            {Established: 1, SourceIP: "192.0.2.1", Total: 1},
		"2001:db8:0:0:0:0:0:1": {Established: 1, SourceIP: "2001:db8:0:0:0:0:0:1", Total: 1},
	}
	if len(byIP) != len(expected) {
		t.Fatalf("connections = %+v, want exactly %+v", connections, expected)
	}
	for sourceIP, want := range expected {
		if got := byIP[sourceIP]; got != want {
			t.Fatalf("connection %s = %+v, want %+v", sourceIP, got, want)
		}
	}
	if connections[0].SourceIP != "140.82.112.26" {
		t.Fatalf("connections are not sorted by established count: %+v", connections)
	}
}

func TestParsePeerConnectionsRequiresListener(t *testing.T) {
	t.Parallel()

	content := joinLines([]string{
		procHeader,
		"1: 1643BACD:A770 1A70528C:01BB 01 00000000:00000000 00:00000000 00000000 0",
	})
	if got := ParsePeerConnections([]string{content}); len(got) != 0 {
		t.Fatalf("connections = %+v, want empty without a listener", got)
	}
}

func TestParseProcNetTCPStats(t *testing.T) {
	t.Parallel()

	content := joinLines([]string{
		procHeader,
		"0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0",
		"1: 0100007F:1F90 0100007F:D431 01 00000000:00000000 00:00000000 00000000 0",
		"2: 0100007F:1F91 0100007F:D432 01 00000000:00000000 00:00000000 00000000 0",
		"3: 0100007F:1F92 0100007F:D433 06 00000000:00000000 03:00001770 00000000 0",
	})
	if got := ParseProcNetTCPStats(content); got != (Stats{Established: 2, Total: 4}) {
		t.Fatalf("stats = %+v, want established=2 total=4", got)
	}
}

func TestReadPeerConnectionsIgnoresMissingProcFiles(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing")
	if got := ReadPeerConnections([]string{missing}); len(got) != 0 {
		t.Fatalf("connections = %+v, want empty for missing proc data", got)
	}
}

func joinLines(lines []string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
