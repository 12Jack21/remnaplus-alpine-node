package contract

import (
	"os"
	"strings"
	"testing"
)

func TestV3FeatureMatrixKeepsAlpineBoundaries(t *testing.T) {
	versionSource, err := os.ReadFile("../version/version.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(versionSource)
	for _, required := range []string{
		`Version = "1.0.4"`,
		`ContractVersion = "2.8.0"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing Alpine version boundary %q", required)
		}
	}

	routes, err := os.ReadFile("routes_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"stats", "plugin", "sni", "audit"} {
		if !strings.Contains(strings.ToLower(string(routes)), required) {
			t.Fatalf("Alpine route evidence for %s is missing", required)
		}
	}
}
