package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
)

func TestFormatsAndLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	values := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	for _, format := range []string{"json", "text"} {
		for threshold, level := range levels {
			t.Run(format+"/"+level, func(t *testing.T) {
				var output bytes.Buffer
				log, err := New(config.Logging{Level: level, Format: format}, &output)
				if err != nil {
					t.Fatal(err)
				}
				log = log.With("component", "comments")
				for i, value := range values {
					log.Log(context.Background(), value, levels[i], "count", 3)
				}
				lines := strings.Split(strings.TrimSpace(output.String()), "\n")
				if len(lines) != len(levels)-threshold {
					t.Fatalf("unexpected record count: %s", output.String())
				}
				for i, line := range lines {
					wantLevel := strings.ToUpper(levels[threshold+i])
					if format == "json" {
						var record map[string]any
						if err := json.Unmarshal([]byte(line), &record); err != nil {
							t.Fatal(err)
						}
						if record["level"] != wantLevel || record["msg"] != levels[threshold+i] ||
							record["component"] != "comments" || record["count"] != float64(3) {
							t.Fatalf("unexpected structured record: %v", record)
						}
						stamp, ok := record["time"].(string)
						if _, err := time.Parse(time.RFC3339Nano, stamp); !ok || err != nil {
							t.Fatal("record has no valid timestamp")
						}
					} else {
						for _, field := range []string{"time=", "level=" + wantLevel, "msg=" + levels[threshold+i], "component=comments", "count=3"} {
							if !strings.Contains(line, field) {
								t.Fatalf("missing %q in text record: %s", field, line)
							}
						}
					}
				}
			})
		}
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, cfg := range []config.Logging{
		{Level: "", Format: "json"},
		{Level: "trace", Format: "json"},
		{Level: "info", Format: ""},
		{Level: "info", Format: "xml"},
	} {
		if log, err := New(cfg, io.Discard); err == nil || log != nil {
			t.Fatal("invalid configuration must not create a logger")
		}
	}
	if log, err := New(config.Logging{Level: "info", Format: "json"}, nil); err == nil || log != nil {
		t.Fatal("missing output must not create a logger")
	}
}

func TestDefaultLoggerIsUnchanged(t *testing.T) {
	before := slog.Default()
	if _, err := New(config.Logging{Level: "info", Format: "json"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if slog.Default() != before {
		t.Fatal("constructor changed the process-wide default logger")
	}
}
