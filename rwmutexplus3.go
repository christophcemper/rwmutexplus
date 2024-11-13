package rwmutexplus

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LockRequest tracks an ongoing lock request
type LockRequest struct {
	purpose     string
	startTime   time.Time
	goroutineID uint64
	callerInfo  string
}

// ActiveLock tracks an acquired lock
type ActiveLock struct {
	purpose     string
	acquiredAt  time.Time
	goroutineID uint64
	callerInfo  string
	stack       string
}

// LockStats tracks statistics for a specific lock purpose
type LockStats struct {
	totalAcquired    atomic.Int64
	totalContentions atomic.Int64
	totalTimeHeld    atomic.Int64 // in nanoseconds
	maxTimeHeld      atomic.Int64 // in nanoseconds
	totalWaitTime    atomic.Int64 // in nanoseconds
	totalWaitEvents  atomic.Int64
	maxWaitTime      atomic.Int64 // in nanoseconds
}

// RWMutexPlus provides a robust logging RWMutex implementation
type RWMutexPlus struct {
	sync.RWMutex
	name           string
	warningTimeout time.Duration
	logger         *log.Logger

	// Protected by internal mutex
	internal      sync.Mutex
	pendingWrites map[uint64]*LockRequest // Tracks write lock requests
	pendingReads  map[uint64]*LockRequest // Tracks read lock requests
	activeWrites  map[uint64]*ActiveLock  // Tracks active write locks
	activeReads   map[uint64]*ActiveLock  // Tracks active read locks
	purposeStats  map[string]*LockStats   // Statistics per purpose

	// Atomic counters
	activeReaders   atomic.Int32
	contentionCount atomic.Int64

	// VerboseLevel controls the verbosity of logging (0-3)
	verboseLevel int
	// DebugLevel controls debug logging (0 or 1)
	debugLevel int
}

// WithVerboseLevel sets the verbose level (0-3) for the mutex and returns the mutex for chaining
func (rw *RWMutexPlus) WithVerboseLevel(level int) *RWMutexPlus {
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	rw.verboseLevel = level
	return rw
}

// WithDebugLevel sets the debug level (0-1) for the mutex and returns the mutex for chaining
func (rw *RWMutexPlus) WithDebugLevel(level int) *RWMutexPlus {
	if level < 0 {
		level = 0
	} else if level > 1 {
		level = 1
	}
	rw.debugLevel = level
	return rw
}

func NewRWMutexPlus(name string, warningTimeout time.Duration, logger *log.Logger) *RWMutexPlus {
	fmt.Printf("\n\nDEBUG: NewRWMutexPlus (%s, %v)\n", name, warningTimeout)
	if logger == nil {
		logger = log.Default()
	}
	mutex := &RWMutexPlus{
		name:           name,
		warningTimeout: warningTimeout,
		logger:         logger,
		pendingWrites:  make(map[uint64]*LockRequest),
		pendingReads:   make(map[uint64]*LockRequest),
		activeWrites:   make(map[uint64]*ActiveLock),
		activeReads:    make(map[uint64]*ActiveLock),
		purposeStats:   make(map[string]*LockStats),
		verboseLevel:   1, // Default verbose level
		debugLevel:     0, // Default debug level
	}

	// Register with global registry
	globalRegistry.register(mutex)

	return mutex
}

// Add Close method to RWMutexPlus struct
func (rw *RWMutexPlus) Close() {
	// First ensure we have no active locks
	rw.internal.Lock()
	if len(rw.activeWrites) > 0 || len(rw.activeReads) > 0 {
		rw.internal.Unlock()
		rw.logger.Printf("[%s] WARNING: Attempting to close mutex with active locks (writes: %d, reads: %d)",
			rw.name, len(rw.activeWrites), len(rw.activeReads))
		return
	}
	rw.internal.Unlock()

	// Unregister from global registry
	globalRegistry.unregister(rw.name)
}

func (rw *RWMutexPlus) getOrCreateStats(purpose string) *LockStats {
	rw.internal.Lock()
	defer rw.internal.Unlock()

	stats, exists := rw.purposeStats[purpose]
	if !exists {
		stats = &LockStats{}
		rw.purposeStats[purpose] = stats
	}
	return stats
}

// logVerboseAction logs detailed lock action information if verbosity requirements are met
func (rw *RWMutexPlus) logVerboseAction(lockAction string, purpose string, goroutineID uint64, callerInfo string) {
	if rw.verboseLevel >= 3 && rw.debugLevel > 0 {
		pos := fmt.Sprintf("purpose '%s' (goroutine %d, %s)", purpose, goroutineID, callerInfo)
		rw.logger.Printf("[%s] %s for %s", rw.name, lockAction, pos)
		rw.PrintLockInfo(rw.name + " " + pos)
	}
}

// logTimeoutWarning logs a warning if duration exceeds timeout, and optionally logs verbose action
func (rw *RWMutexPlus) logTimeoutWarning(duration time.Duration, lockAction string, purpose string, goroutineID uint64, callerInfo string) {
	if duration > rw.warningTimeout {
		str := fmt.Sprintf("[%s] WARNING: %s for purpose '%s' took %v exceeding timeout of %v (goroutine %d, %s)",
			rw.name, lockAction, purpose, duration, rw.warningTimeout, goroutineID, callerInfo)
		fmt.Println("--------------------------------")
		fmt.Println(str)
		fmt.Println("--------------------------------")
		rw.PrintLockInfo(str)
		fmt.Println("--------------------------------")
		fmt.Println("##### ALL RWMUTEXES ------------")
		fmt.Println(DumpAllLockInfo())
		fmt.Println("--------------------------------")
		rw.logger.Printf("%s", str)
	} else if rw.verboseLevel >= 3 && rw.debugLevel > 0 {
		rw.logVerboseAction(lockAction, purpose, goroutineID, callerInfo)
	}
}

func (rw *RWMutexPlus) LockWithPurpose(purpose string) {
	defer ReleaseGoroutineID()
	goroutineID := GetGoroutineID()
	callerInfo := getCallerInfo()
	// Create lock request
	request := &LockRequest{
		purpose:     purpose,
		startTime:   time.Now(),
		goroutineID: goroutineID,
		callerInfo:  callerInfo,
	}

	// Register pending write request
	rw.internal.Lock()
	rw.pendingWrites[goroutineID] = request
	rw.internal.Unlock()

	// Before attempting lock:
	rw.logVerboseAction("Write lock requested", purpose, goroutineID, callerInfo)

	// Start lock acquisition
	lockChan := make(chan struct{})
	go func() {
		rw.RWMutex.Lock()
		close(lockChan)
	}()

	// Wait with timeout
	select {
	case <-lockChan:
		// Lock acquired

	case <-time.After(rw.warningTimeout):

		rw.internal.Lock()
		req := rw.pendingWrites[goroutineID]
		rw.internal.Unlock()

		waitTime := time.Since(req.startTime)
		rw.logger.Printf("[%s] WARNING: Write lock for purpose '%s' taking %v to acquire (goroutine %d, %s)",
			rw.name, purpose, waitTime, goroutineID, callerInfo)

		// Wait for actual acquisition
		<-lockChan

		// Update contention stats
		stats := rw.getOrCreateStats(purpose)
		stats.totalContentions.Add(1)
		rw.contentionCount.Add(1)
	}

	// Lock acquired - create active lock record
	activeLock := &ActiveLock{
		purpose:     purpose,
		acquiredAt:  time.Now(),
		goroutineID: goroutineID,
		callerInfo:  callerInfo,
		stack:       captureStack(),
	}

	// Update records
	rw.internal.Lock()
	delete(rw.pendingWrites, goroutineID)
	rw.activeWrites[goroutineID] = activeLock
	rw.internal.Unlock()

	// Update stats
	stats := rw.getOrCreateStats(purpose)
	stats.totalAcquired.Add(1)

	waitTime := time.Since(request.startTime)

	// Update wait time stats
	stats.totalWaitTime.Add(int64(waitTime))
	stats.totalWaitEvents.Add(1)

	rw.logTimeoutWarning(waitTime, "Write lock acquisition", purpose, goroutineID, callerInfo)

	// Start monitoring
	go rw.monitorWriteLock(goroutineID, activeLock)

	// Update max wait time if needed
	for {
		current := stats.maxWaitTime.Load()
		if int64(waitTime) <= current {
			break
		}
		if stats.maxWaitTime.CompareAndSwap(current, int64(waitTime)) {
			break
		}
	}
}

func (rw *RWMutexPlus) RLockWithPurpose(purpose string) {
	goroutineID := GetGoroutineID()
	callerInfo := getCallerInfo()

	// Create lock request
	request := &LockRequest{
		purpose:     purpose,
		startTime:   time.Now(),
		goroutineID: goroutineID,
		callerInfo:  callerInfo,
	}

	// Register pending read request
	rw.internal.Lock()
	rw.pendingReads[goroutineID] = request
	rw.internal.Unlock()

	// Before attempting lock:
	rw.logVerboseAction("Read lock requested", purpose, goroutineID, callerInfo)

	// Start lock acquisition
	lockChan := make(chan struct{})
	go func() {
		rw.RWMutex.RLock()
		close(lockChan)
	}()

	// Wait with timeout
	select {
	case <-lockChan:
		// Lock acquired

	case <-time.After(rw.warningTimeout):
		rw.internal.Lock()
		req := rw.pendingReads[goroutineID]
		rw.internal.Unlock()

		waitTime := time.Since(req.startTime)
		rw.logger.Printf("[%s] WARNING: Read lock for purpose '%s' taking %v to acquire (goroutine %d, %s)",
			rw.name, purpose, waitTime, goroutineID, callerInfo)

		// Wait for actual acquisition
		<-lockChan

		// Update contention stats
		stats := rw.getOrCreateStats(purpose)
		stats.totalContentions.Add(1)
		rw.contentionCount.Add(1)
	}

	// Lock acquired - create active lock record
	activeLock := &ActiveLock{
		purpose:     purpose,
		acquiredAt:  time.Now(),
		goroutineID: goroutineID,
		callerInfo:  callerInfo,
		stack:       captureStack(),
	}

	// Update records
	rw.internal.Lock()
	delete(rw.pendingReads, goroutineID)
	rw.activeReads[goroutineID] = activeLock
	rw.internal.Unlock()

	// Update stats
	stats := rw.getOrCreateStats(purpose)
	stats.totalAcquired.Add(1)
	rw.activeReaders.Add(1)

	waitTime := time.Since(request.startTime)

	// Update wait time stats
	stats.totalWaitTime.Add(int64(waitTime))
	stats.totalWaitEvents.Add(1)

	rw.logTimeoutWarning(waitTime, "Read lock acquisition", purpose, goroutineID, callerInfo)

	// Start monitoring
	go rw.monitorReadLock(goroutineID, activeLock)

	// Update wait time stats
	stats = rw.getOrCreateStats(purpose)
	stats.totalWaitTime.Add(int64(waitTime))

	// Update max wait time if needed
	for {
		current := stats.maxWaitTime.Load()
		if int64(waitTime) <= current {
			break
		}
		if stats.maxWaitTime.CompareAndSwap(current, int64(waitTime)) {
			break
		}
	}
}

// Add these methods to support backward compatibility
func (rw *RWMutexPlus) Lock() {
	rw.LockWithPurpose("unspecified")
}

func (rw *RWMutexPlus) RLock() {
	rw.RLockWithPurpose("unspecified")
}

// Fix the Unlock method to properly handle the activeLock check
func (rw *RWMutexPlus) Unlock() {
	goroutineID := GetGoroutineID()

	// Check if lock exists for this goroutine
	rw.internal.Lock()
	activeLock, exists := rw.activeWrites[goroutineID]
	rw.internal.Unlock()

	if !exists {
		// Include more context in panic message
		rw.logger.Printf("[%s] WARNING: No active write lock found for goroutine %d", rw.name, goroutineID)
		rw.logger.Printf("[%s] Active write locks: %+v", rw.name, rw.activeWrites)
		panic(fmt.Sprintf("[%s] attempting to unlock an unlocked write mutex (goroutine %d)",
			rw.name, goroutineID))
	}

	// Now proceed with the actual unlock
	rw.internal.Lock()
	holdDuration := time.Since(activeLock.acquiredAt)
	delete(rw.activeWrites, goroutineID)
	rw.internal.Unlock()

	// Update stats before unlocking
	stats := rw.getOrCreateStats(activeLock.purpose)
	stats.totalTimeHeld.Add(int64(holdDuration))

	// Update max hold time
	for {
		current := stats.maxTimeHeld.Load()
		if int64(holdDuration) <= current {
			break
		}
		if stats.maxTimeHeld.CompareAndSwap(current, int64(holdDuration)) {
			break
		}
	}

	// Actually unlock
	rw.RWMutex.Unlock()

	rw.logTimeoutWarning(holdDuration, "Write lock unlocked - held", activeLock.purpose, goroutineID, activeLock.callerInfo)
}

func (rw *RWMutexPlus) RUnlock() {
	goroutineID := GetGoroutineID()

	rw.internal.Lock()
	activeLock, exists := rw.activeReads[goroutineID]
	if !exists {
		rw.internal.Unlock()
		panic("attempting to unlock an unlocked read mutex")
	}

	holdDuration := time.Since(activeLock.acquiredAt)
	delete(rw.activeReads, goroutineID)
	rw.internal.Unlock()

	// Update stats
	stats := rw.getOrCreateStats(activeLock.purpose)
	stats.totalTimeHeld.Add(int64(holdDuration))
	for {
		current := stats.maxTimeHeld.Load()
		if int64(holdDuration) <= current {
			break
		}
		if stats.maxTimeHeld.CompareAndSwap(current, int64(holdDuration)) {
			break
		}
	}

	rw.RWMutex.RUnlock()
	rw.activeReaders.Add(-1)

	rw.logTimeoutWarning(holdDuration, "Read lock unlocked - held", activeLock.purpose, goroutineID, activeLock.callerInfo)
}

func (rw *RWMutexPlus) monitorWriteLock(goroutineID uint64, lock *ActiveLock) {
	ticker := time.NewTicker(rw.warningTimeout)
	defer ticker.Stop()

	for range ticker.C {
		rw.internal.Lock()
		_, exists := rw.activeWrites[goroutineID]
		if !exists {
			rw.internal.Unlock()
			return
		}

		holdDuration := time.Since(lock.acquiredAt)
		rw.internal.Unlock()

		rw.logTimeoutWarning(holdDuration, "Write lock held", lock.purpose, goroutineID, lock.callerInfo)
	}
}

func (rw *RWMutexPlus) monitorReadLock(goroutineID uint64, lock *ActiveLock) {
	ticker := time.NewTicker(rw.warningTimeout)
	defer ticker.Stop()

	for range ticker.C {
		rw.internal.Lock()
		_, exists := rw.activeReads[goroutineID]
		if !exists {
			rw.internal.Unlock()
			return
		}

		holdDuration := time.Since(lock.acquiredAt)
		rw.internal.Unlock()

		rw.logTimeoutWarning(holdDuration, "Read lock held", lock.purpose, goroutineID, lock.callerInfo)
	}
}

// GetPurposeStats returns statistics for all lock purposes
func (rw *RWMutexPlus) GetPurposeStats() map[string]map[string]int64 {
	rw.internal.Lock()
	defer rw.internal.Unlock()

	stats := make(map[string]map[string]int64)
	for purpose, purposeStats := range rw.purposeStats {
		stats[purpose] = map[string]int64{
			"total_acquired":    purposeStats.totalAcquired.Load(),
			"total_contentions": purposeStats.totalContentions.Load(),
			"total_time_held":   purposeStats.totalTimeHeld.Load(),
			"max_time_held":     purposeStats.maxTimeHeld.Load(),
			"total_wait_time":   purposeStats.totalWaitTime.Load(),
			"total_wait_events": purposeStats.totalWaitEvents.Load(),
			"max_wait_time":     purposeStats.maxWaitTime.Load(),
		}
	}
	return stats
}

// GetActiveLocksInfo returns information about all currently held locks
func (rw *RWMutexPlus) GetActiveLocksInfo() map[string][]map[string]interface{} {
	rw.internal.Lock()
	defer rw.internal.Unlock()

	info := map[string][]map[string]interface{}{
		"read_locks":  make([]map[string]interface{}, 0, len(rw.activeReads)),
		"write_locks": make([]map[string]interface{}, 0, len(rw.activeWrites)),
	}

	for _, lock := range rw.activeReads {
		info["read_locks"] = append(info["read_locks"], map[string]interface{}{
			"purpose":      lock.purpose,
			"acquired_at":  lock.acquiredAt,
			"goroutine_id": lock.goroutineID,
			"caller_info":  lock.callerInfo,
			"held_for":     time.Since(lock.acquiredAt),
		})
	}

	for _, lock := range rw.activeWrites {
		info["write_locks"] = append(info["write_locks"], map[string]interface{}{
			"purpose":      lock.purpose,
			"acquired_at":  lock.acquiredAt,
			"goroutine_id": lock.goroutineID,
			"caller_info":  lock.callerInfo,
			"held_for":     time.Since(lock.acquiredAt),
		})
	}

	return info
}

// GetPendingLocksInfo returns information about locks waiting to be acquired
func (rw *RWMutexPlus) GetPendingLocksInfo() map[string][]map[string]interface{} {
	rw.internal.Lock()
	defer rw.internal.Unlock()

	info := map[string][]map[string]interface{}{
		"pending_reads":  make([]map[string]interface{}, 0, len(rw.pendingReads)),
		"pending_writes": make([]map[string]interface{}, 0, len(rw.pendingWrites)),
	}

	for _, req := range rw.pendingReads {
		info["pending_reads"] = append(info["pending_reads"], map[string]interface{}{
			"purpose":      req.purpose,
			"waiting_for":  time.Since(req.startTime),
			"goroutine_id": req.goroutineID,
			"caller_info":  req.callerInfo,
		})
	}

	for _, req := range rw.pendingWrites {
		info["pending_writes"] = append(info["pending_writes"], map[string]interface{}{
			"purpose":      req.purpose,
			"waiting_for":  time.Since(req.startTime),
			"goroutine_id": req.goroutineID,
			"caller_info":  req.callerInfo,
		})
	}

	return info
}

func captureStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// PrintLockInfo prints detailed information about the current state of the mutex
// including active locks, pending locks, and statistics for each purpose.
// pos is a string identifier to help track where the print was called from.
func (m *RWMutexPlus) PrintLockInfo(pos string) {
	activeLocks := m.GetActiveLocksInfo()
	pendingLocks := m.GetPendingLocksInfo()
	purposeStats := m.GetPurposeStats()

	fmt.Println("--------------------------------")
	fmt.Printf("Position: %s\n", pos)
	fmt.Printf("Active Locks (Read: %d, Write: %d): %+v\n",
		len(activeLocks["read_locks"]), len(activeLocks["write_locks"]), activeLocks)
	fmt.Printf("Pending Locks (Read: %d, Write: %d): %+v\n",
		len(pendingLocks["pending_reads"]), len(pendingLocks["pending_writes"]), pendingLocks)

	for purpose, stats := range purposeStats {
		fmt.Printf("Purpose: %s\n", purpose)
		fmt.Printf("  Total acquired: %d\n", stats["total_acquired"])
		fmt.Printf("  Total contentions: %d\n", stats["total_contentions"])
		if stats["total_acquired"] > 0 {
			fmt.Printf("  Average hold time: %v\n", time.Duration(stats["total_time_held"]/stats["total_acquired"]))
		}
		fmt.Printf("  Max hold time: %v\n", time.Duration(stats["max_time_held"]))
		// if stats["total_wait_events"] > 0 {
		fmt.Printf("  Total wait events: %d\n", stats["total_wait_events"])
		// fmt.Printf("  Average wait time: %v\n", time.Duration(stats["total_wait_time"]/stats["total_wait_events"]))
		fmt.Printf("  Total wait time: %v\n", time.Duration(stats["total_wait_time"]))
		fmt.Printf("  Max wait time: %v\n", time.Duration(stats["max_wait_time"]))
		// }

	}
	fmt.Println("--------------------------------")
}

// shouldIncludeLine returns true if the line should be included in stack traces
// and caller information, filtering out runtime, testing, and debug-related frames.
func shouldIncludeLine(line string) bool {

	// we want to see the test code in the stack trace
	if strings.Contains(line, "rwmutexplus_test.go") ||
		strings.Contains(line, "rwmutexplus/examples") {
		return true
	}
	return !strings.Contains(line, "runtime/") &&
		!strings.Contains(line, "runtime.") &&
		!strings.Contains(line, "testing/") &&
		!strings.Contains(line, "testing.") &&
		!strings.Contains(line, "rwmutexplus") &&
		!strings.Contains(line, "debug.Stack") &&
		!strings.Contains(line, "debug/stack")
}

// filterStack removes unnecessary runtime and testing lines from stack traces
// to make the output more readable and focused on application code.
func filterStack(stack []byte) string {
	lines := strings.Split(string(stack), "\n")
	var filtered []string
	for i, line := range lines {
		if shouldIncludeLine(line) {
			filtered = append(filtered, lines[i])
		}
	}
	return strings.Join(filtered, "\n")
}

// getCallerInfo returns the caller information for the function that called Lock or RLock.
// It skips the runtime, testing, and debug frames to focus on application code.
// except for tests where we want to see the test code in the stack trace
func getCallerInfo() string {
	var pcs [32]uintptr
	n := runtime.Callers(0, pcs[:]) // Increase skip count to 3 to get past runtime frames
	frames := runtime.CallersFrames(pcs[:n])

	// Get the first 3 non-runtime frames
	callerInfo := ""
	callerInfoLines := 0
	for callerInfoLines <= 3 {
		frame, more := frames.Next()
		if shouldIncludeLine(frame.File) {
			// we want to the last part of the function name after the last /
			funcName := strings.Split(frame.Function, "/")[len(strings.Split(frame.Function, "/"))-1]
			// fileName := strings.Split(frame.File, "/")[len(strings.Split(frame.File, "/"))-1]
			callerInfo1 := fmt.Sprintf("%s\n\t%s:%d %s", funcName, frame.File, frame.Line, funcName)
			callerInfo = callerInfo + callerInfo1
			callerInfoLines++
		}
		if !more {
			break
		}
	}
	// fmt.Printf("#### \n\n #### callerInfo: %s\n", callerInfo)
	if callerInfo == "" {
		callerInfo = "unknown"
	}
	return callerInfo
}
