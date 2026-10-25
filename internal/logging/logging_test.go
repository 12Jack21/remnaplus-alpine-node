package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestStructuredDiagnostics(t *testing.T) {
	var output bytes.Buffer
	oldWriter, oldFlags := log.Writer(), log.Flags()
	defer func() { log.SetOutput(oldWriter); log.SetFlags(oldFlags) }()
	Configure(&output)
	log.Print("warning: password=private-password https://user:private-password@example.com/sources/private-source?token=private-token\nnext line")
	if strings.Count(output.String(), "\n") != 1 {
		t.Fatal("record spans multiple lines")
	}
	if strings.Contains(output.String(), "private-") {
		t.Fatal("credential leaked")
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != "WARN" || !strings.HasSuffix(record["time"].(string), "Z") {
		t.Fatalf("unexpected record: %v", record)
	}
	output.Reset()
	log.Print("xray/start failed: token=private-token")
	if !strings.Contains(output.String(), `"level":"ERROR"`) {
		t.Fatal("failed operation is not an error")
	}
}
