package rwmutexplus

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGoroutineIDUniqueness verifies that:
// 1. Each goroutine gets a unique ID
// 2. IDs remain consistent within the same goroutine
// 3. No ID collisions occur under high concurrency
// 4. Cleanup properly removes all IDs after use
func TestGoroutineIDUniqueness(t *testing.T) {
	const (
		numIterations = 10    // Multiple iterations to catch sporadic failures
		numGoroutines = 10000 // High goroutine count to stress test concurrency
	)

	for iter := 0; iter < numIterations; iter++ {
		t.Logf("Starting iteration %d with %d goroutines", iter, numGoroutines)

		var (
			wg         sync.WaitGroup
			idMap      sync.Map     // Thread-safe map to track all issued IDs
			duplicates atomic.Int32 // Counter for duplicate IDs
			failures   atomic.Int32 // Counter for ID consistency failures
		)

		// Reset state to ensure clean test environment
		ForceCleanup()
		runtime.GC()

		// Launch goroutines to simulate heavy concurrent usage
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(routineNum int) {
				defer func() {
					ReleaseGoroutineID() // Ensure proper cleanup
					wg.Done()
				}()

				// Get ID twice to verify consistency
				id1 := GetGoroutineID()
				time.Sleep(time.Microsecond) // Force potential race conditions
				id2 := GetGoroutineID()

				// Verify ID remains consistent within goroutine
				if id1 != id2 {
					failures.Add(1)
					t.Errorf("ID changed within same goroutine: %d -> %d", id1, id2)
					return
				}

				// Check for duplicate IDs across goroutines
				if _, loaded := idMap.LoadOrStore(id1, routineNum); loaded {
					duplicates.Add(1)
					t.Errorf("Duplicate ID detected: %d", id1)
				}
			}(i)
		}

		wg.Wait()

		// Allow time for final cleanup operations
		time.Sleep(10 * time.Millisecond)

		// Report any uniqueness violations
		if dups := duplicates.Load(); dups > 0 {
			t.Errorf("Iteration %d: Found %d duplicate IDs", iter, dups)
		}
		if fails := failures.Load(); fails > 0 {
			t.Errorf("Iteration %d: Found %d ID consistency failures", iter, fails)
		}

		// Multiple cleanup passes to ensure thorough cleanup
		// This helps catch edge cases in cleanup logic
		for i := 0; i < 3; i++ {
			CleanupGoroutineIDs()
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}

		// Final cleanup and verification
		ForceCleanup()
		runtime.GC()

		// Verify no IDs remain after cleanup
		remaining := GetStoredGoroutineCount()
		if remaining > 0 {
			t.Logf("Debug: Active IDs still present: %d", remaining)
			t.Logf("Debug: In-use count: %d", GetStoredGoroutineCount())
			t.Errorf("Iteration %d: %d IDs remained after cleanup", iter, remaining)
		}

		// Verify in-use tracking is cleared
		if GetStoredGoroutineCount() > 0 {
			t.Errorf("Iteration %d: %d IDs still marked as in-use", iter, GetStoredGoroutineCount())
		}
	}
}

// TestGoroutineIDStress performs intensive stress testing by:
// 1. Running many concurrent goroutines
// 2. Continuously checking ID consistency
// 3. Verifying no ID collisions under heavy load
// 4. Ensuring proper cleanup after high volume usage
func TestGoroutineIDStress(t *testing.T) {
	const (
		numWorkers    = 10000           // High worker count for stress testing
		duration      = 3 * time.Second // Run long enough to catch timing issues
		checkInterval = 100 * time.Millisecond
	)

	var (
		wg           sync.WaitGroup
		idsGenerated atomic.Int64
		idCollisions atomic.Int64
		stopChan     = make(chan struct{})
		idMap        sync.Map
	)

	// Launch worker goroutines to continuously verify ID consistency
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()

			myID := GetGoroutineID()

			// Check for initial ID collisions
			if _, loaded := idMap.LoadOrStore(myID, workerNum); loaded {
				idCollisions.Add(1)
				t.Errorf("Initial ID collision detected: %d", myID)
			}

			idsGenerated.Add(1)

			// Continuously verify ID consistency until stopped
			for {
				select {
				case <-stopChan:
					return
				default:
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

	// Monitor and log statistics during the test
	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-time.After(checkInterval):
				t.Logf("Current stats - Generated: %d, Collisions: %d, Stored: %d",
					idsGenerated.Load(),
					idCollisions.Load(),
					GetStoredGoroutineCount())
			}
		}
	}()

	// Run test for specified duration
	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	// Gather final statistics
	finalGenerated := idsGenerated.Load()
	finalCollisions := idCollisions.Load()
	storedCount := GetStoredGoroutineCount()

	t.Logf("Final results - Generated: %d, Collisions: %d, Stored: %d",
		finalGenerated, finalCollisions, storedCount)

	// Verify no collisions occurred
	if finalCollisions > 0 {
		t.Errorf("Detected %d ID collisions during stress test", finalCollisions)
	}

	// Verify ID count matches worker count
	if storedCount > numWorkers {
		t.Errorf("More IDs stored (%d) than workers (%d)", storedCount, numWorkers)
	}

	// Thorough cleanup verification
	for i := 0; i < 3; i++ {
		CleanupGoroutineIDs()
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	ForceCleanup()
	runtime.GC()

	// Final cleanup verification
	remaining := GetStoredGoroutineCount()
	if remaining > 0 {
		t.Logf("Debug: Active IDs still present: %d", remaining)
		t.Logf("Debug: In-use count: %d", GetStoredGoroutineCount())
		t.Errorf("Final cleanup: %d IDs remained", remaining)
	}

	if GetStoredGoroutineCount() > 0 {
		t.Errorf("Final cleanup: %d IDs still marked as in-use", GetStoredGoroutineCount())
	}
}

// TestGoroutineIDMemoryLeak verifies memory stability by:
// 1. Establishing a baseline memory usage
// 2. Running multiple iterations of high-concurrency operations
// 3. Monitoring memory growth patterns
// 4. Ensuring cleanup effectively prevents memory leaks
func TestGoroutineIDMemoryLeak(t *testing.T) {
	const (
		numIterations      = 50    // Many iterations to detect memory growth
		numGoroutines      = 50000 // Large number to stress memory usage
		baselineIterations = 3     // Iterations to establish stable baseline
		memoryGrowthLimit  = 1.27  // 27% growth allowance - determined empirically
		// Lower numbers need less memory but may be less stable
	)

	// Helper to calculate percentage growth for clear reporting
	calcGrowthPct := func(current, baseline uint64) float64 {
		return float64(10000*current)/float64(baseline)/100 - 100
	}

	// Warm up phase to establish reliable baseline
	// This eliminates initial runtime allocations from affecting results
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

	// Establish baseline memory usage after warmup
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	baselineAlloc = memStats.Alloc

	// Calculate acceptable memory limits
	memoryLimit := uint64(float64(baselineAlloc) * memoryGrowthLimit)
	allowedGrowthPct := float64(10000*(memoryGrowthLimit-1)) / 100

	t.Logf("Memory monitoring setup:")
	t.Logf("  Baseline allocation: %d bytes", baselineAlloc)
	t.Logf("  Memory limit: %d bytes (%.2f%% growth allowed)", memoryLimit, allowedGrowthPct)

	// Run test iterations and monitor memory usage
	var maxAlloc uint64
	for iter := 0; iter < numIterations; iter++ {
		var wg sync.WaitGroup

		// Create goroutines and verify ID consistency
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

		// Check against calculated limit with detailed percentage reporting
		if memStats.Alloc > memoryLimit {
			t.Errorf("Memory usage exceeded limit - Baseline: %d, Current: %d, Limit: %d (want %.2f%% < current %.2f%%)",
				baselineAlloc, memStats.Alloc, memoryLimit, allowedGrowthPct, currentGrowthPct)
		}

		ForceCleanup()
	}

	// Final cleanup and reporting
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

	// Final verification with percentage-based reporting
	if finalAlloc > memoryLimit {
		t.Errorf("Final memory exceeds limit - Baseline: %d, Final: %d, Limit: %d (want %.2f%% < current %.2f%%)",
			baselineAlloc, finalAlloc, memoryLimit, allowedGrowthPct, finalGrowthPct)
	}
}

// TestGoroutineIDConcurrentCleanup verifies that:
// 1. IDs remain consistent during concurrent cleanup operations
// 2. Cleanup doesn't interfere with active goroutines
// 3. No ID changes occur while goroutines are running
func TestGoroutineIDConcurrentCleanup(t *testing.T) {
	const (
		numGoroutines   = 1000                   // Enough goroutines to create cleanup pressure
		cleanupInterval = 100 * time.Millisecond // Frequent cleanup to stress concurrent access
	)

	var (
		wg       sync.WaitGroup
		stopChan = make(chan struct{})
		errors   atomic.Int32
	)

	// Start aggressive cleanup goroutine
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

	// Launch test goroutines that continuously verify their IDs
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Repeatedly verify ID consistency while cleanup is happening
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

// BenchmarkGoroutineID provides performance metrics for:
// 1. Sequential ID retrieval (single goroutine performance)
// 2. Parallel ID retrieval (concurrent access performance)
func BenchmarkGoroutineID(b *testing.B) {
	// Benchmark sequential performance
	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = GetGoroutineID()
		}
	})

	// Benchmark concurrent performance
	b.Run("Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = GetGoroutineID()
			}
		})
	})
}

// TestGoroutineIDWithRace runs core tests with race detector
// Only runs full tests when not in short mode to avoid long CI times
func TestGoroutineIDWithRace(t *testing.T) {
	if !testing.Short() {
		TestGoroutineIDUniqueness(t)
		TestGoroutineIDStress(t)
	}
}
