package xray

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// HashedSet mirrors @remnawave/hashed-set: XOR of per-string djb2Dual hashes.
type HashedSet struct {
	members  map[string]struct{}
	hashHigh uint32
	hashLow  uint32
}

func NewHashedSet(items ...string) *HashedSet {
	set := &HashedSet{members: make(map[string]struct{}, len(items))}
	for _, item := range items {
		set.Add(item)
	}
	return set
}

func (s *HashedSet) Add(value string) {
	if _, exists := s.members[value]; exists {
		return
	}
	s.members[value] = struct{}{}
	high, low := djb2Dual(value)
	s.hashHigh ^= high
	s.hashLow ^= low
}

func (s *HashedSet) Delete(value string) bool {
	if _, exists := s.members[value]; !exists {
		return false
	}
	delete(s.members, value)
	high, low := djb2Dual(value)
	s.hashHigh ^= high
	s.hashLow ^= low
	return true
}

func (s *HashedSet) Size() int {
	return len(s.members)
}

func (s *HashedSet) Hash64String() string {
	// JavaScript XOR leaves signed int32 values. The dashboard formats each
	// signed word with toString(16).padStart(8, '0'), including a possible '-'.
	word := func(value uint32) string {
		text := strconv.FormatInt(int64(int32(value)), 16)
		if len(text) < 8 {
			text = strings.Repeat("0", 8-len(text)) + text
		}
		return text
	}
	return word(s.hashHigh) + word(s.hashLow)
}

func djb2Dual(value string) (high, low uint32) {
	high = 5381
	low = 5387
	// charCodeAt hashes UTF-16 code units, rather than UTF-8 bytes.
	for _, unit := range utf16.Encode([]rune(value)) {
		char := uint32(unit)
		high = (high<<5 + high + char)
		low = (low<<6 + low + char*37)
	}
	return high, low
}

func hash64FromItems(items []string) string {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	set := NewHashedSet(sorted...)
	return set.Hash64String()
}
