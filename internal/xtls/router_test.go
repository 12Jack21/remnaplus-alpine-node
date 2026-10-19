package xtls

import "testing"

func TestSourceIPRuleTagMatchesObjectHashStringEncoding(t *testing.T) {
	if got := SourceIPRuleTag("203.0.113.10"); got != "328532377296822858a6a166bd9e1823" {
		t.Fatalf("tag = %s", got)
	}
}
