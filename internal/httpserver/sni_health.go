package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/12Jack21/remnaplus-alpine-node/internal/snihealth"
)

func (s *Server) handleGetSNIHealthStatus(w http.ResponseWriter, r *http.Request) {
	if s.sniHealthService == nil {
		writeError(w, http.StatusInternalServerError, "SNI health service unavailable")
		return
	}
	writeJSON(w, http.StatusOK, envelope[snihealth.StatusResponse]{Response: s.sniHealthService.Status()})
}

func (s *Server) handleProbeSNIHealth(w http.ResponseWriter, r *http.Request) {
	if s.sniHealthService == nil {
		writeError(w, http.StatusInternalServerError, "SNI health service unavailable")
		return
	}
	defer r.Body.Close()
	var request struct {
		Mode    string                       `json:"mode"`
		Targets *[]snihealth.CandidateTarget `json:"targets"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid SNI health probe request")
		return
	}
	var results []snihealth.Result
	switch request.Mode {
	case "active":
		if request.Targets != nil {
			writeError(w, http.StatusBadRequest, "active probes do not accept candidate targets")
			return
		}
		results = s.sniHealthService.RunActive(r.Context())
	case "candidates":
		if request.Targets == nil {
			writeError(w, http.StatusBadRequest, "candidate probes require targets")
			return
		}
		var err error
		results, err = s.sniHealthService.RunCandidates(r.Context(), *request.Targets)
		if err != nil {
			writeError(w, http.StatusBadRequest, strings.TrimSuffix(err.Error(), "."))
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "mode must be active or candidates")
		return
	}
	writeJSON(w, http.StatusOK, envelope[snihealth.ProbeResponse]{Response: snihealth.BuildProbeResponse(request.Mode, results, s.sniHealthService.Now())})
}
