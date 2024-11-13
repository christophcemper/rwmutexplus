package rwmutexplus

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGoroutineIDUniqueness verifies IDs are unique across goroutines
func TestGoroutineIDUniqueness(t *testing.T) {
	const (
		numIterations = 10
		numGoroutines = 10000
	)

	for iter := 0; iter < numIterations; iter++ {
		t.Logf("Starting iteration %d with %d goroutines", iter, numGoroutines)

		var (
			wg         sync.WaitGroup
			idMap      sync.Map
			duplicates atomic.Int32
			failures   atomic.Int32
		)

		// Reset state before each iteration
		ForceCleanup()
		runtime.GC()

		// Launch goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(routineNum int) {
				defer func() {
					ReleaseGoroutineID()
					wg.Done()
				}()

				id1 := GetGoroutineID()
				time.Sleep(time.Microsecond)
				id2 := GetGoroutineID()

				if id1 != id2 {
					failures.Add(1)
					t.Errorf("ID changed within same goroutine: %d -> %d", id1, id2)
					return
				}

				if _, loaded := idMap.LoadOrStore(id1, routineNum); loaded {
					duplicates.Add(1)
					t.Errorf("Duplicate ID detected: %d", id1)
				}
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()

		// Ensure all IDs are released
		time.Sleep(10 * time.Millisecond)

		if dups := duplicates.Load(); dups > 0 {
			t.Errorf("Iteration %d: Found %d duplicate IDs", iter, dups)
		}
		if fails := failures.Load(); fails > 0 {
			t.Errorf("Iteration %d: Found %d ID consistency failures", iter, fails)
		}

		// Multiple cleanup passes to ensure complete cleanup
		for i := 0; i < 3; i++ {
			CleanupGoroutineIDs()
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}

		// Final cleanup
		ForceCleanup()
		runtime.GC()

		// Verify cleanup
		remaining := GetStoredGoroutineCount()
		if remaining > 0 {
			// Dump debug info
			t.Logf("Debug: Active IDs still present: %d", remaining)
			t.Logf("Debug: In-use count: %d", GetInUseCount())
			t.Errorf("Iteration %d: %d IDs remained after cleanup", iter, remaining)
		}

		// Extra verification
		if GetInUseCount() > 0 {
			t.Errorf("Iteration %d: %d IDs still marked as in-use", iter, GetInUseCount())
		}
	}
}

// Add this helper function to the main implementation
func GetInUseCount() int {
	idStore.RLock()
	defer idStore.RUnlock()
	return len(idStore.inUse)
}

// TestGoroutineIDStress performs stress testing with rapid creation/deletion
func TestGoroutineIDStress(t *testing.T) {
	const (
		numWorkers    = 10000
		duration      = 3 * time.Second
		checkInterval = 100 * time.Millisecond
	)

	var (
		wg           sync.WaitGroup
		idsGenerated atomic.Int64
		idCollisions atomic.Int64
		stopChan     = make(chan struct{})
		idMap        sync.Map
	)

	// Launch worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()

			// Get initial ID for this goroutine
			myID := GetGoroutineID()

			// Store initial ID
			if _, loaded := idMap.LoadOrStore(myID, workerNum); loaded {
				idCollisions.Add(1)
				t.Errorf("Initial ID collision detected: %d", myID)
			}

			idsGenerated.Add(1)

			for {
				select {
				case <-stopChan:
					return
				default:
					// Verify ID stays consistent
					currentID := GetGoroutineID()
					if currentID != myID {
						idCollisions.Add(1)
						t.Errorf("ID changed within same goroutine: %d -> %d", myID, currentID)
					}

					idsGenerated.Add(1)
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Monitor goroutines periodically
	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-time.After(checkInterval):
				// Log current statistics
				t.Logf("Current stats - Generated: %d, Collisions: %d, Stored: %d",
					idsGenerated.Load(),
					idCollisions.Load(),
					GetStoredGoroutineCount())
			}
		}
	}()

	// Run for specified duration
	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	finalGenerated := idsGenerated.Load()
	finalCollisions := idCollisions.Load()
	storedCount := GetStoredGoroutineCount()

	t.Logf("Final results - Generated: %d, Collisions: %d, Stored: %d",
		finalGenerated, finalCollisions, storedCount)

	if finalCollisions > 0 {
		t.Errorf("Detected %d ID collisions during stress test", finalCollisions)
	}

	if storedCount > numWorkers {
		t.Errorf("More IDs stored (%d) than workers (%d)", storedCount, numWorkers)
	}

	// Multiple cleanup passes to ensure complete cleanup
	for i := 0; i < 3; i++ {
		CleanupGoroutineIDs()
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Final cleanup
	ForceCleanup()
	runtime.GC()

	// Verify cleanup
	remaining := GetStoredGoroutineCount()
	if remaining > 0 {
		// Dump debug info
		t.Logf("Debug: Active IDs still present: %d", remaining)
		t.Logf("Debug: In-use count: %d", GetInUseCount())
		t.Errorf("Iteration %d: %d IDs remained after cleanup", 1, remaining)
	}

	// Extra verification
	if GetInUseCount() > 0 {
		t.Errorf("Iteration %d: %d IDs still marked as in-use", 1, GetInUseCount())
	}
}

// TestGoroutineIDMemoryLeak verifies no memory leaks occur
func TestGoroutineIDMemoryLeak(t *testing.T) {
	const (
		numIterations      = 50
		numGoroutines      = 50000
		baselineIterations = 3
		memoryGrowthLimit  = 1.27 // 27% growth allowance for this setup, lower numbers need less memory
	)

	// Helper function to calculate percentage growth
	calcGrowthPct := func(current, baseline uint64) float64 {
		return float64(10000*current)/float64(baseline)/100 - 100
	}

	// Warm up the system and establish baseline
	var baselineAlloc uint64
	for i := 0; i < baselineIterations; i++ {
		runtime.GC()
		ForceCleanup()
		runtime.GC()

		var wg sync.WaitGroup
		for j := 0; j < numGoroutines; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = GetGoroutineID()
			}()
		}
		wg.Wait()
		ForceCleanup()
	}

	// Get baseline memory after warmup
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineAlloc = memStats.Alloc

	// Calculate our memory limit and allowed growth percentage
	memoryLimit := uint64(float64(baselineAlloc) * memoryGrowthLimit)
	allowedGrowthPct := float64(10000*(memoryGrowthLimit-1)) / 100

	t.Logf("Memory monitoring setup:")
	t.Logf("  Baseline allocation: %d bytes", baselineAlloc)
	t.Logf("  Memory limit: %d bytes (%.2f%% growth allowed)", memoryLimit, allowedGrowthPct)

	// Run actual test iterations
	var maxAlloc uint64
	for iter := 0; iter < numIterations; iter++ {
		var wg sync.WaitGroup

		// Create goroutines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id1 := GetGoroutineID()
				time.Sleep(time.Microsecond)
				id2 := GetGoroutineID()
				if id1 != id2 {
					t.Errorf("ID mismatch: %d != %d", id1, id2)
				}
			}()
		}

		wg.Wait()
		runtime.GC()
		runtime.ReadMemStats(&memStats)

		currentGrowthPct := calcGrowthPct(memStats.Alloc, baselineAlloc)

		t.Logf("Iteration %d:", iter)
		t.Logf("  Current memory: %d bytes (%.2f%% growth)", memStats.Alloc, currentGrowthPct)
		t.Logf("  Stored IDs: %d", GetStoredGoroutineCount())

		if memStats.Alloc > maxAlloc {
			maxAlloc = memStats.Alloc
		}

		// Check against our calculated limit with percentage reporting
		if memStats.Alloc > memoryLimit {
			t.Errorf("Memory usage exceeded limit - Baseline: %d, Current: %d, Limit: %d (want %.2f%% < current %.2f%%)",
				baselineAlloc, memStats.Alloc, memoryLimit, allowedGrowthPct, currentGrowthPct)
		}

		ForceCleanup()
	}

	// Final cleanup and check
	runtime.GC()
	ForceCleanup()
	runtime.GC()

	runtime.ReadMemStats(&memStats)
	finalAlloc := memStats.Alloc
	finalGrowthPct := calcGrowthPct(finalAlloc, baselineAlloc)
	maxGrowthPct := calcGrowthPct(maxAlloc, baselineAlloc)

	t.Logf("Final memory stats:")
	t.Logf("  Baseline: %d bytes", baselineAlloc)
	t.Logf("  Peak: %d bytes (%.2f%% growth)", maxAlloc, maxGrowthPct)
	t.Logf("  Final: %d bytes (%.2f%% growth)", finalAlloc, finalGrowthPct)
	t.Logf("  Allowed growth: %.2f%%", allowedGrowthPct)
	t.Logf("  Remaining IDs: %d", GetStoredGoroutineCount())

	// Final memory check with percentage reporting
	if finalAlloc > memoryLimit {
		t.Errorf("Final memory exceeds limit - Baseline: %d, Final: %d, Limit: %d (want %.2f%% < current %.2f%%)",
			baselineAlloc, finalAlloc, memoryLimit, allowedGrowthPct, finalGrowthPct)
	}
}

// TestGoroutineIDConcurrentCleanup verifies cleanup safety
func TestGoroutineIDConcurrentCleanup(t *testing.T) {
	const (
		numGoroutines   = 1000
		cleanupInterval = 100 * time.Millisecond
	)

	var (
		wg       sync.WaitGroup
		stopChan = make(chan struct{})
		errors   atomic.Int32
	)

	// Start cleanup goroutine
	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-time.After(cleanupInterval):
				CleanupGoroutineIDs()
			}
		}
	}()

	// Launch test goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Repeatedly get and verify IDs while cleanup is happening
			prevID := GetGoroutineID()
			for j := 0; j < 100; j++ {
				currentID := GetGoroutineID()
				if currentID != prevID {
					errors.Add(1)
					t.Errorf("ID changed during concurrent cleanup: %d -> %d",
						prevID, currentID)
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	close(stopChan)

	if errorCount := errors.Load(); errorCount > 0 {
		t.Errorf("Detected %d errors during concurrent cleanup", errorCount)
	}
}

// BenchmarkGoroutineID measures performance
func BenchmarkGoroutineID(b *testing.B) {
	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetGoroutineID()
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = GetGoroutineID()
			}
		})
	})
}

// Helper function to run with -race detector
func TestGoroutineIDWithRace(t *testing.T) {
	if !testing.Short() {
		TestGoroutineIDUniqueness(t)
		TestGoroutineIDStress(t)
	}
}
