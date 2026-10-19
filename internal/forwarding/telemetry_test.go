package forwarding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfiguredProxyMappings(t *testing.T) {
	content := "version\tname\tprotocol\tsource_ip_mode\tlisten_port\ttarget_ip\ttarget_port\tenabled\n" +
		"1\tTo_VMRL1\ttcp\tproxy_protocol\t443\t10.50.0.118\t443\t1\n" +
		"1\t$Mapping\ttcp\tproxy_protocol\t7443\t10.50.0.119\t443\t1\n" +
		"1\tDisabled\ttcp\tproxy_protocol\t8443\t10.50.0.118\t443\t0\n" +
		"1\tNatOnly\ttcp\tnat\t9443\t10.50.0.118\t443\t1\n"
	mappings := ParseConfiguredProxyMappings(content)
	if len(mappings) != 2 || mappings["$Mapping"].TableName != "mf__Mapping" {
		t.Fatalf("unexpected mappings: %#v", mappings)
	}
}

func TestParseHAProxyStickTable(t *testing.T) {
	connections, err := ParseHAProxyStickTable("To_VMRL1", "# table: mf_To_VMRL1\n"+
		"0x1: key=203.0.113.10 use=1 exp=599999 conn_cur=7\n"+
		"0x2: key=2001:db8::10 use=1 exp=599999 conn_cur=2\n")
	if err != nil || len(connections) != 2 || connections[1].ConnectionCount != 2 {
		t.Fatalf("connections=%#v err=%v", connections, err)
	}
	if _, err := ParseHAProxyStickTable("To_VMRL1", "key=not-an-ip conn_cur=3"); err == nil {
		t.Fatal("invalid source IP unexpectedly accepted")
	}
	if _, err := ParseHAProxyStickTable("To_VMRL1", "key=203.0.113.10 conn_cur=2\nkey=203.0.113.10 conn_cur=1"); err == nil {
		t.Fatal("duplicate source IP unexpectedly accepted")
	}
}

func TestGetMissingConfigurationReturnsDiagnostics(t *testing.T) {
	result := Get([]string{"TO_VML128"}, filepath.Join(t.TempDir(), "missing.tsv"), "")
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "mapping-not-configured" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Diagnostics[0].Message == "" {
		t.Fatal("missing diagnostic message")
	}
}

func TestGetReadsConfiguredMappingAndReportsSocketFailure(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "mappings.tsv")
	content := "version\tname\tprotocol\tsource_ip_mode\tlisten_port\ttarget_ip\ttarget_port\tenabled\n" +
		"1\tTo_VMRL1\ttcp\tproxy_protocol\t443\t10.50.0.118\t443\t1\n"
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Get([]string{"To_VMRL1"}, config, filepath.Join(dir, "missing.sock"))
	if len(result.Mappings) != 1 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "socket-unavailable" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
