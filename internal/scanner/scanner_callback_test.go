package scanner

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// TestCallbackInvokedWhileHoldingMutex verifies that the onScanComplete
// callback is invoked while s.mu is held. This means a callback that
// synchronously calls ScanBlocking or Scan will deadlock, because those
// methods try to acquire the same mutex.
//
// The production code in main.go avoids this by launching a goroutine
// from within the callback, but the Scanner API provides no protection
// against a caller that omits the goroutine.
func TestCallbackInvokedWhileHoldingMutex(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	lockHeld := make(chan bool, 1)

	s.SetOnScanComplete(func(_ []Game) {
		// TryLock returns false if the mutex is already held.
		acquired := s.mu.TryLock()
		if acquired {
			s.mu.Unlock()
			lockHeld <- false
		} else {
			lockHeld <- true
		}
	})

	s.ScanBlocking()

	select {
	case held := <-lockHeld:
		if !held {
			t.Error(
				"Expected onScanComplete to be invoked while s.mu is held, " +
					"but TryLock succeeded. The callback runs outside the lock.",
			)
		}
		// If held == true, the callback IS called with the lock held.
		// This is a design issue: synchronous Scan/ScanBlocking from the
		// callback would deadlock. Document as confirmed.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for callback result")
	}
}

// TestCallbackScanDeadlock demonstrates that calling Scan() (TryLock-based)
// from within the onScanComplete callback always fails because the mutex
// is held. The re-scan is silently skipped rather than executed.
func TestCallbackScanDeadlock(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	scanResult := make(chan bool, 1)

	callCount := 0
	s.SetOnScanComplete(func(_ []Game) {
		callCount++
		if callCount > 1 {
			return
		}
		// Scan uses TryLock and returns false if the lock is held.
		// Since the callback runs while s.mu is held, this should always
		// return false — the re-scan is silently dropped.
		scanResult <- s.Scan()
	})

	s.ScanBlocking()

	select {
	case ran := <-scanResult:
		if ran {
			t.Error(
				"Scan() returned true from within the callback, " +
					"but the mutex should be held",
			)
		}
		// ran == false confirms: a caller using Scan() (not ScanBlocking)
		// from the callback will silently lose the re-scan.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestSetOnScanCompleteDataRace verifies that concurrent scans and
// SetOnScanComplete calls are race-free. Both scan() and SetOnScanComplete
// hold s.mu while accessing s.onScanComplete, so the race detector must not
// report a DATA RACE here.
func TestSetOnScanCompleteDataRace(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	var wg sync.WaitGroup

	// Launch concurrent scans that read s.onScanComplete while holding s.mu.
	for range 5 {
		wg.Go(func() {
			s.ScanBlocking()
		})
	}

	// Concurrently write to s.onScanComplete while holding s.mu.
	for range 5 {
		wg.Go(func() {
			s.SetOnScanComplete(func(_ []Game) {})
		})
	}

	wg.Wait()
	// If the race detector reports DATA RACE here, the mutex protection
	// of s.onScanComplete has been broken.
}

// TestOnScanCompleteGoroutineRescanTerminates simulates the exact pattern
// from main.go: callback launches a goroutine that conditionally triggers
// ScanBlocking. Verifies that the re-scan loop terminates correctly.
func TestOnScanCompleteGoroutineRescanTerminates(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	var scanCount atomic.Int32
	allDone := make(chan struct{})

	s.SetOnScanComplete(func(_ []Game) {
		count := scanCount.Add(1)
		if count == 1 {
			// First scan: trigger a re-scan in a goroutine
			// (simulating FetchAll returning saved > 0)
			go func() {
				s.ScanBlocking()
				close(allDone)
			}()
		}
		// Second scan (count == 2): do not trigger re-scan
		// (simulating FetchAll returning 0)
	})

	s.ScanBlocking()

	select {
	case <-allDone:
		got := scanCount.Load()
		if got != 2 {
			t.Errorf("expected exactly 2 scans, got %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("re-scan loop did not terminate within 5 seconds")
	}
}

// TestOnScanCompleteUnboundedRescanLoop demonstrates that with no re-entry
// guard or depth limit, a callback that always triggers ScanBlocking will
// produce an unbounded chain of goroutines.
func TestOnScanCompleteUnboundedRescanLoop(t *testing.T) {
	cfg := &config.Config{
		ROMs: map[string]config.ROM{
			"NES": {Path: t.TempDir(), Core: "fceumm"},
		},
	}
	s := New(cfg, t.TempDir())

	var scanCount atomic.Int32
	const maxScans = 20

	stopped := make(chan struct{})

	s.SetOnScanComplete(func(_ []Game) {
		count := scanCount.Add(1)
		if count >= maxScans {
			// Safety valve: stop the loop in the test
			close(stopped)
			return
		}
		// Always trigger a re-scan — simulates a persistent failure
		// where FetchAll keeps returning saved > 0.
		go s.ScanBlocking()
	})

	go s.ScanBlocking()

	select {
	case <-stopped:
		got := scanCount.Load()
		if got < maxScans {
			t.Errorf("expected at least %d scans, got %d", maxScans, got)
		}
		// Reaching maxScans confirms: there is no re-entry guard.
		// A persistent FetchAll > 0 condition would cause unbounded
		// goroutine spawning.
	case <-time.After(10 * time.Second):
		t.Fatal("test timed out — possible deadlock in re-scan loop")
	}
}
