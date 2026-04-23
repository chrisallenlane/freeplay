package saves

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
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

	// Verify the file was actually created at the path rooted in the
	// manager's dataDir, not at a cwd-relative default. Also kills any
	// mutation that drops the dataDir field in New() or Put() in favor
	// of "" — both would still roundtrip via Get but fail this stat.
	expected := filepath.Join(dir, "saves", "SNES", "game2", "sram")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("save file not at expected path %s: %v", expected, err)
	}
	got := m.Get("SNES", "game2", "sram")
	if got == nil {
		t.Fatalf("save not found at expected path: %s", expected)
	}
	if string(got) != "sram data" {
		t.Errorf("got %q, want %q", string(got), "sram data")
	}
}

func TestPutReaderError(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	err := m.Put("NES", "game1", "state", iotest.ErrReader(iotest.ErrTimeout))
	if err == nil {
		t.Fatal("expected error from ErrReader, got nil")
	}
	if !strings.Contains(err.Error(), "reading save data") {
		t.Errorf("error should mention %q, got: %v", "reading save data", err)
	}
	// Error must wrap the underlying reader error (%w), not drop it. A
	// mutation that changes fmt.Errorf("reading save data: %w", err)
	// to fmt.Errorf("reading save data") would still match the
	// substring above but would lose errors.Is unwrapping.
	if !errors.Is(err, iotest.ErrTimeout) {
		t.Errorf("error should wrap iotest.ErrTimeout, got: %v", err)
	}
}

func TestPutOverwrites(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)

	if err := m.Put("NES", "game", "state", bytes.NewReader([]byte("old"))); err != nil {
		t.Fatal(err)
	}
	if err := m.Put("NES", "game", "state", bytes.NewReader([]byte("new"))); err != nil {
		t.Fatal(err)
	}

	got := m.Get("NES", "game", "state")
	if string(got) != "new" {
		t.Errorf("got %q, want %q", string(got), "new")
	}
}
