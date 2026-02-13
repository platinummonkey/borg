package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetup_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := Setup("info", "text", &buf)

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected 'test message' in output, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected 'key=value' in output, got: %s", output)
	}
}

func TestSetup_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := Setup("info", "json", &buf)

	logger.Info("test json")

	output := buf.String()
	if !strings.Contains(output, `"msg":"test json"`) {
		t.Errorf("expected JSON msg field in output, got: %s", output)
	}
}

func TestSetup_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := Setup("debug", "text", &buf)

	logger.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("expected debug message in output, got: %s", output)
	}
}

func TestSetup_InfoLevelFiltersDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := Setup("info", "text", &buf)

	logger.Debug("should not appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Errorf("debug message should not appear at info level, got: %s", output)
	}
}
