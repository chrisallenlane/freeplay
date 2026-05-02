package main

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel slog.Level
		wantOK    bool
	}{
		{"empty defaults to info", "", slog.LevelInfo, true},
		{"debug", "debug", slog.LevelDebug, true},
		{"info explicit", "info", slog.LevelInfo, true},
		{"warn", "warn", slog.LevelWarn, true},
		{"error", "error", slog.LevelError, true},
		{"INFO uppercase", "INFO", slog.LevelInfo, true},
		{"DEBUG uppercase", "DEBUG", slog.LevelDebug, true},
		{"WARN uppercase", "WARN", slog.LevelWarn, true},
		{"ERROR uppercase", "ERROR", slog.LevelError, true},
		{"mixed case Info", "Info", slog.LevelInfo, true},
		{"garbage falls back to info", "garbage", slog.LevelInfo, false},
		{"numeric garbage", "42", slog.LevelInfo, false},
		{"partial match", "deb", slog.LevelInfo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLevel, gotOK := parseLogLevel(tt.input)
			if gotLevel != tt.wantLevel {
				t.Errorf(
					"parseLogLevel(%q) level = %v, want %v",
					tt.input, gotLevel, tt.wantLevel,
				)
			}
			if gotOK != tt.wantOK {
				t.Errorf(
					"parseLogLevel(%q) ok = %v, want %v",
					tt.input, gotOK, tt.wantOK,
				)
			}
		})
	}
}
