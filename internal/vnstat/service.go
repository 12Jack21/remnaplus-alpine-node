package vnstat

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultCacheTTL       = 5 * time.Minute
	defaultCommandTimeout = 5 * time.Second
	defaultMaxAge         = 24 * time.Hour
)

type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

type Options struct {
	CacheTTL       time.Duration
	CommandTimeout time.Duration
	MaxAge         time.Duration
	Now            func() time.Time
	Runner         Runner
}

type cacheEntry struct {
	fetchedAt time.Time
	result    Result
}

type Service struct {
	cacheTTL       time.Duration
	commandTimeout time.Duration
	maxAge         time.Duration
	now            func() time.Time
	runner         Runner
	mu             sync.Mutex
	cache          map[string]cacheEntry
}

var defaultService = NewService(Options{})

func NewService(options Options) *Service {
	if options.CacheTTL <= 0 {
		options.CacheTTL = defaultCacheTTL
	}
	if options.CommandTimeout <= 0 {
		options.CommandTimeout = defaultCommandTimeout
	}
	if options.MaxAge <= 0 {
		options.MaxAge = defaultMaxAge
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Runner == nil {
		options.Runner = runCommand
	}
	return &Service{
		cacheTTL:       options.CacheTTL,
		commandTimeout: options.CommandTimeout,
		maxAge:         options.MaxAge,
		now:            options.Now,
		runner:         options.Runner,
		cache:          make(map[string]cacheEntry),
	}
}

func DefaultSnapshot(ctx context.Context, defaultInterface string) Result {
	return defaultService.Snapshot(ctx, defaultInterface)
}

func (s *Service) Snapshot(ctx context.Context, defaultInterface string) Result {
	interfaceName := strings.TrimSpace(defaultInterface)
	now := s.now()
	s.mu.Lock()
	entry, ok := s.cache[interfaceName]
	s.mu.Unlock()
	if ok && now.Sub(entry.fetchedAt) >= 0 && now.Sub(entry.fetchedAt) < s.cacheTTL {
		return entry.result
	}

	bounded, cancel := context.WithTimeout(ctx, s.commandTimeout)
	raw, err := s.runner(bounded, "vnstat", "--json", "d", "62")
	cancel()
	var result Result
	if err != nil {
		result = errorResult(ErrorCommandFailed)
	} else {
		result, err = ParseJSONResult(raw, ParseOptions{
			DefaultInterface:  interfaceName,
			MaxAge:            s.maxAge,
			Now:               now,
			RequireCurrentDay: true,
		})
		if err != nil {
			result = errorResult(ErrorCommandFailed)
		}
	}

	s.mu.Lock()
	s.cache[interfaceName] = cacheEntry{fetchedAt: now, result: result}
	s.mu.Unlock()
	return result
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
