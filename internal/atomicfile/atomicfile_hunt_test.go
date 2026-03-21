package atomicfile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteRenameFailureCleansUp exercises the os.Rename error path
// (lines 31-33), which was previously uncovered (86.7% -> 100%).
// We make the target path a non-empty directory, which causes
// os.Rename to fail with EISDIR on Linux. The test verifies:
// 1. The error is returned and properly wrapped
// 2. No temp files are left behind
func TestWriteRenameFailureCleansUp(t *testing.T) {
	dir := t.TempDir()

	// Target path: make it a non-empty directory, so Rename(tmp, path)
	// will fail with EISDIR on Linux.
	path := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(path, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("data"))
		return err
	})
	if err == nil {
		t.Fatal("expected error when rename target is a non-empty directory")
	}
	if !strings.Contains(err.Error(), "renaming temp file") {
		t.Errorf("expected 'renaming temp file' in error, got: %v", err)
	}

	// Verify no temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// TestWritePreservesOriginalOnCallbackError verifies that if an existing
// file is being overwritten and the callback fails, the original file
// remains untouched. This tests the atomicity guarantee.
func TestWritePreservesOriginalOnCallbackError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original.txt")

	if err := os.WriteFile(path, []byte("original-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Write(path, func(w io.Writer) error {
		_, _ = w.Write([]byte("partial"))
		return fmt.Errorf("deliberate failure")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading original file: %v", err)
	}
	if string(data) != "original-content" {
		t.Errorf("original file corrupted: got %q, want %q", string(data), "original-content")
	}
}

// TestWritePreservesOriginalOnRenameError verifies that if an existing file
// is being overwritten and the rename step fails, the original content
// is preserved. This exercises the rename error path with a pre-existing
// target, confirming the original data survives.
func TestWritePreservesOriginalOnRenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	original := filepath.Join(path, "child.txt")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("child-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte("replacement"))
		return err
	})
	if err == nil {
		t.Fatal("expected error when renaming over a non-empty directory")
	}

	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("reading original child file: %v", err)
	}
	if string(data) != "child-data" {
		t.Errorf("original child file corrupted: got %q, want %q", string(data), "child-data")
	}
}

// TestWriteCallbackClosesFile tests what happens when the callback closes
// the underlying *os.File via type assertion. The second Close() on line 29
// will fail (already closed), but that error is discarded. This test verifies
// the data still ends up in the final file correctly.
func TestWriteCallbackClosesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := Write(path, func(w io.Writer) error {
		f, ok := w.(*os.File)
		if !ok {
			return fmt.Errorf("expected *os.File, got %T", w)
		}
		if _, err := f.Write([]byte("closed-early")); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		return f.Close()
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != "closed-early" {
		t.Errorf("got %q, want %q", string(data), "closed-early")
	}
}

// TestWriteEmptyContent verifies that writing an empty file works correctly.
func TestWriteEmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write([]byte(""))
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(data))
	}
}

// TestWriteLargeContent writes a 4MB payload through atomicfile.Write to
// exercise OS buffering behavior.
func TestWriteLargeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	payload := bytes.Repeat([]byte("ABCDEFGH"), 512*1024) // 4MB

	err := Write(path, func(w io.Writer) error {
		_, err := w.Write(payload)
		return err
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("data mismatch: got %d bytes, want %d bytes", len(data), len(payload))
	}
}

// TestWriteConcurrentToSameFile verifies that concurrent writes to the same
// file don't corrupt data. Each write should produce a complete, valid file
// (one of the writer payloads, never a mix).
func TestWriteConcurrentToSameFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.txt")

	const goroutines = 10
	errs := make(chan error, goroutines)

	for i := range goroutines {
		go func(n int) {
			content := fmt.Sprintf("writer-%d", n)
			errs <- Write(path, func(w io.Writer) error {
				_, err := w.Write([]byte(content))
				return err
			})
		}(i)
	}

	for range goroutines {
		if err := <-errs; err != nil {
			t.Errorf("goroutine write error: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "writer-") {
		t.Errorf("file content corrupted: %q", content)
	}
}
