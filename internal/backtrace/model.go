package backtrace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Isp is the checkpoint operator a probe belongs to.
type Isp string

const (
	IspTelecom Isp = "telecom"
	IspUnicom  Isp = "unicom"
	IspMobile  Isp = "mobile"
)

// Target is one checkpoint the dashboard asks the node to trace.
type Target struct {
	ISP  Isp    `json:"isp"`
	Host string `json:"host"`
}

// Hop mirrors the dashboard node contract (node/libs/contract/commands/backtrace).
type Hop struct {
	TTL      int      `json:"ttl"`
	IP       *string  `json:"ip"`
	Hostname *string  `json:"hostname"`
	ASN      *string  `json:"asn"`
	Owner    *string  `json:"owner"`
	ISP      *string  `json:"isp"`
	Province *string  `json:"province"`
	Country  *string  `json:"country"`
	RTTMs    *float64 `json:"rttMs"`
	Success  bool     `json:"success"`
}

// Probe is one checkpoint result.
type Probe struct {
	ISP        Isp     `json:"isp"`
	Checkpoint string  `json:"checkpoint"`
	DurationMs int64   `json:"durationMs"`
	Success    bool    `json:"success"`
	Error      *string `json:"error"`
	Hops       []Hop   `json:"hops"`
	RawOutput  string  `json:"rawOutput"`
}

// Response is the node HTTP response body.
type Response struct {
	Probes      []Probe `json:"probes"`
	GeneratedAt string  `json:"generatedAt"`
}

type rawAddress struct {
	IP   string
	Zone string
}

// UnmarshalJSON accepts both shapes nexttrace emits: an object with IP/Zone
// and a bare address string.
func (address *rawAddress) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		address.IP = value
		return nil
	}
	var object struct {
		IP   string `json:"IP"`
		Zone string `json:"Zone"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	address.IP = object.IP
	address.Zone = object.Zone
	return nil
}

type rawGeo struct {
	ASNumber  string `json:"asnumber"`
	Country   string `json:"country"`
	CountryEn string `json:"country_en"`
	Owner     string `json:"owner"`
	ISP       string `json:"isp"`
	Prov      string `json:"prov"`
	ProvEn    string `json:"prov_en"`
}

type rawHop struct {
	Address  *rawAddress `json:"Address"`
	Geo      *rawGeo     `json:"Geo"`
	Hostname string      `json:"Hostname"`
	RTT      *float64    `json:"RTT"`
	Success  bool        `json:"Success"`
	TTL      *int        `json:"TTL"`
}

type rawStopReason struct {
	Hop    int    `json:"hop"`
	Reason string `json:"reason"`
}

type rawResult struct {
	Hops       [][]rawHop     `json:"Hops"`
	StopReason *rawStopReason `json:"StopReason"`
}

// parsedResult is the subset of nexttrace JSON the dashboard consumes.
type parsedResult struct {
	Hops       []Hop
	StopReason string
}

var asnPattern = regexp.MustCompile(`^(?i:AS)?(\d+)$`)
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var whitespacePattern = regexp.MustCompile(`\s+`)

// normalizeASN canonicalises "4134" and "as4134" to "AS4134".
func normalizeASN(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if match := asnPattern.FindStringSubmatch(trimmed); match != nil {
		canonical := "AS" + match[1]
		return &canonical
	}
	trimmed = truncate(trimmed, 64)
	return &trimmed
}

// truncate limits a string without splitting a UTF-8 rune.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func optionalString(value string, limit int) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	trimmed = truncate(trimmed, limit)
	return &trimmed
}

func parseHopAddress(address *rawAddress) *string {
	if address == nil {
		return nil
	}
	return optionalString(address.IP, 128)
}

func parseAttempt(attempt rawHop, fallbackTTL int) Hop {
	ttl := fallbackTTL
	if attempt.TTL != nil && *attempt.TTL >= 0 {
		ttl = *attempt.TTL
	}

	var rtt *float64
	if attempt.RTT != nil && !math.IsNaN(*attempt.RTT) && !math.IsInf(*attempt.RTT, 0) && *attempt.RTT >= 0 {
		// nexttrace reports RTT in nanoseconds.
		milliseconds := math.Round((*attempt.RTT/1_000_000)*10) / 10
		rtt = &milliseconds
	}

	var asn, owner, isp, province, country *string
	if attempt.Geo != nil {
		asn = normalizeASN(attempt.Geo.ASNumber)
		owner = optionalString(attempt.Geo.Owner, 256)
		isp = optionalString(attempt.Geo.ISP, 256)
		province = optionalString(firstNonEmpty(attempt.Geo.ProvEn, attempt.Geo.Prov), 128)
		country = optionalString(firstNonEmpty(attempt.Geo.CountryEn, attempt.Geo.Country), 128)
	}

	return Hop{
		TTL:      ttl,
		IP:       parseHopAddress(attempt.Address),
		Hostname: optionalString(attempt.Hostname, 256),
		ASN:      asn,
		Owner:    owner,
		ISP:      isp,
		Province: province,
		Country:  country,
		RTTMs:    rtt,
		Success:  attempt.Success,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// sanitizeError turns stderr into the single-line message the panel shows.
func sanitizeError(stderr, fallback string) string {
	cleaned := ansiPattern.ReplaceAllString(stderr, "")
	cleaned = strings.TrimSpace(whitespacePattern.ReplaceAllString(cleaned, " "))
	if cleaned == "" {
		cleaned = fallback
	}
	return truncate(cleaned, 1024)
}

func decodeJSON(raw []byte) (parsedResult, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if trimmed == "" {
		return parsedResult{}, fmt.Errorf("nexttrace-tiny returned no output.")
	}

	parsed, err := decodeJSONPayload(trimmed)
	if err == nil {
		return parsed, nil
	}

	// nexttrace occasionally prints a banner before the JSON payload; fall
	// back to the last line that starts with an object.
	lines := bytes.Split([]byte(trimmed), []byte("\n"))
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(strings.TrimSuffix(string(lines[index]), "\r"))
		if !strings.HasPrefix(line, "{") {
			continue
		}
		if parsed, lastErr := decodeJSONPayload(line); lastErr == nil {
			return parsed, nil
		}
	}

	return parsedResult{}, fmt.Errorf("nexttrace-tiny did not return JSON output.")
}

func decodeJSONPayload(payload string) (parsedResult, error) {
	var parsed rawResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return parsedResult{}, err
	}

	hops := make([]Hop, 0, len(parsed.Hops))
	for index, group := range parsed.Hops {
		if len(group) == 0 {
			continue
		}
		var chosen *rawHop
		for attemptIndex := range group {
			attempt := &group[attemptIndex]
			if attempt.Success && parseHopAddress(attempt.Address) != nil {
				chosen = attempt
				break
			}
		}
		if chosen == nil {
			chosen = &group[0]
		}
		hops = append(hops, parseAttempt(*chosen, index+1))
	}

	hops = sortHops(hops)
	if len(hops) > maxHops {
		hops = hops[:maxHops]
	}

	stopReason := ""
	if parsed.StopReason != nil {
		stopReason = strings.TrimSpace(parsed.StopReason.Reason)
	}
	return parsedResult{Hops: hops, StopReason: stopReason}, nil
}

func sortHops(hops []Hop) []Hop {
	// Hops arrive in TTL order for healthy traces; insertion sort keeps the
	// output deterministic without pulling in a dependency.
	for index := 1; index < len(hops); index++ {
		current := hops[index]
		position := index - 1
		for position >= 0 && hops[position].TTL > current.TTL {
			hops[position+1] = hops[position]
			position--
		}
		hops[position+1] = current
	}
	return hops
}
