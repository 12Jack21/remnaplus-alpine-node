package plugin

import "testing"

func TestPreStartStateRequiresBothGatesAndReturnsCopy(t *testing.T) {
	state := NewState()
	plugin, err := NewSyncPlugin("plugin", "test", map[string]any{
		"preStart": map[string]any{
			"enabled": true,
			"cleanupSockets": map[string]any{
				"enabled": true,
				"files":   []any{" /run/xray/a.sock ", "/run/xray/b.sock"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, accepted := state.UpdateFromSync(plugin); !accepted {
		t.Fatal("plugin configuration was not accepted")
	}
	enabled, files := state.PreStartCleanupSockets()
	if !enabled || len(files) != 2 || files[0] != "/run/xray/a.sock" {
		t.Fatalf("pre-start state = %v, %#v", enabled, files)
	}
	files[0] = "mutated"
	_, stored := state.PreStartCleanupSockets()
	if stored[0] != "/run/xray/a.sock" {
		t.Fatal("caller mutated stored pre-start paths")
	}
	state.Reset()
	if enabled, files := state.PreStartCleanupSockets(); enabled || files != nil {
		t.Fatalf("reset retained pre-start state: %v, %#v", enabled, files)
	}
}
