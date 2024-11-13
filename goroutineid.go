package rwmutexplus

import (
	"bytes"
	"hash/fnv"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	goroutineCounter atomic.Uint64
	bufferPool       = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 64)
			return &b
		},
	}

	// Main storage with RWMutex for better concurrency
	idStore struct {
		sync.RWMutex
		ids   map[[8]byte]uint64
		inUse map[[8]byte]struct{} // Track actively used IDs
	}
)

func init() {
	idStore.ids = make(map[[8]byte]uint64, 1024)
	idStore.inUse = make(map[[8]byte]struct{}, 1024)
}

// GetGoroutineID returns a unique identifier for the current goroutine
func GetGoroutineID() uint64 {
	key := getGoRoutineKey()

	// Fast path with read lock
	idStore.RLock()
	if id, exists := idStore.ids[key]; exists {
		// Mark as in use
		idStore.RUnlock()
		idStore.Lock()
		idStore.inUse[key] = struct{}{}
		idStore.Unlock()
		return id
	}
	idStore.RUnlock()

	// Slow path with write lock
	idStore.Lock()
	defer idStore.Unlock()

	// Double-check after acquiring write lock
	if id, exists := idStore.ids[key]; exists {
		idStore.inUse[key] = struct{}{}
		return id
	}

	// Create new ID
	id := goroutineCounter.Add(1)
	idStore.ids[key] = id
	idStore.inUse[key] = struct{}{}
	return id
}

// ReleaseGoroutineID marks an ID as no longer in active use
func ReleaseGoroutineID() {
	key := getGoRoutineKey()
	idStore.Lock()
	delete(idStore.inUse, key)
	idStore.Unlock()
}

func getGoRoutineKey() [8]byte {
	buf := *(bufferPool.Get().(*[]byte))
	defer bufferPool.Put(&buf)

	n := runtime.Stack(buf, false)
	h := fnv.New64a()

	if idx := bytes.IndexByte(buf[:n], '\n'); idx > 0 {
		h.Write(buf[:idx])
	} else {
		h.Write(buf[:n])
	}

	var key [8]byte
	copy(key[:], h.Sum(nil))
	return key
}

// // CleanupGoroutineIDs safely removes unused IDs
// func CleanupGoroutineIDs() {
// 	// Get all current goroutine stacks
// 	buf := make([]byte, 102400)
// 	n := runtime.Stack(buf, true)
// 	currentStacks := buf[:n]

// 	idStore.Lock()
// 	defer idStore.Unlock()

// 	// Create new maps
// 	newIDs := make(map[[8]byte]uint64, len(idStore.ids))
// 	newInUse := make(map[[8]byte]struct{}, len(idStore.inUse))

// 	// First, copy all actively used IDs
// 	for key := range idStore.inUse {
// 		if id, exists := idStore.ids[key]; exists {
// 			newIDs[key] = id
// 			newInUse[key] = struct{}{}
// 		}
// 	}

// 	// Then check for any other valid goroutines
// 	for key, id := range idStore.ids {
// 		keyBuf := *(bufferPool.Get().(*[]byte))
// 		n := runtime.Stack(keyBuf, false)

// 		if bytes.Contains(currentStacks, keyBuf[:n]) {
// 			newIDs[key] = id
// 		}

// 		bufferPool.Put(&keyBuf)
// 	}

// 	// Atomic replacement
// 	idStore.ids = newIDs
// 	idStore.inUse = newInUse
// }

// GetStoredGoroutineCount returns count of stored IDs
func GetStoredGoroutineCount() int {
	idStore.RLock()
	defer idStore.RUnlock()
	return len(idStore.ids)
}

// ForceCleanup removes all IDs (for testing)
func ForceCleanup() {
	idStore.Lock()
	defer idStore.Unlock()

	// Completely new maps
	idStore.ids = make(map[[8]byte]uint64, 1024)
	idStore.inUse = make(map[[8]byte]struct{}, 1024)
}

// CleanupGoroutineIDs safely removes unused IDs
func CleanupGoroutineIDs() {
	// Get current goroutine stacks
	buf := make([]byte, 102400)
	n := runtime.Stack(buf, true)
	currentStacks := buf[:n]

	idStore.Lock()
	defer idStore.Unlock()

	// Only keep IDs that are either in use or belong to current goroutines
	newIDs := make(map[[8]byte]uint64, 1024)
	newInUse := make(map[[8]byte]struct{}, 1024)

	for key, id := range idStore.ids {
		// Check if ID is in use
		if _, inUse := idStore.inUse[key]; inUse {
			// Verify the goroutine still exists
			keyBuf := *(bufferPool.Get().(*[]byte))
			n := runtime.Stack(keyBuf, false)
			if bytes.Contains(currentStacks, keyBuf[:n]) {
				newIDs[key] = id
				newInUse[key] = struct{}{}
			}
			bufferPool.Put(&keyBuf)
		}
	}

	// Replace with cleaned maps
	idStore.ids = newIDs
	idStore.inUse = newInUse
}
