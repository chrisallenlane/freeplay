package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.txt")

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("hello"))
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestWriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("new"))
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("got %q, want %q", string(data), "new")
	}
}

func TestWriteErrorCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := Write(path, func(_ io.Writer) error {
		return errors.New("deliberate error")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("target file should not exist after write error")
	}

	// No temp files left behind
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("unexpected file left behind: %s", e.Name())
	}
}

func TestWriteCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.txt")

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("deep"))
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep" {
		t.Errorf("got %q, want %q", string(data), "deep")
	}
}

func TestWriteDirectoryCreationFails(t *testing.T) {
	err := Write("/dev/null/impossible/file.txt", func(w io.Writer) error {
		_, err := w.Write([]byte("data"))
		return err
	})
	if err == nil {
		t.Fatal("expected error when directory cannot be created")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("expected 'creating directory' in error, got: %v", err)
	}
}

func TestWriteReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem mode bits; test requires non-root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := Write(filepath.Join(dir, "file.txt"), func(w io.Writer) error {
		_, err := w.Write([]byte("data"))
		return err
	})
	if err == nil {
		t.Fatal("expected error when directory is read-only")
	}
	if !strings.Contains(err.Error(), "creating temp file") {
		t.Errorf("expected 'creating temp file' in error, got: %v", err)
	}
}

// TestWritePanicInCallbackCleansUp verifies that a panic in fn leaves no
// residual .tmp-* file behind. Callers may panic during streaming; the
// atomic-write flow must not leak temporary files.
func TestWritePanicInCallbackCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic to propagate")
			}
		}()
		_ = Write(path, func(_ io.Writer) error {
			panic("deliberate panic in callback")
		})
	}()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("target file should not exist after callback panic")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("unexpected file left behind after panic: %s", e.Name())
	}
}

// TestWriteDirModeHasNoWorldBits verifies that directories created by
// atomicfile are not world-accessible. Uses 0o007 (world rwx) as the
// invariant rather than asserting exact mode, to stay umask-independent.
func TestWriteDirModeHasNoWorldBits(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "created-by-atomicfile")
	path := filepath.Join(subdir, "test.txt")

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("x"))
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("stat subdir: %v", err)
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Errorf("directory is world-accessible: mode=%o", info.Mode().Perm())
	}
}
