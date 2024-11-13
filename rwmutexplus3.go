package rwmutexplus

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LockStats tracks statistics for a specific lock purpose
type LockStats struct {
	totalAcquired          atomic.Int64
	totalTimeoutsReadLock  atomic.Int64
	totalTimeoutsWriteLock atomic.Int64
	totalTimeHeld          atomic.Int64 // in nanoseconds
	maxTimeHeld            atomic.Int64 // in nanoseconds
	totalWaitTime          atomic.Int64 // in nanoseconds
	totalWaitEvents        atomic.Int64
	maxWaitTime            atomic.Int64 // in nanoseconds
}

// RWMutexPlus provides a robust logging RWMutex implementation
type RWMutexPlus struct {
	sync.RWMutex
	name           string
	warningTimeout time.Duration // CFG
	logger         *log.Logger

	// Protected by internal mutex
	internal      sync.Mutex
	pendingWrites map[uint64]*LockRequest // Tracks write lock requests
	pendingReads  map[uint64]*LockRequest // Tracks read lock requests
	activeWrites  map[uint64]*ActiveLock  // Tracks active write locks
	activeReads   map[uint64]*ActiveLock  // Tracks active read locks
	purposeStats  map[string]*LockStats   // Statistics per purpose

	// Atomic counters
	activeReaders     atomic.Int32
	timeoutsReadLock  atomic.Int64
	timeoutsWriteLock atomic.Int64

	// VerboseLevel controls the verbosity of logging (0-4)
	// 4 = print ALL RWMutex info with every Debug Print of the current mutes using registry.DumpAllLockInfo().
	//     WARNING: Leads to extreme log volume - ONLY needed for complex real time debugging scenarios
	// 3 = print current RXMutex stats + info
	// 2 = print only one line per lock action + stats
	// 1 = print only one line per lock action
	// 0 = print nothing extra
	verboseLevel int
	// DebugLevel controls debug logging (0 or 1)
	debugLevel int
	// callerInfoLines controls how many (useful) lines of caller stack info to include in logs
	callerInfoLines int
	// callerInfoSkip controls how many lines of caller stack info to skip in logs
	callerInfoSkip int
	// lastLogMessages tracks last logged messages to prevent duplicates
	lastLogMessages sync.Map
}

func (rw *RWMutexPlus) setVerboseLevel(level int) int {
	if level < 0 {
		level = 0
	} else if level > 4 {
		level = 4
	}
	rw.verboseLevel = level
	return level
}

// WithVerboseLevel sets the verbose level (0-3) for the mutex and returns the mutex for chaining
func (rw *RWMutexPlus) WithVerboseLevel(level int) *RWMutexPlus {
	if override := rw.OverrideVerboseLevelFromEnv(); override {
		return rw
	}
	level = rw.setVerboseLevel(level)
	rw.DebugPrint(fmt.Sprintf("\nDEBUG: RWMutexPlus (%s, %v) - WithVerboseLevel (%d)", rw.name, rw.warningTimeout, level), 1)
	return rw
}

func (rw *RWMutexPlus) setDebugLevel(level int) int {
	if level < 0 {
		level = 0
	} else if level > 1 {
		level = 1
	}
	rw.debugLevel = level
	return level
}

// WithDebugLevel sets the debug level (0-1) for the mutex and returns the mutex for chaining
func (rw *RWMutexPlus) WithDebugLevel(level int) *RWMutexPlus {
	if override := rw.OverrideDebugLevelFromEnv(); override {
		return rw
	}
	level = rw.setDebugLevel(level)
	rw.DebugPrint(fmt.Sprintf("\nDEBUG: RWMutexPlus (%s, %v) - WithDebugLevel (%d)", rw.name, rw.warningTimeout, level), 1)
	return rw
}

// check os.Getenv variables RWMUTEXTPLUS_VERBOSELEVEL and RWMUTEXTPLUS_DEBUGLEVEL to set the verboseLevel and debugLevel and override any defaults from the constructor code or With...Level setters
func (rw *RWMutexPlus) OverrideVerboseLevelFromEnv() bool {
	if verboseLevel := GetIntEnvOrDefault("RWMUTEXTPLUS_VERBOSELEVEL", rw.verboseLevel); verboseLevel >= 0 {
		rw.PrintOncef("\nENV: RWMutexPlus (%s, %v) - RWMUTEXTPLUS_VERBOSELEVEL (%d)",
			rw.name, rw.warningTimeout, rw.setVerboseLevel(verboseLevel))
		return true
	}
	return false
}

func (rw *RWMutexPlus) OverrideDebugLevelFromEnv() bool {
	if debugLevel := GetIntEnvOrDefault("RWMUTEXTPLUS_DEBUGLEVEL", rw.debugLevel); debugLevel >= 0 {
		rw.PrintOncef("\nENV: RWMutexPlus (%s, %v) - RWMUTEXTPLUS_DEBUGLEVEL (%d)",
			rw.name, rw.warningTimeout, rw.setDebugLevel(debugLevel))
		return true
	}
	return false
}

func (rw *RWMutexPlus) OverrideWarningTimeoutFromEnv() bool {
	if timeout := GetDurationEnvOrDefault("RWMUTEXTPLUS_TIMEOUT", rw.warningTimeout); timeout > 0 {
		rw.PrintOncef("\nENV: RWMutexPlus (%s, %v) - RWMUTEXTPLUS_TIMEOUT (%v)",
			rw.name, rw.warningTimeout, timeout)
		rw.warningTimeout = timeout
		return true
	}
	return false
}

func (rw *RWMutexPlus) WithCallerInfoLines(lines int) *RWMutexPlus {
	if override := rw.OverrideCallerInfoLinesFromEnv(); override {
		return rw
	}
	rw.callerInfoLines = lines
	rw.DebugPrint(fmt.Sprintf("\nDEBUG: RWMutexPlus (%s, %v) - WithCallerInfoLines (%d)", rw.name, rw.warningTimeout, lines), 1)
	return rw
}

func (rw *RWMutexPlus) OverrideCallerInfoLinesFromEnv() bool {
	if lines := GetIntEnvOrDefault("RWMUTEXTPLUS_CALLERINFOLINES", rw.callerInfoLines); lines >= 0 {
		rw.PrintOncef("\nENV: RWMutexPlus (%s, %v) - RWMUTEXTPLUS_CALLERINFOLINES (%d)", rw.name, rw.warningTimeout, lines)
		rw.callerInfoLines = lines
		return true
	}
	return false
}

func (rw *RWMutexPlus) OverrideCallerInfoSkipFromEnv() bool {
	if skip := GetIntEnvOrDefault("RWMUTEXTPLUS_CALLERINFOSKIP", rw.callerInfoSkip); skip >= 0 {
		rw.PrintOncef("\nENV: RWMutexPlus (%s, %v) - RWMUTEXTPLUS_CALLERINFOSKIP (%d)", rw.name, rw.warningTimeout, skip)
		rw.callerInfoSkip = skip
		return true
	}
	return false
}

func NewRWMutexPlus(name string, warningTimeout time.Duration, logger *log.Logger) *RWMutexPlus {
	if logger == nil {
		logger = log.Default()
	}
	mutex := &RWMutexPlus{
		name:            name,
		warningTimeout:  warningTimeout,
		logger:          logger,
		pendingWrites:   make(map[uint64]*LockRequest),
		pendingReads:    make(map[uint64]*LockRequest),
		activeWrites:    make(map[uint64]*ActiveLock),
		activeReads:     make(map[uint64]*ActiveLock),
		purposeStats:    make(map[string]*LockStats),
		verboseLevel:    1, // Default verbose level
		debugLevel:      0, // Default debug level
		callerInfoLines: 3, // Default number of lines of caller stack info to include in logs
		callerInfoSkip:  0, // Default number of lines of caller stack info to skip in logs
	}

	mutex.OverrideVerboseLevelFromEnv()
	mutex.OverrideDebugLevelFromEnv()
	mutex.OverrideWarningTimeoutFromEnv()
	mutex.OverrideCallerInfoLinesFromEnv()
	mutex.OverrideCallerInfoSkipFromEnv()

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
func (rw *RWMutexPlus) logVerboseAction(action string, li LockInfo) {
	str := fmt.Sprintf("[%s] %s %s %s for %s", rw.name, action, li.GetLockTX().String(), li.GetLockType().String(), li.GetPurpose())
	if rw.verboseLevel >= 3 || rw.debugLevel > 0 {
		str += li.GetCallerInfo()
		rw.DebugPrintAllLockInfo(str)
		return
	}

	switch rw.verboseLevel {
	case 2:
		{
			rw.logger.Printf("%s", str)
			rw.PrintLockInfo(str)
		}
	case 1:
		{
			rw.logger.Printf("%s", str)
		}
	default:
		return
	}

}

func (rw *RWMutexPlus) DebugPrintAllLockInfo(pos string) {
	if rw.verboseLevel >= 2 || rw.debugLevel > 0 {

		if rw.verboseLevel >= 3 {
			fmt.Println("================================")
			rw.PrintLockInfo(pos)
			fmt.Println("--------------------------------")
		}
		if rw.verboseLevel >= 4 {
			fmt.Println("-------- ALL RWMUTEXES ---------")
			fmt.Println(DumpAllLockInfo())

		}
		if rw.debugLevel >= 3 {
			fmt.Println("================================")
		}
	}
}

// DebugPrint prints the string if debug level is greater than 0
// if variadic/optional param level is provided, it will print only if level is greater than or equal to debugLevel
func (rw *RWMutexPlus) DebugPrint(str string, level ...int) {
	if rw.debugLevel > 0 {
		if len(level) > 0 && level[0] >= rw.debugLevel {
			rw.logger.Printf("%s", str)
		}
	}
}

// we want to have rounded duration with units, i.e. 53ms instead of 53.23232ms
func formatDuration(duration time.Duration) string {
	durationStr := fmt.Sprintf("%v", duration)
	durationStr = strings.Split(durationStr, ".")[0]
	durationUnit := ""

	return durationStr + durationUnit
}

// logTimeoutWarning logs a warning if duration exceeds timeout
// and optionally logs verbose lockAction and whatLock
// optional timesWarning is the number of times the warning has been logged
func (rw *RWMutexPlus) logTimeoutWarning(action string, lockInfo LockInfo, timesWarning ...int) {

	if duration := lockInfo.GetSinceTime(); duration > rw.warningTimeout {
		lockAction := action + " " + lockInfo.GetLockTX().String() + " " + lockInfo.GetLockType().String()
		timesExceeded1 := 1
		// divide duration by warningTimeout to get number of times it has exceeded the timeout
		if rw.warningTimeout > 0 {
			timesExceeded1 = int(duration / rw.warningTimeout)
		}
		timesWarning1 := 1
		if len(timesWarning) > 0 {
			timesWarning1 = timesWarning[0]
		}

		str := fmt.Sprintf("[%s] WARNING #%d: %s took %s - %dx exceeding timeout of %v - %s",
			rw.name, timesWarning1,
			lockAction,
			formatDuration(duration), timesExceeded1, rw.warningTimeout,
			lockInfo.GetPosition())
		rw.DebugPrintAllLockInfo(str)
		rw.logger.Printf("%s", str)
	} else {
		rw.logVerboseAction(action, lockInfo)
	}
}

func (rw *RWMutexPlus) LockWithPurpose(purpose string) {
	defer ReleaseGoroutineID()
	goroutineID := GetGoroutineID()
	callerInfo := getCallerInfo(rw.callerInfoSkip, rw.callerInfoLines)
	// Create lock request
	request := &LockRequest{
		lockType:    WriteLock,
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
	rw.logTimeoutWarning("WAIT FOR", request)

	// Start lock acquisition
	lockChan := make(chan struct{})
	go func() {
		rw.RWMutex.Lock()
		close(lockChan)
	}()

	timesWarning := 0
	// Wait with timeout
	select {
	case <-lockChan:
		// Lock acquired

	case <-time.After(rw.warningTimeout):

		rw.internal.Lock()
		req := rw.pendingWrites[goroutineID]
		rw.internal.Unlock()

		// waitTime := time.Since(req.startTime)

		// rw.logTimeoutWarning(waitTime, fmt.Sprintf("%s requested", req.lockType), purpose, goroutineID, callerInfo)
		timesWarning++
		// rw.logTimeoutWarning2(waitTime, "REQUEST "+req.lockType.String(), req.String(), timesWarning)
		rw.logTimeoutWarning("WAIT FOR", req, timesWarning)

		// Wait for actual acquisition
		<-lockChan

		// Update contention stats
		stats := rw.getOrCreateStats(purpose)
		stats.totalTimeoutsWriteLock.Add(1)
		rw.timeoutsWriteLock.Add(1)
	}

	// Lock acquired - create active lock record
	waitTime := time.Since(request.startTime)
	activeLock := &ActiveLock{
		lockType:        WriteLock,
		purpose:         purpose,
		acquireWaitTime: waitTime,
		acquiredAt:      time.Now(),
		goroutineID:     goroutineID,
		callerInfo:      callerInfo,
		stack:           captureStack(),
	}

	// Update records
	rw.internal.Lock()
	delete(rw.pendingWrites, goroutineID)
	rw.activeWrites[goroutineID] = activeLock
	rw.internal.Unlock()

	// Update stats
	stats := rw.getOrCreateStats(purpose)
	stats.totalAcquired.Add(1)

	// Update wait time stats
	stats.totalWaitTime.Add(int64(waitTime))
	stats.totalWaitEvents.Add(1)

	// rw.logTimeoutWarning(waitTime, fmt.Sprintf("%s aquired", activeLock.lockType), purpose, goroutineID, callerInfo)
	// rw.logTimeoutWarning(waitTime, fmt.Sprintf("%s requested", req.lockType), purpose, goroutineID, callerInfo)
	timesWarning++
	// rw.logTimeoutWarning2(waitTime, "ACQUIRED", activeLock.String(), timesWarning)
	rw.logTimeoutWarning("ACQUIRED", activeLock, timesWarning)

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
	defer ReleaseGoroutineID()
	goroutineID := GetGoroutineID()
	callerInfo := getCallerInfo(rw.callerInfoSkip, rw.callerInfoLines)

	// Create lock request
	request := &LockRequest{
		lockType:    ReadLock,
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
	rw.logTimeoutWarning("WAIT FOR", request)

	// Start lock acquisition
	lockChan := make(chan struct{})
	go func() {
		rw.RWMutex.RLock()
		close(lockChan)
	}()

	timesWarning := 0
	// Wait with timeout
	select {
	case <-lockChan:
		// Lock acquired

	case <-time.After(rw.warningTimeout):
		rw.internal.Lock()
		req := rw.pendingReads[goroutineID]
		rw.internal.Unlock()

		// waitTime := time.Since(req.startTime)
		timesWarning++
		// rw.logTimeoutWarning2(waitTime, "REQUEST "+req.lockType.String(), req.String(), timesWarning)
		rw.logTimeoutWarning("WAIT FOR", req, timesWarning)

		// Wait for actual acquisition
		<-lockChan

		// Update contention stats
		stats := rw.getOrCreateStats(purpose)
		stats.totalTimeoutsReadLock.Add(1)
		rw.timeoutsReadLock.Add(1)
	}

	// Lock acquired - create active lock record
	activeLock := &ActiveLock{
		lockType:    ReadLock,
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

	// rw.logTimeoutWarning(waitTime, "Read lock acquisition", purpose, goroutineID, callerInfo)
	timesWarning++
	// rw.logTimeoutWarning2(waitTime, "ACQUIRED "+activeLock.lockType.String(), activeLock.String(),

	rw.logTimeoutWarning("ACQUIRED", activeLock, timesWarning)

	// Start monitoring
	go rw.monitorReadLock(goroutineID, activeLock)

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
		// Replace panic with warning log and return
		rw.logger.Printf("[%s] ERROR: No active write lock found for goroutine %d", rw.name, goroutineID)
		rw.logger.Printf("[%s] Active write locks: %+v", rw.name, rw.activeWrites)
		rw.verboseLevel = 4
		rw.debugLevel = 1
		rw.DebugPrintAllLockInfo(rw.name + " " + getCallerInfo(rw.callerInfoSkip, rw.callerInfoLines))
		return
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

	// rw.logTimeoutWarning2(holdDuration, "UNLOCKED ", activeLock.String())
	rw.logTimeoutWarning("UNLOCK", activeLock)

}

func (rw *RWMutexPlus) RUnlock() {
	goroutineID := GetGoroutineID()

	rw.internal.Lock()
	activeLock, exists := rw.activeReads[goroutineID]
	if !exists {
		rw.internal.Unlock()
		// Replace panic with warning log and return
		rw.logger.Printf("[%s] ERROR: attempting to unlock an unlocked read mutex (goroutine %d)", rw.name, goroutineID)
		rw.verboseLevel = 4
		rw.debugLevel = 1
		rw.DebugPrintAllLockInfo(rw.name + " " + getCallerInfo0())
		return
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

	// rw.logTimeoutWarning2(holdDuration, "UNLOCKED "+activeLock.lockType.String(), activeLock.String())
	rw.logTimeoutWarning("UNLOCKED", activeLock)
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

		// holdDuration := time.Since(lock.acquiredAt)
		rw.internal.Unlock()

		// rw.logTimeoutWarning2(holdDuration, "HOLDING "+lock.lockType.String(), lock.String())
		rw.logTimeoutWarning("HOLDING", lock)
	}
}

func (rw *RWMutexPlus) monitorReadLock(goroutineID uint64, lock *ActiveLock) {
	timesWarning := 0
	for {
		time.Sleep(rw.warningTimeout)

		rw.internal.Lock()
		_, exists := rw.activeReads[goroutineID]
		if !exists {
			rw.internal.Unlock()
			return
		}

		// holdDuration := time.Since(lock.acquiredAt)
		rw.internal.Unlock()

		timesWarning++
		readCount := rw.activeReaders.Load()

		stat := fmt.Sprintf("%s (%d readers)\n", lock.lockType.String(), readCount)

		// rw.logTimeoutWarning2(holdDuration, "HOLDING "+stat, lock.String(), timesWarning)
		rw.logTimeoutWarning("HOLDING "+stat, lock, timesWarning)
	}
}

// GetPurposeStats returns statistics for all lock purposes
func (rw *RWMutexPlus) GetPurposeStats() map[string]map[string]int64 {
	rw.internal.Lock()
	defer rw.internal.Unlock()

	stats := make(map[string]map[string]int64)
	for purpose, purposeStats := range rw.purposeStats {
		stats[purpose] = map[string]int64{
			"total_acquired":            purposeStats.totalAcquired.Load(),
			"total_timeouts_read_lock":  purposeStats.totalTimeoutsReadLock.Load(),
			"total_timeouts_write_lock": purposeStats.totalTimeoutsWriteLock.Load(),
			"total_time_held":           purposeStats.totalTimeHeld.Load(),
			"max_time_held":             purposeStats.maxTimeHeld.Load(),
			"total_wait_time":           purposeStats.totalWaitTime.Load(),
			"total_wait_events":         purposeStats.totalWaitEvents.Load(),
			"max_wait_time":             purposeStats.maxWaitTime.Load(),
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
	fmt.Printf("Active Readers: %d\n", m.activeReaders.Load())
	fmt.Printf("Total Timeouts (Read: %d, Write: %d)\n",
		m.timeoutsReadLock.Load(), m.timeoutsWriteLock.Load())

	for purpose, stats := range purposeStats {
		fmt.Printf("Purpose: %s\n", purpose)
		fmt.Printf("  Total acquired: %d\n", stats["total_acquired"])
		fmt.Printf("  Total hold time: %v\n", time.Duration(stats["total_time_held"]))
		fmt.Printf("  Total wait time: %v\n", time.Duration(stats["total_wait_time"]))
		fmt.Printf("  Total wait events: %d\n", stats["total_wait_events"])
		fmt.Printf("  Total timeouts read lock: %d\n", stats["total_timeouts_read_lock"])
		fmt.Printf("  Total timeouts write lock: %d\n", stats["total_timeouts_write_lock"])
		if stats["total_acquired"] > 0 {
			fmt.Printf("  Average hold time: %v\n", time.Duration(stats["total_time_held"]/stats["total_acquired"]))
			fmt.Printf("  Max hold time: %v\n", time.Duration(stats["max_time_held"]))
		}
		if stats["total_wait_events"] > 0 {
			fmt.Printf("  Average wait time: %v\n", time.Duration(stats["total_wait_time"]/stats["total_wait_events"]))
			fmt.Printf("  Max wait time: %v\n", time.Duration(stats["max_wait_time"]))
		}

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

func getCallerInfo0() string {
	return getCallerInfo(0, 3)
}

// getCallerInfo returns the caller information for the function that called Lock or RLock.
// It skips the runtime, testing, and debug frames to focus on application code.
// except for tests where we want to see the test code in the stack trace
// optional skip and numlines parameters control how many lines to skip and return
func getCallerInfo(skip int, numLines int) string {

	var pcs [32]uintptr
	n := runtime.Callers(skip, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	// Get the first 3 non-runtime frames
	callerInfo := ""
	callerInfoLines := 0
	keep := numLines

	for callerInfoLines <= keep {
		frame, more := frames.Next()
		if shouldIncludeLine(frame.File) {
			// we want to the last part of the function name after the last /
			funcName := strings.Split(frame.Function, "/")[len(strings.Split(frame.Function, "/"))-1]
			// fileName := strings.Split(frame.File, "/")[len(strings.Split(frame.File, "/"))-1]
			callerInfo1 := fmt.Sprintf("\n\t%s:%d -  %s", frame.File, frame.Line, funcName)
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

// GetIntEnvOrDefault returns the integer value of an environment variable or the default value if not set
func GetIntEnvOrDefault(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// PrintOncef prints a message if it hasn't been printed before
func (rw *RWMutexPlus) PrintOncef(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if PrintOnce(msg) {
		rw.logger.Printf(msg)
	}
}
