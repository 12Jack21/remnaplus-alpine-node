package warp

import (
	"context"
	"testing"

	"github.com/12Jack21/remnaplus-alpine-node/internal/xray"
)

func TestCheckWithoutConfigReportsNotConfigured(t *testing.T) {
	manager, err := xray.NewManager(xray.Options{
		XrayBin:            "definitely-missing-rw-core",
		GeoDir:             t.TempDir(),
		LogDir:             t.TempDir(),
		InternalSocketPath: "/run/remnawave.sock",
		InternalRESTToken:  "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := NewService(manager, 61001).Check(context.Background())
	if result.State != "not-configured" || result.Reason == nil || *result.Reason != "warp-outbound-missing" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
