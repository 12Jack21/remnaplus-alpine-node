package tcpstats

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestParseListeningPortsCollapsesProtocolsAndSorts(t *testing.T) {
	t.Parallel()

	output := joinLines([]string{
		"tcp LISTEN 0 4096 127.0.0.1:61000 0.0.0.0:*",
		"tcp LISTEN 0 4096 *:44112 *:*",
		"udp UNCONN 0 0 *:44112 *:*",
		"tcp LISTEN 0 4096 [::]:55273 [::]:*",
		"tcp LISTEN 0 4096 [::ffff:127.0.0.1]:55273 [::]:*",
		"udp UNCONN 0 0 *:0 *:*",
		"malformed output",
	})
	want := []ListeningPort{
		{Networks: []string{"tcp", "udp"}, Port: 44112},
		{Networks: []string{"tcp"}, Port: 55273},
		{Networks: []string{"tcp"}, Port: 61000},
	}
	if got := ParseListeningPorts(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("listeners = %+v, want %+v", got, want)
	}
}

func TestCollectListeningPortsReturnsNilOnCommandFailure(t *testing.T) {
	t.Parallel()

	got := collectListeningPorts(context.Background(), func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("ss unavailable")
	})
	if got != nil {
		t.Fatalf("listeners = %+v, want nil on command failure", got)
	}
}

func TestCollectListeningPortsUsesBoundedSSCommand(t *testing.T) {
	t.Parallel()

	var command string
	var args []string
	got := collectListeningPorts(context.Background(), func(ctx context.Context, name string, values ...string) ([]byte, error) {
		command = name
		args = append([]string(nil), values...)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2100*time.Millisecond {
			t.Fatalf("ss context deadline = %v, want about two seconds", deadline)
		}
		return []byte("tcp LISTEN 0 4096 *:443 *:*"), nil
	})
	if command != "ss" || !reflect.DeepEqual(args, []string{"-H", "-lnut"}) {
		t.Fatalf("command = %q %v, want ss -H -lnut", command, args)
	}
	if !reflect.DeepEqual(got, []ListeningPort{{Networks: []string{"tcp"}, Port: 443}}) {
		t.Fatalf("listeners = %+v", got)
	}
}
