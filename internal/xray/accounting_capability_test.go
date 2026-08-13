package xray

import (
	"encoding/json"
	"testing"
)

func TestStartResponsePublishesAccountingCapability(t *testing.T) {
	response := (&Manager{}).startResponse(false, nil)
	if !response.NodeInformation.AccountingSnapshot {
		t.Fatal("accountingSnapshot capability is false")
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		NodeInformation struct {
			AccountingSnapshot bool `json:"accountingSnapshot"`
		} `json:"nodeInformation"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.NodeInformation.AccountingSnapshot {
		t.Fatalf("serialized response omitted accountingSnapshot: %s", raw)
	}
}
