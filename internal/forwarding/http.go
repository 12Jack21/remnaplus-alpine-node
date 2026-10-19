package forwarding

import (
	"net/http"
	"strings"
	"time"
)

type writeJSONFn func(w http.ResponseWriter, status int, value any)

type HTTPService struct {
	configPath string
	socketPath string
}

func NewHTTPService(configPath, socketPath string) *HTTPService {
	return &HTTPService{configPath: configPath, socketPath: socketPath}
}

func (s *HTTPService) HandleGet(w http.ResponseWriter, r *http.Request, write writeJSONFn) {
	raw := strings.TrimSpace(r.URL.Query().Get("mappingNames"))
	if raw == "" || len(raw) > 3300 {
		write(w, http.StatusBadRequest, map[string]any{"message": "mappingNames must be a non-empty string of at most 3300 characters"})
		return
	}
	names := strings.Split(raw, ",")
	result := Get(names, s.configPath, s.socketPath)
	write(w, http.StatusOK, map[string]any{
		"response": map[string]any{
			"collectedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"connections": result.Connections,
			"diagnostics": result.Diagnostics,
			"mappings":    result.Mappings,
		},
	})
}
