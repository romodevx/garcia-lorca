package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/garcia-lorca/pkg/logger"
)

func TestNewJSONEmitsStructuredOutput(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Options{Format: logger.FormatJSON, Level: slog.LevelInfo, Writer: &buf})

	log.Info("module enabled", "module", "iam")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, buf.String())
	}
	if entry["msg"] != "module enabled" || entry["module"] != "iam" {
		t.Fatalf("unexpected log entry: %v", entry)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Options{Format: logger.FormatText, Level: slog.LevelWarn, Writer: &buf})

	log.Info("should be filtered")
	log.Warn("should appear")

	out := buf.String()
	if strings.Contains(out, "should be filtered") {
		t.Fatalf("info line leaked past Warn level: %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Fatalf("warn line missing: %q", out)
	}
}
