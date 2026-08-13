package stats

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AccountingReader interface {
	ReadAccountingCounters(context.Context) (map[string]int64, string, error)
}

type AccountingDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type AccountingTraffic struct {
	Name     string
	Downlink string `json:"downlink"`
	Uplink   string `json:"uplink"`
}
type AccountingUser struct {
	Username string `json:"username"`
	Downlink string `json:"downlink"`
	Uplink   string `json:"uplink"`
}
type AccountingInbound struct {
	Inbound  string `json:"inbound"`
	Downlink string `json:"downlink"`
	Uplink   string `json:"uplink"`
}
type AccountingOutbound struct {
	Outbound string `json:"outbound"`
	Downlink string `json:"downlink"`
	Uplink   string `json:"uplink"`
}
type AccountingResponse struct {
	ContractVersion int                    `json:"contractVersion"`
	Diagnostics     []AccountingDiagnostic `json:"diagnostics"`
	Generation      string                 `json:"generation"`
	Inbounds        []AccountingInbound    `json:"inbounds"`
	Outbounds       []AccountingOutbound   `json:"outbounds"`
	Pending         bool                   `json:"pending"`
	SampleID        string                 `json:"sampleId"`
	SampledAt       string                 `json:"sampledAt"`
	Users           []AccountingUser       `json:"users"`
}
type accountingCheckpoint struct {
	Counters   map[string]string `json:"counters"`
	Generation string            `json:"generation"`
	SampleID   string            `json:"sampleId"`
}
type accountingPending struct {
	Diagnostics []AccountingDiagnostic `json:"diagnostics"`
	From        map[string]string      `json:"fromCounters"`
	Generation  string                 `json:"generation"`
	SampleID    string                 `json:"sampleId"`
	SampledAt   string                 `json:"sampledAt"`
	To          map[string]string      `json:"toCounters"`
}
type accountingJournal struct {
	Acknowledged *accountingCheckpoint `json:"acknowledged,omitempty"`
	Pending      *accountingPending    `json:"pending,omitempty"`
}

type AccountingSnapshotService struct {
	mu     sync.Mutex
	reader AccountingReader
	path   string
}

func NewAccountingSnapshotService(reader AccountingReader, path string) *AccountingSnapshotService {
	return &AccountingSnapshotService{reader: reader, path: path}
}

func (s *AccountingSnapshotService) Snapshot(ctx context.Context, acknowledge string) (AccountingResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	journal, err := s.read()
	if err != nil {
		return AccountingResponse{}, err
	}
	extra := []AccountingDiagnostic{}
	if acknowledge != "" && journal.Pending != nil {
		if acknowledge == journal.Pending.SampleID {
			journal.Acknowledged = &accountingCheckpoint{Counters: journal.Pending.To, Generation: journal.Pending.Generation, SampleID: journal.Pending.SampleID}
			journal.Pending = nil
			if err := s.write(journal); err != nil {
				return AccountingResponse{}, err
			}
		} else {
			extra = append(extra, AccountingDiagnostic{Code: "ACCOUNTING_ACK_UNKNOWN", Message: "Acknowledgement does not match the pending accounting sample."})
		}
	}
	if journal.Pending != nil {
		return accountingResponse(journal.Pending, extra), nil
	}
	counters, generation, err := s.reader.ReadAccountingCounters(ctx)
	if err != nil {
		return AccountingResponse{}, err
	}
	to := map[string]string{}
	for name, value := range counters {
		to[name] = strconv.FormatInt(value, 10)
	}
	from := to
	diagnostics := []AccountingDiagnostic{{Code: "ACCOUNTING_BASELINE", Message: "Initial cumulative baseline captured."}}
	if journal.Acknowledged != nil {
		from = journal.Acknowledged.Counters
		diagnostics = nil
		rebaseline := journal.Acknowledged.Generation != generation
		for name, value := range to {
			old, _ := strconv.ParseInt(from[name], 10, 64)
			current, _ := strconv.ParseInt(value, 10, 64)
			if current < old {
				rebaseline = true
			}
		}
		if rebaseline {
			from = to
			diagnostics = []AccountingDiagnostic{{Code: "ACCOUNTING_REBASELINE", Message: "Xray generation or cumulative counters changed; baseline replaced."}}
		}
	}
	pending := &accountingPending{Diagnostics: diagnostics, From: from, Generation: generation, SampleID: accountingID(), SampledAt: time.Now().UTC().Format(time.RFC3339Nano), To: to}
	journal.Pending = pending
	if err := s.write(journal); err != nil {
		return AccountingResponse{}, err
	}
	return accountingResponse(pending, extra), nil
}

func accountingResponse(p *accountingPending, extra []AccountingDiagnostic) AccountingResponse {
	rows := map[string]*AccountingTraffic{}
	for name, value := range p.To {
		parts := strings.Split(name, ">>>")
		if len(parts) != 3 || (parts[2] != "downlink" && parts[2] != "uplink") {
			continue
		}
		current, _ := strconv.ParseInt(value, 10, 64)
		old, _ := strconv.ParseInt(p.From[name], 10, 64)
		delta := current - old
		if delta < 0 {
			delta = 0
		}
		key := parts[0] + ">>>" + parts[1]
		if rows[key] == nil {
			rows[key] = &AccountingTraffic{Name: parts[1], Downlink: "0", Uplink: "0"}
		}
		if parts[2] == "downlink" {
			rows[key].Downlink = strconv.FormatInt(delta, 10)
		} else {
			rows[key].Uplink = strconv.FormatInt(delta, 10)
		}
	}
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	r := AccountingResponse{ContractVersion: 1, Diagnostics: append(append([]AccountingDiagnostic{}, p.Diagnostics...), extra...), Generation: p.Generation, Inbounds: []AccountingInbound{}, Outbounds: []AccountingOutbound{}, Pending: true, SampleID: p.SampleID, SampledAt: p.SampledAt, Users: []AccountingUser{}}
	for _, key := range keys {
		row := rows[key]
		switch {
		case strings.HasPrefix(key, "user>>>"):
			r.Users = append(r.Users, AccountingUser{row.Name, row.Downlink, row.Uplink})
		case strings.HasPrefix(key, "inbound>>>"):
			r.Inbounds = append(r.Inbounds, AccountingInbound{row.Name, row.Downlink, row.Uplink})
		case strings.HasPrefix(key, "outbound>>>"):
			r.Outbounds = append(r.Outbounds, AccountingOutbound{row.Name, row.Downlink, row.Uplink})
		}
	}
	return r
}
func (s *AccountingSnapshotService) read() (accountingJournal, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return accountingJournal{}, nil
	}
	if err != nil {
		return accountingJournal{}, err
	}
	var value accountingJournal
	err = json.Unmarshal(data, &value)
	return value, err
}
func (s *AccountingSnapshotService) write(value accountingJournal) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0750); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temp := s.path + "." + accountingID() + ".tmp"
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp, s.path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func accountingID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(value)
}
