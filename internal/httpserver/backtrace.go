package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/backtrace"
)

const backtraceRequestBodyLimit = 1 * 1024 * 1024

func (s *Server) handleBacktraceRun(w http.ResponseWriter, r *http.Request) {
	if s.backtraceService == nil {
		writeError(w, http.StatusInternalServerError, "backtrace service unavailable")
		return
	}

	defer r.Body.Close()
	var request struct {
		Targets []backtrace.Target `json:"targets"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, backtraceRequestBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid backtrace request")
		return
	}

	probes, err := s.backtraceService.Run(r.Context(), request.Targets)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, "backtrace request cancelled")
			return
		}
		writeError(w, http.StatusBadRequest, strings.TrimSuffix(err.Error(), "."))
		return
	}

	writeJSON(w, http.StatusOK, envelope[backtrace.Response]{Response: backtrace.Response{
		Probes:      probes,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})
}
