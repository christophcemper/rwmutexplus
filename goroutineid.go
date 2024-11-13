package rwmutexplus

import (
	"bytes"
	"hash/fnv"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	// Global counter ensures unique IDs even if hash collisions occur
	goroutineCounter atomic.Uint64

	// Pool of reusable buffers to minimize allocations during stack trace capture
	// Uses pointer to slice to prevent copying and reduce allocations
	bufferPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 64)
			return &b // Return pointer to prevent slice copying
		},
	}

	// Thread-safe storage for goroutine IDs with separate tracking of active IDs
	// Split design allows quick release without full cleanup
	idStore struct {
		sync.RWMutex
		ids   map[[8]byte]uint64   // Maps hash of goroutine stack to unique ID
		inUse map[[8]byte]struct{} // Tracks which IDs are currently active
	}
)

func init() {
	// Pre-allocate maps with reasonable capacity to reduce resizing
	// 1024 is chosen as a balance between memory usage and resize frequency
	idStore.ids = make(map[[8]byte]uint64, 1024)
	idStore.inUse = make(map[[8]byte]struct{}, 1024)
}

// GetGoroutineID returns a unique identifier for the current goroutine
// Uses a two-phase locking strategy to optimize for the common case
// where the ID already exists
func GetGoroutineID() uint64 {
	key := getGoRoutineKey()

	// Fast path: Check if ID exists with read lock
	// This optimizes for the common case where goroutines
	// request their ID multiple times
	idStore.RLock()
	if id, exists := idStore.ids[key]; exists {
		// Need to mark as in-use, but must upgrade to write lock
		idStore.RUnlock()
		idStore.Lock()
		idStore.inUse[key] = struct{}{}
		idStore.Unlock()
		return id
	}
	idStore.RUnlock()

	// Slow path: Create new ID with write lock
	// Full lock needed for creating new IDs to maintain consistency
	idStore.Lock()
	defer idStore.Unlock()

	// Double-check after acquiring write lock to handle race conditions
	if id, exists := idStore.ids[key]; exists {
		idStore.inUse[key] = struct{}{}
		return id
	}

	// Create new unique ID and store it
	id := goroutineCounter.Add(1)
	idStore.ids[key] = id
	idStore.inUse[key] = struct{}{}
	return id
}

// ReleaseGoroutineID marks an ID as no longer in active use
// This allows the cleanup routine to eventually remove it
func ReleaseGoroutineID() {
	key := getGoRoutineKey()
	idStore.Lock()
	delete(idStore.inUse, key)
	idStore.Unlock()
}

// getGoRoutineKey generates a fixed-size hash key from the goroutine's stack trace
// Uses buffer pooling to minimize allocations during frequent calls
func getGoRoutineKey() [8]byte {
	// Get buffer from pool and ensure it's returned
	buf := *(bufferPool.Get().(*[]byte))
	defer bufferPool.Put(&buf)

	// Capture minimal stack trace (just first line contains goroutine ID)
	n := runtime.Stack(buf, false)
	h := fnv.New64a()

	// Hash only the first line which contains the goroutine ID
	if idx := bytes.IndexByte(buf[:n], '\n'); idx > 0 {
		h.Write(buf[:idx])
	} else {
		h.Write(buf[:n])
	}

	// Convert hash to fixed-size key
	var key [8]byte
	copy(key[:], h.Sum(nil))
	return key
}

// CleanupGoroutineIDs safely removes unused IDs while preserving active ones
// Uses a complete map replacement strategy to prevent memory leaks
func CleanupGoroutineIDs() {
	// Large buffer for complete stack dump of all goroutines
	buf := make([]byte, 102400)
	n := runtime.Stack(buf, true)
	currentStacks := buf[:n]

	idStore.Lock()
	defer idStore.Unlock()

	// Create new maps to prevent memory leaks from the old ones
	// Fixed capacity helps prevent map growth
	newIDs := make(map[[8]byte]uint64, 1024)
	newInUse := make(map[[8]byte]struct{}, 1024)

	// Only keep IDs that are both marked as in-use AND have a living goroutine
	for key, id := range idStore.ids {
		if _, inUse := idStore.inUse[key]; inUse {
			// Verify goroutine still exists
			keyBuf := *(bufferPool.Get().(*[]byte))
			n := runtime.Stack(keyBuf, false)
			if bytes.Contains(currentStacks, keyBuf[:n]) {
				newIDs[key] = id
				newInUse[key] = struct{}{}
			}
			bufferPool.Put(&keyBuf)
		}
	}

	// Atomic replacement of entire maps ensures consistency
	idStore.ids = newIDs
	idStore.inUse = newInUse
}

// GetStoredGoroutineCount returns number of stored IDs for monitoring
func GetStoredGoroutineCount() int {
	idStore.RLock()
	defer idStore.RUnlock()
	return len(idStore.ids)
}

// ForceCleanup removes all IDs for testing/reset purposes
// Creates entirely new maps to ensure complete cleanup
func ForceCleanup() {
	idStore.Lock()
	defer idStore.Unlock()

	// Use fixed capacity to prevent growing beyond expected size
	idStore.ids = make(map[[8]byte]uint64, 1024)
	idStore.inUse = make(map[[8]byte]struct{}, 1024)
}
