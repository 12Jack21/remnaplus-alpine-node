package warp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/xray"
	"golang.org/x/net/proxy"
)

const targetURL = "https://cloudflare.com/cdn-cgi/trace"

type Response struct {
	CheckedAt    string  `json:"checkedAt"`
	LatencyMS    *int64  `json:"latencyMs"`
	ObservedWARP *string `json:"observedWarp"`
	Reason       *string `json:"reason"`
	State        string  `json:"state"`
	Target       string  `json:"target"`
}

type Service struct {
	manager *xray.Manager
	port    int
}

func NewService(manager *xray.Manager, port int) *Service {
	if port == 0 {
		port = 61001
	}
	return &Service{manager: manager, port: port}
}

func (s *Service) Check(ctx context.Context) Response {
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	result := Response{CheckedAt: checkedAt, Target: targetURL, State: "unusable"}
	config := s.manager.CurrentConfig()
	outbounds, _ := config["outbounds"].([]any)
	var warp map[string]any
	for _, item := range outbounds {
		candidate, ok := item.(map[string]any)
		if ok && candidate["tag"] == "WARP" {
			warp = candidate
			break
		}
	}
	if warp == nil {
		result.State = "not-configured"
		result.Reason = stringPtr("warp-outbound-missing")
		return result
	}
	if warp["protocol"] != "wireguard" {
		result.State = "unsupported"
		result.Reason = stringPtr("warp-outbound-invalid")
		return result
	}
	settings, _ := warp["settings"].(map[string]any)
	peers, _ := settings["peers"].([]any)
	var endpoint string
	if len(peers) > 0 {
		peer, _ := peers[0].(map[string]any)
		endpoint, _ = peer["endpoint"].(string)
	}
	if endpoint == "warp-is-disabled-on-this-remnanode.invalid:9" {
		result.State = "disabled"
		result.ObservedWARP = stringPtr("off")
		result.Reason = stringPtr("warp-disabled")
		return result
	}
	if !s.manager.Health().XrayInternalStatusCached {
		result.Reason = stringPtr("xray-not-running")
		return result
	}
	started := time.Now()
	body, err := s.fetch(ctx)
	latency := time.Since(started).Milliseconds()
	result.LatencyMS = &latency
	if err != nil {
		message := err.Error()
		if len(message) > 240 {
			message = message[:240]
		}
		result.Reason = &message
		result.LatencyMS = nil
		return result
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "warp=") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "warp="))
			if value == "off" || value == "on" || value == "plus" {
				result.ObservedWARP = &value
			}
			break
		}
	}
	if result.ObservedWARP == nil {
		result.Reason = stringPtr("warp-marker-missing")
		return result
	}
	if *result.ObservedWARP != "on" && *result.ObservedWARP != "plus" {
		result.Reason = stringPtr("warp-marker-off")
		return result
	}
	result.State = "usable"
	return result
}

func (s *Service) fetch(ctx context.Context) (string, error) {
	port := s.port
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid WARP probe port")
	}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), nil, &net.Dialer{Timeout: 8 * time.Second})
	if err != nil {
		return "", err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return "", fmt.Errorf("warp-proxy-context-unavailable")
	}
	transport := &http.Transport{DialContext: contextDialer.DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("User-Agent", "Remnawave-WARP-Health/1")
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return "", errors.New("warp-probe-timeout")
		}
		return "", fmt.Errorf("warp-probe-failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("warp-http-status-%d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil {
		return "", err
	}
	if len(body) > 8192 {
		return "", fmt.Errorf("warp-response-too-large")
	}
	return string(body), nil
}

func stringPtr(value string) *string { return &value }
