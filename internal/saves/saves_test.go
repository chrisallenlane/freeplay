package saves

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestValidType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"state", true},
		{"sram", true},
		{"", false},
		{"other", false},
		{"STATE", false},
	}

	for _, tt := range tests {
		if got := ValidType(tt.input); got != tt.want {
			t.Errorf("ValidType(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPutAndGet(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	data := []byte("save data content")
	err := m.Put("NES", "game1", "state", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got := m.Get("NES", "game1", "state")
	if got == nil {
		t.Fatal("Get returned nil for existing save")
	}
	if !bytes.Equal(got, data) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, data)
	}
}

func TestGetNonexistent(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	got := m.Get("NES", "noexist", "state")
	if got != nil {
		t.Errorf("expected nil for nonexistent save, got %d bytes", len(got))
	}
}

func TestPutCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	err := m.Put("SNES", "game2", "sram", bytes.NewReader([]byte("sram data")))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify the directory structure was created
	expected := filepath.Join(dir, "saves", "SNES", "game2", "sram")
	got := m.Get("SNES", "game2", "sram")
	if got == nil {
		t.Fatalf("save not found at expected path: %s", expected)
	}
	if string(got) != "sram data" {
		t.Errorf("got %q, want %q", string(got), "sram data")
	}
}

func TestPutOverwrites(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	m.Put("NES", "game", "state", bytes.NewReader([]byte("old")))
	m.Put("NES", "game", "state", bytes.NewReader([]byte("new")))

	got := m.Get("NES", "game", "state")
	if string(got) != "new" {
		t.Errorf("got %q, want %q", string(got), "new")
	}
}
