package contract_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamProvenanceSeparatesTagAndCommit(t *testing.T) {
	raw, err := os.ReadFile("../../upstream.lock")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		SchemaVersion int                                                                          `json:"schemaVersion"`
		Repository    string                                                                       `json:"repository"`
		Baseline      struct{ Commit string }                                                      `json:"baseline"`
		Comparison    struct{ Commit, TagObject, ReportedNodeVersion, WireContractVersion string } `json:"comparison"`
		Local         struct {
			ReportedNodeVersion string
			ForwardingTelemetry bool
			RequiredExtensions  []string
		} `json:"local"`
		Units []struct {
			Name, Status, SourceCommit string
			LocalPaths, Tests          []string
		} `json:"units"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.SchemaVersion != 1 || lock.Repository != "https://github.com/ike-sh/remnawave-node-lite-go" {
		t.Fatal("unexpected upstream provenance schema or repository")
	}
	if lock.Baseline.Commit != "1984341771c69e6639176cfceb171d26c91b8064" ||
		lock.Comparison.Commit != "db79f4bc7474149708c397125b31b3c45c93f4dc" ||
		lock.Comparison.TagObject != "aaa9cf5a6831feced98a614516891f8757171368" {
		t.Fatal("upstream refs changed without updating the reviewed provenance boundary")
	}
	if lock.Comparison.ReportedNodeVersion != "3.3.2" || lock.Comparison.WireContractVersion != "3.2.3" ||
		lock.Local.ReportedNodeVersion != "2.8.0" || lock.Local.ForwardingTelemetry {
		t.Fatal("upstream comparison must not replace the local contract or forwarding boundary")
	}
	if !reflect.DeepEqual(lock.Local.RequiredExtensions, []string{"auditlog", "snihealth", "tcpstats", "vnstat", "accounting"}) {
		t.Fatal("local capability requirements changed")
	}
	foundLock := false
	for _, unit := range lock.Units {
		if unit.SourceCommit != lock.Baseline.Commit && unit.SourceCommit != lock.Comparison.Commit {
			t.Fatalf("unreviewed source for %s", unit.Name)
		}
		if len(unit.LocalPaths) == 0 || len(unit.Tests) == 0 {
			t.Fatalf("unit %s has no local scope or verification", unit.Name)
		}
		if unit.Name == "duplicate-instance-lock" && unit.Status == "accepted" {
			foundLock = true
		}
	}
	if !foundLock {
		t.Fatal("missing accepted duplicate-instance unit")
	}
}
