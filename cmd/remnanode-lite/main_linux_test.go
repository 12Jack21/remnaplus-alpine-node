//go:build linux

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/12Jack21/remnaplus-alpine-node/internal/instance"
)

func TestDuplicateExitsBeforeConfiguration(t *testing.T) {
	if os.Getenv("RNL_TEST_DUPLICATE_PROCESS") == "1" {
		os.Args = []string{"remnanode-lite"}
		main()
		return
	}
	lock, acquired, err := instance.Acquire()
	if err != nil || !acquired || lock == nil {
		t.Fatalf("acquire parent lock: acquired=%v err=%v", acquired, err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	child := exec.Command(os.Args[0], "-test.run=^TestDuplicateExitsBeforeConfiguration$")
	child.Env = append(os.Environ(), "RNL_TEST_DUPLICATE_PROCESS=1")
	output, err := child.CombinedOutput()
	if err == nil {
		t.Fatal("duplicate node did not exit with an error")
	}
	if !strings.Contains(string(output), "another Remnawave Node is already running") {
		t.Fatalf("unexpected duplicate failure: %s", output)
	}
	if strings.Contains(string(output), "load config") {
		t.Fatalf("duplicate reached configuration initialization: %s", output)
	}
}
