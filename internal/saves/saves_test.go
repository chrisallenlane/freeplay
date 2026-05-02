package saves

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

	got, err := m.Get("NES", "game1", "state")
	if err != nil {
		t.Fatalf("Get returned error for existing save: %v", err)
	}
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

	got, err := m.Get("NES", "noexist", "state")
	if err != nil {
		t.Errorf("ENOENT path returned error %v, want nil error", err)
	}
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
	got, err := m.Get("SNES", "game2", "sram")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
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
	if !errors.Is(err, ErrReadSaveData) {
		t.Errorf("expected ErrReadSaveData, got: %v", err)
	}
	// Error must also wrap the underlying reader error so callers can
	// distinguish the cause (network vs. timeout vs. malformed body).
	// A mutation that drops %w would break this check.
	if !errors.Is(err, iotest.ErrTimeout) {
		t.Errorf("error should wrap iotest.ErrTimeout, got: %v", err)
	}
}

// TestGetUnreadableFile pins the contract that Manager.Get returns a
// non-nil error for an existing-but-unreadable save file (permission
// denied, stale mount, underlying IO error). Returning (nil, nil) here
// would be indistinguishable from "save does not exist" — the failure
// mode the prior version of this code admitted, where the next
// auto-save tick silently overwrote the real save.
func TestGetUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix file permissions; skipping")
	}
	dir := t.TempDir()
	m := New(dir)

	// Write a real save and then make it unreadable.
	if err := m.Put("NES", "game", "sram", bytes.NewReader([]byte("real save data"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	path := filepath.Join(dir, "saves", "NES", "game", "sram")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	// Sanity-check: os.ReadFile must actually fail under this chmod.
	// If it doesn't (root, or some unusual filesystem), this test cannot
	// say anything meaningful.
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("filesystem does not enforce read permissions; skipping")
	}

	got, err := m.Get("NES", "game", "sram")
	if err == nil {
		t.Errorf(
			"Get returned nil error for an existing but unreadable save " +
				"file; caller cannot distinguish 'missing' from 'unreadable'",
		)
	}
	if got != nil {
		t.Errorf("Get returned %d bytes for unreadable file; want nil data on error", len(got))
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

	got, err := m.Get("NES", "game", "state")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("got %q, want %q", string(got), "new")
	}
}
