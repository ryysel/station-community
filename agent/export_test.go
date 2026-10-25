package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDefaultsDoNotIncludeLocationOrWeather(t *testing.T) {
	t.Setenv("STATION_AGENT_API_KEY", "unit-test-placeholder")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"chat_model":"test-model"}`), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := loadBrain(path, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if b.cfg.City != "" || b.weather != nil || b.cfg.Latitude != 0 || b.cfg.Longitude != 0 {
		t.Fatal("export must not silently enable a location/weather request")
	}
	if b.cfg.APIKey != "unit-test-placeholder" {
		t.Fatal("environment key not loaded")
	}
}

func TestExportLogsRedactContentByDefault(t *testing.T) {
	t.Setenv("STATION_AGENT_LOG_CONTENT", "")
	if got := trimForLog("synthetic private speech"); got != "[redacted]" {
		t.Fatal("speech logging was not redacted")
	}
	t.Setenv("STATION_AGENT_LOG_CONTENT", "1")
	if got := trimForLog(strings.Repeat("x", 170)); len([]rune(got)) != 161 {
		t.Fatal("opt-in diagnostic content must still be truncated")
	}
}
