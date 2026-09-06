package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type gatedBody struct {
	io.Reader
	ctx     context.Context
	entered chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *gatedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { b.entered <- struct{}{} })
	select {
	case <-b.release:
		return b.Reader.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}

func (*gatedBody) Close() error { return nil }

func assertNoStagingFiles(t *testing.T, destination string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".download-*"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("remaining staging files: %v, err=%v", paths, err)
	}
}

func TestConcurrentDownloadsOwnTheirStagingFiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	payload := "verified artifact"
	digest := sha256.Sum256([]byte(payload))
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200, ContentLength: -1, Request: req,
			Body: &gatedBody{Reader: strings.NewReader(payload), ctx: req.Context(), entered: entered, release: release},
		}, nil
	})}
	destination := filepath.Join(t.TempDir(), "core")
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := Download(ctx, "https://example.com/core", destination, Options{Client: client, ExpectedSHA256: hex.EncodeToString(digest[:])})
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("downloads did not both reach their staging files")
		}
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != payload {
		t.Fatalf("destination=%q err=%v", got, err)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact permissions: %v err=%v", info, err)
	}
	assertNoStagingFiles(t, destination)
}

func TestDownloadRejectsHTTPSDowngradeBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("untrusted"))
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer secure.Close()
	destination := filepath.Join(t.TempDir(), "core")
	_, err := Download(context.Background(), secure.URL, destination, Options{Client: secure.Client()})
	if err == nil || requests.Load() != 0 {
		t.Fatalf("downgrade error=%v requests=%d", err, requests.Load())
	}
	assertNoStagingFiles(t, destination)
}

func TestFailedStreamingDownloadsPreserveExistingFile(t *testing.T) {
	for _, scenario := range []string{"empty", "oversize", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				payload := ""
				if scenario == "oversize" {
					payload = "too large"
				}
				if scenario == "cancelled" {
					payload = "new"
					cancel()
				}
				return &http.Response{StatusCode: 200, Request: req, ContentLength: -1, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})}
			destination := filepath.Join(t.TempDir(), "core")
			if err := os.WriteFile(destination, []byte("current"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Download(ctx, "https://example.com/core", destination, Options{Client: client, MaxSize: 4})
			if err == nil {
				t.Fatal("expected download rejection")
			}
			got, err := os.ReadFile(destination)
			if err != nil || string(got) != "current" {
				t.Fatalf("destination changed: %q err=%v", got, err)
			}
			assertNoStagingFiles(t, destination)
		})
	}
}
