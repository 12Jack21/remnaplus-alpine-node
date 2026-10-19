package vision

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/12Jack21/remnaplus-alpine-node/internal/xtls"
)

type Router interface {
	RouterAddSourceIPRule(ctx context.Context, ruleTag, outbound, ip string, appendRule bool) xtls.RouterResult
	RouterRemoveRuleByTag(ctx context.Context, ruleTag string) xtls.RouterResult
}

type Service struct {
	router Router
}

func NewService(router Router) *Service { return &Service{router: router} }

func (s *Service) HandleBlockIP(w http.ResponseWriter, r *http.Request, write func(http.ResponseWriter, int, any)) {
	var request struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || net.ParseIP(strings.TrimSpace(request.IP)) == nil {
		write(w, http.StatusBadRequest, map[string]any{"message": "invalid IP address"})
		return
	}
	ip := strings.TrimSpace(request.IP)
	result := s.router.RouterAddSourceIPRule(r.Context(), xtls.SourceIPRuleTag(ip), "BLOCK", ip, true)
	s.writeResult(w, write, result)
}

func (s *Service) HandleUnblockIP(w http.ResponseWriter, r *http.Request, write func(http.ResponseWriter, int, any)) {
	var request struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || net.ParseIP(strings.TrimSpace(request.IP)) == nil {
		write(w, http.StatusBadRequest, map[string]any{"message": "invalid IP address"})
		return
	}
	ip := strings.TrimSpace(request.IP)
	result := s.router.RouterRemoveRuleByTag(r.Context(), xtls.SourceIPRuleTag(ip))
	s.writeResult(w, write, result)
}

func (s *Service) writeResult(w http.ResponseWriter, write func(http.ResponseWriter, int, any), result xtls.RouterResult) {
	status := http.StatusOK
	if !result.OK {
		status = http.StatusInternalServerError
	}
	message := any(nil)
	if result.Message != "" {
		message = result.Message
	}
	write(w, status, map[string]any{"response": map[string]any{"success": result.OK, "error": message}})
}
