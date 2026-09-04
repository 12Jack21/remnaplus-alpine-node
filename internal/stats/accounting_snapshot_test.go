package stats

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

type accountingReaderStub struct {
	counters   map[string]int64
	generation string
}

func (s *accountingReaderStub) ReadAccountingCounters(context.Context) (map[string]int64, string, error) {
	return s.counters, s.generation, nil
}

func TestAccountingSnapshotReplaysPendingUntilAcknowledged(t *testing.T) {
	reader := &accountingReaderStub{generation: "xray-1", counters: map[string]int64{
		"user>>>29>>>traffic>>>uplink":   10,
		"user>>>29>>>traffic>>>downlink": 20,
	}}
	service := NewAccountingSnapshotService(reader, filepath.Join(t.TempDir(), "accounting.json"))
	baseline, err := service.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	reader.counters["user>>>29>>>traffic>>>uplink"] = 25
	reader.counters["user>>>29>>>traffic>>>downlink"] = 40
	first, err := service.Snapshot(context.Background(), baseline.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Users[0].Uplink != "15" {
		t.Fatalf("uplink = %s, want 15", first.Users[0].Uplink)
	}
	if first.Users[0].Downlink != "20" {
		t.Fatalf("downlink = %s, want 20", first.Users[0].Downlink)
	}
	reader.counters["user>>>29>>>traffic>>>uplink"] = 40
	reader.counters["user>>>29>>>traffic>>>downlink"] = 70
	replay, err := service.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if replay.SampleID != first.SampleID || replay.Users[0].Uplink != "15" {
		t.Fatalf("pending sample was not replayed: %+v", replay)
	}
	if replay.Users[0].Downlink != "20" {
		t.Fatalf("replayed downlink = %s, want 20", replay.Users[0].Downlink)
	}
	next, err := service.Snapshot(context.Background(), first.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Users[0].Uplink != "15" {
		t.Fatalf("next uplink = %s, want 15", next.Users[0].Uplink)
	}
	repeated, err := service.Snapshot(context.Background(), first.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.SampleID != next.SampleID || repeated.Users[0].Uplink != "15" {
		t.Fatalf("repeated acknowledgement changed pending sample: %+v", repeated)
	}
	for _, diagnostic := range repeated.Diagnostics {
		if diagnostic.Code == "ACCOUNTING_ACK_UNKNOWN" {
			t.Fatalf("repeated acknowledgement was reported unknown: %+v", repeated.Diagnostics)
		}
	}
	unknown, err := service.Snapshot(context.Background(), "unknown-sample")
	if err != nil {
		t.Fatal(err)
	}
	foundUnknown := false
	for _, diagnostic := range unknown.Diagnostics {
		foundUnknown = foundUnknown || diagnostic.Code == "ACCOUNTING_ACK_UNKNOWN"
	}
	if !foundUnknown {
		t.Fatalf("unknown acknowledgement diagnostic missing: %+v", unknown.Diagnostics)
	}
}

func TestAccountingSnapshotAcceptsLegacyThreePartUserCounter(t *testing.T) {
	reader := &accountingReaderStub{generation: "xray-1", counters: map[string]int64{"user>>>29>>>uplink": 10}}
	service := NewAccountingSnapshotService(reader, filepath.Join(t.TempDir(), "accounting.json"))
	baseline, err := service.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	reader.counters["user>>>29>>>uplink"] = 19
	sample, err := service.Snapshot(context.Background(), baseline.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sample.Users) != 1 || sample.Users[0].Uplink != "9" {
		t.Fatalf("legacy user counter = %+v, want 9 uplink", sample.Users)
	}
}

func TestAccountingSnapshotReportsUnsupportedRecognizedCounter(t *testing.T) {
	reader := &accountingReaderStub{generation: "xray-1", counters: map[string]int64{
		"user>>>29>>>other>>>foo>>>uplink": 10,
		"system>>>cpu>>>uplink":            20,
	}}
	service := NewAccountingSnapshotService(reader, filepath.Join(t.TempDir(), "accounting.json"))
	sample, err := service.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sample.Users) != 0 {
		t.Fatalf("users = %+v, want empty", sample.Users)
	}
	if len(sample.Diagnostics) != 2 || sample.Diagnostics[1].Code != "ACCOUNTING_COUNTER_FORMAT_UNSUPPORTED" {
		t.Fatalf("diagnostics = %+v", sample.Diagnostics)
	}
}

func TestAccountingSnapshotPreservesMaxInt64AndRebaselines(t *testing.T) {
	reader := &accountingReaderStub{generation: "xray-1", counters: map[string]int64{"outbound>>>DIRECT>>>downlink": math.MaxInt64 - 10}}
	service := NewAccountingSnapshotService(reader, filepath.Join(t.TempDir(), "accounting.json"))
	baseline, err := service.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	reader.counters["outbound>>>DIRECT>>>downlink"] = math.MaxInt64
	sample, err := service.Snapshot(context.Background(), baseline.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Outbounds[0].Downlink != "10" {
		t.Fatalf("downlink = %s, want 10", sample.Outbounds[0].Downlink)
	}
	reader.generation = "xray-2"
	reader.counters["outbound>>>DIRECT>>>downlink"] = 1
	rebaseline, err := service.Snapshot(context.Background(), sample.SampleID)
	if err != nil {
		t.Fatal(err)
	}
	if rebaseline.Outbounds[0].Downlink != "0" || rebaseline.Diagnostics[0].Code != "ACCOUNTING_REBASELINE" {
		t.Fatalf("unexpected rebaseline: %+v", rebaseline)
	}
}
