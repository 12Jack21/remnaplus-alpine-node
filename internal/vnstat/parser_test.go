package vnstat

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestParseJSONResultAggregatesNonLoopbackInterfaces(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	result, err := ParseJSONResult([]byte(vnstatFixture(now)), ParseOptions{
		DefaultInterface:  "eth7",
		MaxAge:            time.Hour,
		Now:               now,
		RequireCurrentDay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Fatalf("error = %q, want nil", *result.Error)
	}
	want := &Snapshot{
		Daily: []DailyEntry{
			{Date: Date{Day: 17, Month: 7, Year: 2026}, RX: 20, TX: 10, Timestamp: int64Ptr(1784246400)},
			{Date: Date{Day: 18, Month: 7, Year: 2026}, RX: 300, TX: 120, Timestamp: int64Ptr(1784332800)},
		},
		TotalBytes: 4200,
	}
	if !reflect.DeepEqual(result.Snapshot, want) {
		t.Fatalf("snapshot = %#v, want %#v", result.Snapshot, want)
	}
}

func TestParseJSONResultRequiresTrackedDefaultInterface(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	result, err := ParseJSONResult([]byte(vnstatFixture(now)), ParseOptions{
		DefaultInterface: "eth0",
		MaxAge:           time.Hour,
		Now:              now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResultError(t, result, ErrorDefaultInterfaceNotTracked)
}

func TestParseJSONResultRejectsStaleAndFutureUpdates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	for name, updatedAt := range map[string]time.Time{
		"stale":  now.Add(-time.Hour - time.Second),
		"future": now.Add(time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := ParseJSONResult([]byte(vnstatFixture(updatedAt)), ParseOptions{
				DefaultInterface: "eth7",
				MaxAge:           time.Hour,
				Now:              now,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertResultError(t, result, ErrorDataStale)
		})
	}
}

func TestParseJSONResultRejectsMissingCurrentSourceDay(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 19, 2, 0, 0, 0, time.UTC)
	result, err := ParseJSONResult([]byte(vnstatFixture(now)), ParseOptions{
		DefaultInterface:  "eth7",
		MaxAge:            time.Hour,
		Now:               now,
		RequireCurrentDay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResultError(t, result, ErrorDataStale)
}

func TestParseJSONResultUsesLoopbackOnlyAsFallback(t *testing.T) {
	t.Parallel()

	raw, _ := json.Marshal(map[string]any{"interfaces": []any{
		map[string]any{
			"name": "lo",
			"traffic": map[string]any{
				"day":   []any{map[string]any{"date": map[string]int{"day": 18, "month": 7, "year": 2026}, "rx": 3, "tx": 4}},
				"total": map[string]int{"rx": 30, "tx": 40},
			},
		},
	}})
	result, err := ParseJSONResult(raw, ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := &Snapshot{
		Daily:      []DailyEntry{{Date: Date{Day: 18, Month: 7, Year: 2026}, RX: 3, TX: 4}},
		TotalBytes: 70,
	}
	if !reflect.DeepEqual(result.Snapshot, want) {
		t.Fatalf("snapshot = %#v, want %#v", result.Snapshot, want)
	}
}

func vnstatFixture(updatedAt time.Time) string {
	raw, _ := json.Marshal(map[string]any{"interfaces": []any{
		vnstatInterface("lo", updatedAt, []any{vnstatDay(18, 1, 2, 1784332800)}, 10, 20),
		vnstatInterface("ens3", updatedAt, []any{
			vnstatDay(18, 100, 50, 1784332800),
			vnstatDay(17, 20, 10, 1784246400),
		}, 1000, 500),
		vnstatInterface("eth7", updatedAt, []any{vnstatDay(18, 200, 70, 1784332800)}, 2000, 700),
		map[string]any{
			"name": "broken0",
			"traffic": map[string]any{
				"day": []any{map[string]any{"date": map[string]any{}}},
			},
		},
	}})
	return string(raw)
}

func vnstatInterface(name string, updatedAt time.Time, days []any, rx, tx int) map[string]any {
	return map[string]any{
		"name":    name,
		"updated": map[string]int64{"timestamp": updatedAt.Unix()},
		"traffic": map[string]any{"day": days, "total": map[string]int{"rx": rx, "tx": tx}},
	}
}

func vnstatDay(day, rx, tx int, timestamp int64) map[string]any {
	return map[string]any{
		"date":      map[string]int{"day": day, "month": 7, "year": 2026},
		"rx":        rx,
		"timestamp": timestamp,
		"tx":        tx,
	}
}

func assertResultError(t *testing.T, result Result, expected ErrorCode) {
	t.Helper()
	if result.Snapshot != nil {
		t.Fatalf("snapshot = %#v, want nil", result.Snapshot)
	}
	if result.Error == nil || *result.Error != expected {
		t.Fatalf("error = %v, want %q", result.Error, expected)
	}
}

func int64Ptr(value int64) *int64 { return &value }
