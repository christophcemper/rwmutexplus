package rwmutexplus

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRWMutexPlus(t *testing.T) {
	name := "test-mutex"
	timeout := 100 * time.Millisecond
	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)

	mutex := NewRWMutexPlus(name, timeout, logger)

	if mutex.name != name {
		t.Errorf("Expected name %v, got %v", name, mutex.name)
	}
	if mutex.warningTimeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, mutex.warningTimeout)
	}
	if mutex.logger != logger {
		t.Errorf("Expected logger to be set")
	}
}

func TestLockWithPurpose(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, logger)

	// Test normal lock acquisition
	mutex.LockWithPurpose("write-test")
	if len(mutex.activeWrites) != 1 {
		t.Error("Expected one active write lock")
	}
	mutex.Unlock()

	// Test lock contention warning
	var wg sync.WaitGroup
	wg.Add(2)

	// First goroutine holds the lock
	go func() {
		defer wg.Done()
		mutex.LockWithPurpose("long-write")
		time.Sleep(100 * time.Millisecond)
		mutex.Unlock()
	}()

	// Give time for first lock to be acquired
	time.Sleep(10 * time.Millisecond)

	// Second goroutine attempts to acquire lock
	go func() {
		defer wg.Done()
		mutex.LockWithPurpose("waiting-write")
		mutex.Unlock()
	}()

	wg.Wait()

	// Verify warning was logged
	if !strings.Contains(buf.String(), "WARNING: Write lock") {
		t.Error("Expected warning log for delayed lock acquisition")
	}
}

func TestRLockWithPurpose(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, logger)

	// Test concurrent read locks
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mutex.RLockWithPurpose("concurrent-read")
			time.Sleep(10 * time.Millisecond)
			mutex.RUnlock()
		}()
	}
	wg.Wait()

	// Test read lock contention
	wg.Add(2)
	go func() {
		defer wg.Done()
		mutex.LockWithPurpose("blocking-write")
		time.Sleep(100 * time.Millisecond)
		mutex.Unlock()
	}()

	time.Sleep(10 * time.Millisecond)

	go func() {
		defer wg.Done()
		mutex.RLockWithPurpose("waiting-read")
		mutex.RUnlock()
	}()

	wg.Wait()

	if !strings.Contains(buf.String(), "WARNING: Read lock") {
		t.Error("Expected warning log for delayed read lock acquisition")
	}
}

func TestUnlockPanic(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on unlock without lock")
		}
	}()

	mutex.Unlock()
}

func TestRUnlockPanic(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on RUnlock without RLock")
		}
	}()

	mutex.RUnlock()
}

func TestGetPurposeStats(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)

	// Generate some lock activity
	for i := 0; i < 3; i++ {
		mutex.LockWithPurpose("write-op")
		time.Sleep(10 * time.Millisecond)
		mutex.Unlock()
	}

	stats := mutex.GetPurposeStats()
	writeStats := stats["write-op"]

	if writeStats["total_acquired"] != 3 {
		t.Errorf("Expected 3 total_acquired, got %d", writeStats["total_acquired"])
	}
	if writeStats["total_time_held"] == 0 {
		t.Error("Expected non-zero total_time_held")
	}
}

func TestGetActiveLocksInfo(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		mutex.LockWithPurpose("active-write")
		wg.Done()
		time.Sleep(50 * time.Millisecond)
		mutex.Unlock()
	}()

	wg.Wait()
	info := mutex.GetActiveLocksInfo()

	if len(info["write_locks"]) != 1 {
		t.Error("Expected one active write lock")
	}
	if info["write_locks"][0]["purpose"] != "active-write" {
		t.Error("Expected write lock purpose to be 'active-write'")
	}
}

func TestGetPendingLocksInfo(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)

	// Create lock contention
	mutex.LockWithPurpose("blocking-write")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		mutex.LockWithPurpose("pending-write")
		mutex.Unlock()
	}()

	// Give time for the goroutine to register as pending
	time.Sleep(10 * time.Millisecond)

	info := mutex.GetPendingLocksInfo()

	if len(info["pending_writes"]) != 1 {
		t.Error("Expected one pending write lock")
	}

	mutex.Unlock()
	wg.Wait()
}

func TestLockMonitoring(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, logger)

	mutex.LockWithPurpose("monitored-write")
	time.Sleep(100 * time.Millisecond)
	mutex.Unlock()

	if !strings.Contains(buf.String(), "WARNING: Write lock for purpose 'monitored-write' held for") {
		t.Error("Expected warning log for long-held lock")
	}
}

func TestConcurrentReadersAndWriters(t *testing.T) {
	mutex := NewRWMutexPlus("test", 50*time.Millisecond, nil)
	var wg sync.WaitGroup

	// Launch multiple readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mutex.RLockWithPurpose("concurrent-read")
			time.Sleep(10 * time.Millisecond)
			mutex.RUnlock()
		}()
	}

	// Launch competing writers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mutex.LockWithPurpose("concurrent-write")
			time.Sleep(10 * time.Millisecond)
			mutex.Unlock()
		}()
	}

	wg.Wait()
}

// Benchmark lock operations
func BenchmarkRWMutexPlus(b *testing.B) {
	mutex := NewRWMutexPlus("bench", 1*time.Second, nil)

	b.Run("Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mutex.LockWithPurpose("bench-write")
			mutex.Unlock()
		}
	})

	b.Run("Read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mutex.RLockWithPurpose("bench-read")
			mutex.RUnlock()
		}
	})
}
