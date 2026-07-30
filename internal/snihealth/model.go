package snihealth

import "time"

type Verdict string

const (
	VerdictUsable     Verdict = "usable"
	VerdictWarning    Verdict = "warning"
	VerdictUnusable   Verdict = "unusable"
	VerdictUnverified Verdict = "unverified"
)

type CheckStatus string

const (
	StatusPass        CheckStatus = "pass"
	StatusWarning     CheckStatus = "warning"
	StatusFail        CheckStatus = "fail"
	StatusUnavailable CheckStatus = "unavailable"
)

type Check struct {
	ID            string      `json:"id"`
	Status        CheckStatus `json:"status"`
	ObservedValue any         `json:"observedValue"`
	Reason        string      `json:"reason"`
	Hint          *string     `json:"hint"`
}

type Address struct {
	Address  string      `json:"address"`
	Family   int         `json:"family"`
	IsPublic bool        `json:"isPublic"`
	Status   CheckStatus `json:"status"`
	Error    *string     `json:"error"`
}

type Result struct {
	CorrelationID  *string   `json:"correlationId"`
	InboundTag     *string   `json:"inboundTag"`
	SNI            string    `json:"sni"`
	Dest           string    `json:"dest"`
	Hostname       string    `json:"hostname"`
	Port           int       `json:"port"`
	Verdict        Verdict   `json:"verdict"`
	Healthy        bool      `json:"healthy"`
	LatencyMS      *int      `json:"latencyMs"`
	Error          *string   `json:"error"`
	Checks         []Check   `json:"checks"`
	Addresses      []Address `json:"addresses"`
	LatencySamples []int     `json:"latencySamplesMs"`
	JitterMS       *int      `json:"jitterMs"`
	CheckedAt      time.Time `json:"checkedAt"`
}

type Target struct {
	Identity              string
	CorrelationID         *string
	InboundTag            *string
	SNI                   string
	NormalizedSNI         string
	Host                  string
	NormalizedHost        string
	Port                  int
	Dest                  string
	DestinationMatchesSNI bool
}

type Destination struct {
	Host string
	Port int
	Dest string
}

type CandidateTarget struct {
	CorrelationID string `json:"correlationId"`
	Hostname      string `json:"hostname"`
	Port          int    `json:"port"`
}

type Counts struct {
	Usable     int `json:"usable"`
	Warning    int `json:"warning"`
	Unusable   int `json:"unusable"`
	Unverified int `json:"unverified"`
}

type ProbeResponse struct {
	Mode             string    `json:"mode"`
	Results          []Result  `json:"results"`
	AggregateVerdict Verdict   `json:"aggregateVerdict"`
	Counts           Counts    `json:"counts"`
	UnhealthyCount   int       `json:"unhealthyCount"`
	CheckedAt        time.Time `json:"checkedAt"`
}

type StatusResponse struct {
	Results        []Result  `json:"results"`
	UnhealthyCount int       `json:"unhealthyCount"`
	CheckedAt      time.Time `json:"checkedAt"`
}
