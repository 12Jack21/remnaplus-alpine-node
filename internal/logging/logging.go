// Package logging formats native service diagnostics as bounded UTC JSON records.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	ansi        = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	privateKey  = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	urls        = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
	bearer      = regexp.MustCompile(`(?i)\bBearer\s+\S+`)
	jwt         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	credentials = regexp.MustCompile(`(?i)\b(password|secret|token|authorization|cookie|api[-_]?key|credential|private[-_]?key)\b["']?\s*[:=]\s*(?:"[^"\n]*"|'[^'\n]*'|[^\s,;\]}]+)`)
)

func Sanitize(value string) string {
	if start := strings.IndexAny(value, "[{"); start >= 0 {
		var payload any
		if json.Unmarshal([]byte(value[start:]), &payload) == nil {
			switch payload.(type) {
			case map[string]any, []any:
				value = value[:start] + "[structured payload omitted]"
			}
		}
	}
	value = ansi.ReplaceAllString(value, "")
	value = privateKey.ReplaceAllString(value, "[REDACTED KEY]")
	value = urls.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[REDACTED URL]"
		}
		return parsed.Scheme + "://" + parsed.Host + "/[REDACTED]"
	})
	value = bearer.ReplaceAllString(value, "Bearer [REDACTED]")
	value = jwt.ReplaceAllString(value, "[REDACTED TOKEN]")
	value = credentials.ReplaceAllString(value, "${1}=<redacted>")
	if len(value) > 8192 {
		value = value[:8192]
	}
	return value
}

type writer struct{ logger *slog.Logger }

func (w writer) Write(p []byte) (int, error) {
	message := strings.TrimSuffix(string(p), "\n")
	lower := strings.ToLower(message)
	level := slog.LevelInfo
	if strings.HasPrefix(lower, "warning:") {
		level = slog.LevelWarn
	}
	if strings.Contains(lower, "failed") || strings.Contains(lower, "error:") || strings.HasPrefix(lower, "fatal:") {
		level = slog.LevelError
	}
	w.logger.Log(context.Background(), level, Sanitize(message))
	return len(p), nil
}

func Configure(output io.Writer) {
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			attr.Value = slog.TimeValue(attr.Value.Time().UTC())
		}
		if attr.Value.Kind() == slog.KindString {
			attr.Value = slog.StringValue(Sanitize(attr.Value.String()))
		}
		return attr
	}})
	log.SetFlags(0)
	log.SetOutput(writer{logger: slog.New(handler).With("service", "remnanode-native")})
}

// Fatal preserves the existing startup exit semantics while identifying the severity explicitly.
func Fatal(format string, args ...any) {
	log.Print("fatal: " + fmt.Sprintf(format, args...))
	os.Exit(1)
}
