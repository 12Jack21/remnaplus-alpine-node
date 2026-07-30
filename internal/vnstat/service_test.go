package vnstat

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestServiceRunsPinnedCommandAndCachesByInterface(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	runs := 0
	service := NewService(Options{
		CacheTTL: time.Minute,
		MaxAge:   time.Hour,
		Now:      func() time.Time { return now },
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			runs++
			if name != "vnstat" || !reflect.DeepEqual(args, []string{"--json", "d", "62"}) {
				t.Fatalf("command = %q %v, want vnstat --json d 62", name, args)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 5100*time.Millisecond {
				t.Fatalf("vnstat deadline = %v, want about five seconds", deadline)
			}
			return []byte(vnstatFixture(now)), nil
		},
	})

	first := service.Snapshot(context.Background(), "eth7")
	second := service.Snapshot(context.Background(), "eth7")
	if first.Error != nil || second.Error != nil || runs != 1 {
		t.Fatalf("cached results = %#v %#v, runs = %d", first, second, runs)
	}
	service.Snapshot(context.Background(), "ens3")
	if runs != 2 {
		t.Fatalf("runs = %d, want cache separated by interface", runs)
	}
	now = now.Add(time.Minute + time.Second)
	service.Snapshot(context.Background(), "eth7")
	if runs != 3 {
		t.Fatalf("runs = %d, want expired cache refresh", runs)
	}
}

func TestServiceReturnsCommandFailureWithoutContainerMountDiagnostic(t *testing.T) {
	t.Parallel()

	service := NewService(Options{
		Runner: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("vnstat unavailable")
		},
	})
	result := service.Snapshot(context.Background(), "eth0")
	assertResultError(t, result, ErrorCommandFailed)
	if result.Error != nil && *result.Error == ErrorCode("mount-not-read-only") {
		t.Fatal("native vnStat emitted the Docker-only mount-not-read-only diagnostic")
	}
}

func TestServiceTreatsInvalidJSONAsCommandFailure(t *testing.T) {
	t.Parallel()

	service := NewService(Options{
		Runner: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("not-json"), nil
		},
	})
	assertResultError(t, service.Snapshot(context.Background(), "eth0"), ErrorCommandFailed)
}
