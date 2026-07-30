package vnstat

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

type ErrorCode string

const (
	ErrorCommandFailed              ErrorCode = "command-failed"
	ErrorDataStale                  ErrorCode = "data-stale"
	ErrorDefaultInterfaceNotTracked ErrorCode = "default-interface-not-tracked"
)

type Date struct {
	Day   int `json:"day"`
	Month int `json:"month"`
	Year  int `json:"year"`
}

type DailyEntry struct {
	Date      Date    `json:"date"`
	RX        float64 `json:"rx"`
	Timestamp *int64  `json:"timestamp,omitempty"`
	TX        float64 `json:"tx"`
}

type Snapshot struct {
	Daily      []DailyEntry `json:"daily"`
	TotalBytes float64      `json:"totalBytes"`
}

type Result struct {
	Error    *ErrorCode
	Snapshot *Snapshot
}

type ParseOptions struct {
	DefaultInterface  string
	MaxAge            time.Duration
	Now               time.Time
	RequireCurrentDay bool
}

type parsedInterface struct {
	daily       []DailyEntry
	loopback    bool
	name        string
	totalBytes  float64
	updatedAt   time.Time
	updatedOkay bool
}

func ParseJSONResult(raw []byte, options ParseOptions) (Result, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return Result{}, err
	}
	interfacesValue, _ := root["interfaces"].([]any)
	interfaces := make([]parsedInterface, 0, len(interfacesValue))
	for _, value := range interfacesValue {
		if parsed, ok := parseInterface(value); ok {
			interfaces = append(interfaces, parsed)
		}
	}

	defaultName := strings.TrimSpace(options.DefaultInterface)
	var defaultInterface *parsedInterface
	if defaultName != "" {
		for index := range interfaces {
			if interfaces[index].name == defaultName {
				defaultInterface = &interfaces[index]
				break
			}
		}
		if defaultInterface == nil {
			return errorResult(ErrorDefaultInterfaceNotTracked), nil
		}
		if options.MaxAge > 0 && !isFresh(*defaultInterface, options.Now, options.MaxAge) {
			return errorResult(ErrorDataStale), nil
		}
	}

	selected := make([]parsedInterface, 0, len(interfaces))
	for _, item := range interfaces {
		if !item.loopback {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		selected = interfaces
	}

	if options.RequireCurrentDay {
		calendarEntries := aggregateDaily(selected)
		if defaultInterface != nil {
			calendarEntries = defaultInterface.daily
		}
		if !hasCurrentSourceDay(calendarEntries, options.Now) {
			return errorResult(ErrorDataStale), nil
		}
	}

	totalBytes := float64(0)
	for _, item := range selected {
		totalBytes += item.totalBytes
	}
	return Result{Snapshot: &Snapshot{Daily: aggregateDaily(selected), TotalBytes: totalBytes}}, nil
}

func parseInterface(value any) (parsedInterface, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return parsedInterface{}, false
	}
	traffic, ok := record["traffic"].(map[string]any)
	if !ok {
		return parsedInterface{}, false
	}
	dailyValues, _ := traffic["day"].([]any)
	daily := make([]DailyEntry, 0, len(dailyValues))
	for _, value := range dailyValues {
		if entry, ok := parseDailyEntry(value); ok {
			daily = append(daily, entry)
		}
	}
	total, _ := traffic["total"].(map[string]any)
	totalBytes := positiveNumber(total["rx"]) + positiveNumber(total["tx"])
	if len(daily) == 0 && totalBytes == 0 {
		return parsedInterface{}, false
	}
	name, _ := record["name"].(string)
	updated, _ := record["updated"].(map[string]any)
	updatedSeconds, updatedOkay := positiveInteger(updated["timestamp"])
	return parsedInterface{
		daily:       daily,
		loopback:    name == "lo",
		name:        name,
		totalBytes:  totalBytes,
		updatedAt:   time.Unix(updatedSeconds, 0),
		updatedOkay: updatedOkay,
	}, true
}

func parseDailyEntry(value any) (DailyEntry, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return DailyEntry{}, false
	}
	date, ok := record["date"].(map[string]any)
	if !ok {
		return DailyEntry{}, false
	}
	year, yearOK := integer(date["year"])
	month, monthOK := integer(date["month"])
	day, dayOK := integer(date["day"])
	if !yearOK || !monthOK || !dayOK {
		return DailyEntry{}, false
	}
	entry := DailyEntry{
		Date: Date{Day: int(day), Month: int(month), Year: int(year)},
		RX:   positiveNumber(record["rx"]),
		TX:   positiveNumber(record["tx"]),
	}
	if timestamp, ok := positiveInteger(record["timestamp"]); ok {
		entry.Timestamp = &timestamp
	}
	return entry, true
}

func aggregateDaily(interfaces []parsedInterface) []DailyEntry {
	byDate := make(map[int]DailyEntry)
	for _, item := range interfaces {
		for _, entry := range item.daily {
			key := entry.Date.Year*10000 + entry.Date.Month*100 + entry.Date.Day
			current, exists := byDate[key]
			if !exists {
				current.Date = entry.Date
				current.Timestamp = entry.Timestamp
			} else if current.Timestamp == nil {
				current.Timestamp = entry.Timestamp
			}
			current.RX += entry.RX
			current.TX += entry.TX
			byDate[key] = current
		}
	}
	keys := make([]int, 0, len(byDate))
	for key := range byDate {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	result := make([]DailyEntry, 0, len(keys))
	for _, key := range keys {
		result = append(result, byDate[key])
	}
	return result
}

func hasCurrentSourceDay(entries []DailyEntry, now time.Time) bool {
	if now.IsZero() || len(entries) == 0 {
		return false
	}
	location := time.UTC
	for _, entry := range entries {
		if entry.Timestamp == nil {
			continue
		}
		dateUTC := time.Date(entry.Date.Year, time.Month(entry.Date.Month), entry.Date.Day, 0, 0, 0, 0, time.UTC)
		offset := int(dateUTC.Unix() - *entry.Timestamp)
		if offset >= -14*60*60 && offset <= 14*60*60 {
			location = time.FixedZone("vnstat-source", offset)
			break
		}
	}
	sourceNow := now.In(location)
	year, month, day := sourceNow.Date()
	for _, entry := range entries {
		if entry.Date.Year == year && entry.Date.Month == int(month) && entry.Date.Day == day {
			return true
		}
	}
	return false
}

func isFresh(item parsedInterface, now time.Time, maxAge time.Duration) bool {
	return item.updatedOkay && !now.IsZero() && !item.updatedAt.After(now) && now.Sub(item.updatedAt) <= maxAge
}

func positiveNumber(value any) float64 {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return 0
	}
	return number
}

func integer(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok || math.Trunc(number) != number {
		return 0, false
	}
	return int64(number), true
}

func positiveInteger(value any) (int64, bool) {
	number, ok := integer(value)
	return number, ok && number > 0
}

func errorResult(code ErrorCode) Result {
	return Result{Error: &code}
}
