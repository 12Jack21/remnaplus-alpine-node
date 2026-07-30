package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/12Jack21/remnaplus-alpine-node/internal/auditlog"
)

const maxAuditReadBytes = int64(50_000_000)

func (s *Server) handleGetAuditLogChunk(w http.ResponseWriter, r *http.Request) {
	if s.auditLogService == nil {
		writeError(w, http.StatusInternalServerError, "audit log service unavailable")
		return
	}
	request, err := decodeAuditChunkRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	chunk, err := s.auditLogService.ReadChunk(request)
	if errors.Is(err, auditlog.ErrInodeChanged) {
		writeError(w, http.StatusConflict, auditlog.ErrInodeChanged.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read audit log")
		return
	}
	writeJSON(w, http.StatusOK, envelope[auditlog.Chunk]{Response: chunk})
}

func (s *Server) handleGetAuditLogSourceMetadata(w http.ResponseWriter, r *http.Request) {
	if s.auditLogService == nil {
		writeError(w, http.StatusInternalServerError, "audit log service unavailable")
		return
	}
	source, err := decodeAuditSource(r.URL.Query().Get("source"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metadata, err := s.auditLogService.SourceMetadata(source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read audit log metadata")
		return
	}
	writeJSON(w, http.StatusOK, envelope[auditlog.SourceMetadata]{Response: metadata})
}

func (s *Server) handleCleanAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.auditLogService == nil {
		writeError(w, http.StatusInternalServerError, "audit log service unavailable")
		return
	}
	defer r.Body.Close()
	var request struct {
		RetentionDays int `json:"retentionDays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.RetentionDays <= 0 {
		writeError(w, http.StatusBadRequest, "retentionDays must be a positive integer")
		return
	}
	result, err := s.auditLogService.CleanLogs(request.RetentionDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clean audit logs")
		return
	}
	writeJSON(w, http.StatusOK, envelope[auditlog.CleanResult]{Response: result})
}

func decodeAuditChunkRequest(values url.Values) (auditlog.ChunkRequest, error) {
	source, err := decodeAuditSource(values.Get("source"))
	if err != nil {
		return auditlog.ChunkRequest{}, err
	}
	offset, err := optionalAuditInt(values, "offset", 0, 0)
	if err != nil {
		return auditlog.ChunkRequest{}, err
	}
	endOffset, err := optionalAuditInt(values, "endOffset", 0, 0)
	if err != nil {
		return auditlog.ChunkRequest{}, err
	}
	initialTailBytes, err := optionalAuditInt(values, "initialTailBytes", 1, maxAuditReadBytes)
	if err != nil {
		return auditlog.ChunkRequest{}, err
	}
	maxReadBytes, err := optionalAuditInt(values, "maxReadBytes", 1, maxAuditReadBytes)
	if err != nil {
		return auditlog.ChunkRequest{}, err
	}
	request := auditlog.ChunkRequest{
		EndOffset: endOffset,
		Offset:    offset,
		Source:    source,
	}
	if initialTailBytes != nil {
		request.InitialTailBytes = *initialTailBytes
	}
	if maxReadBytes != nil {
		request.MaxReadBytes = *maxReadBytes
	}
	if values.Has("expectedInode") {
		request.ExpectedInode = strings.TrimSpace(values.Get("expectedInode"))
		if request.ExpectedInode == "" {
			return auditlog.ChunkRequest{}, errors.New("expectedInode must not be empty")
		}
	}
	if values.Has("fromTimestamp") {
		parsed, err := time.Parse(time.RFC3339, values.Get("fromTimestamp"))
		if err != nil {
			return auditlog.ChunkRequest{}, errors.New("fromTimestamp must be an RFC3339 timestamp")
		}
		request.FromTimestamp = &parsed
	}
	return request, nil
}

func decodeAuditSource(value string) (auditlog.Source, error) {
	switch auditlog.Source(value) {
	case auditlog.SourceAccess:
		return auditlog.SourceAccess, nil
	case auditlog.SourceError:
		return auditlog.SourceError, nil
	default:
		return "", errors.New("source must be access or error")
	}
}

func optionalAuditInt(values url.Values, key string, minimum, maximum int64) (*int64, error) {
	if !values.Has(key) {
		return nil, nil
	}
	value, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || value < minimum || (maximum > 0 && value > maximum) {
		if maximum > 0 {
			return nil, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
		}
		return nil, fmt.Errorf("%s must be at least %d", key, minimum)
	}
	return &value, nil
}
