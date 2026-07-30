package snihealth

import (
	"reflect"
	"testing"
)

func TestNormalizeHostnameAndParseDestination(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		" Example.COM. ": "example.com",
		"bücher.example": "xn--bcher-kva.example",
	} {
		got, err := NormalizeHostname(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeHostname(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"192.0.2.1", "https://example.com", "example.com:443", "*.example.com"} {
		if _, err := NormalizeHostname(input); err == nil {
			t.Fatalf("NormalizeHostname(%q) succeeded", input)
		}
	}

	tests := map[string]Destination{
		"example.com":            {Host: "example.com", Port: 443, Dest: "example.com:443"},
		"Example.COM.:8443":      {Host: "Example.COM.", Port: 8443, Dest: "Example.COM.:8443"},
		"[2001:db8::1]:443":      {Host: "2001:db8::1", Port: 443, Dest: "[2001:db8::1]:443"},
		"[2606:4700:4700::1111]": {Host: "2606:4700:4700::1111", Port: 443, Dest: "[2606:4700:4700::1111]:443"},
	}
	for input, want := range tests {
		got, ok := ParseDestination(input)
		if !ok || got != want {
			t.Fatalf("ParseDestination(%q) = %+v, %v; want %+v", input, got, ok, want)
		}
	}
	for _, input := range []string{"example.com:0", "example.com:65536", "https://example.com:443", "bad host:443"} {
		if _, ok := ParseDestination(input); ok {
			t.Fatalf("ParseDestination(%q) succeeded", input)
		}
	}
}

func TestExtractRealityTargetsUsesOnlyActiveTagsAndDeduplicates(t *testing.T) {
	t.Parallel()

	config := map[string]any{"inbounds": []any{
		map[string]any{"tag": "reality-main", "streamSettings": map[string]any{"realitySettings": map[string]any{
			"dest": "Example.COM.:8443", "serverNames": []any{" example.com,EXAMPLE.com. "},
		}}},
		map[string]any{"tag": "reality-mismatch", "streamSettings": map[string]any{"realitySettings": map[string]any{
			"dest": "destination.example:443", "serverNames": []any{"sni.example"},
		}}},
		map[string]any{"tag": "inactive", "streamSettings": map[string]any{"realitySettings": map[string]any{
			"dest": "inactive.example:443", "serverNames": []any{"inactive.example"},
		}}},
	}}

	targets := ExtractRealityTargets(config, []string{"reality-main", "reality-mismatch"})
	if len(targets) != 2 {
		t.Fatalf("targets = %+v", targets)
	}
	want := Target{
		Identity:              "reality-main\x00example.com\x00example.com\x008443",
		InboundTag:            stringPtr("reality-main"),
		SNI:                   "example.com",
		NormalizedSNI:         "example.com",
		Host:                  "Example.COM.",
		NormalizedHost:        "example.com",
		Port:                  8443,
		Dest:                  "Example.COM.:8443",
		DestinationMatchesSNI: true,
	}
	if !reflect.DeepEqual(targets[0], want) {
		t.Fatalf("first target = %#v, want %#v", targets[0], want)
	}
	if targets[1].DestinationMatchesSNI {
		t.Fatal("mismatched destination unexpectedly matched SNI")
	}
}

func stringPtr(value string) *string { return &value }
