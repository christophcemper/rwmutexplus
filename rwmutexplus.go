// Copyright (c) 2024 Christoph C. Cemper
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package rwmutexplus

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// InstrumentedRWMutex wraps sync.RWMutex with debugging capabilities.
// It tracks lock holders, wait times, and can emit warnings when locks are held for too long.
type InstrumentedRWMutex struct {
	sync.RWMutex
	lastLockTime    time.Time     // Time when the last lock was acquired
	lastLockHolder  string        // Information about the last lock holder (function and file location)
	lockWaitTimeout time.Duration // Duration after which a warning is emitted if lock isn't acquired
	purpose         string        // Purpose of the mutex
	debugMutex      sync.Mutex    // Protects our debug fields from concurrent access
}

// NewInstrumentedRWMutex creates a new InstrumentedRWMutex with the specified lock wait timeout.
// The timeout determines how long to wait before emitting a warning about potential deadlocks.
//
// Example:
//
//	mutex := NewInstrumentedRWMutex(time.Second) // Warn after 1 second of waiting
func NewInstrumentedRWMutex(lockWaitTimeout time.Duration) *InstrumentedRWMutex {
	return &InstrumentedRWMutex{
		lockWaitTimeout: lockWaitTimeout,
	}
}

// LockPurpose acquires an exclusive lock and sets the purpose of the mutex and monitors the wait time.
func (m *InstrumentedRWMutex) LockPurpose(purpose string) {
	m.internalLock(purpose)
}

// Lock acquires an exclusive lock and monitors the wait time.
func (m *InstrumentedRWMutex) Lock() {
	m.internalLock("unknown")
}

// Lock acquires an exclusive lock and monitors the wait time.
// If the wait exceeds lockWaitTimeout, it prints a warning with stack trace and lock holder information
// If the lock is still held after lockWaitTimeout, it prints a warning that the lock is still held
func (m *InstrumentedRWMutex) internalLock(purpose string) {
	lockCaller := getCallerInfo0()
	waitStart := time.Now()
	stackTrace := debug.Stack() // Capture stack trace BEFORE the goroutine

	// go func() {
	// 	time.Sleep(m.lockWaitTimeout)
	// 	m.debugMutex.Lock()

	// 	if m.lastLockTime.Add(m.lockWaitTimeout).Before(time.Now()) {
	// 		waitDuration := time.Since(waitStart)
	// 		if m.lastLockHolder != lockCaller {
	// 			fmt.Printf("\n=== LOCK WAIT TIMEOUT WARNING ===\n")
	// 			fmt.Printf("Waiting for mutex lock for %v\n", waitDuration)
	// 			fmt.Printf("Purpose: %s\n", purpose)
	// 			fmt.Printf("Caller: %s\n", lockCaller)
	// 			fmt.Printf("Last lock holder: %s\n", m.lastLockHolder)
	// 			fmt.Printf("Last lock time: %v ago\n", time.Since(m.lastLockTime))
	// 			fmt.Printf("Current stack trace:\n%s", filterStack(stackTrace))
	// 			fmt.Printf("========================\n\n")
	// 		} else {
	// 			fmt.Printf("\n=== LONG LOCK HOLD WARNING ===\n")
	// 			fmt.Printf("Still waiting in mutex lock for %v\n", waitDuration)
	// 			fmt.Printf("Purpose: %s\n", purpose)
	// 			fmt.Printf("Too long lock holder: %s\n", m.lastLockHolder)
	// 			fmt.Printf("Last lock time: %v ago\n", time.Since(m.lastLockTime))
	// 			fmt.Printf("Too long holder stack trace:\n%s", filterStack(stackTrace))
	// 			fmt.Printf("========================\n\n")
	// 		}
	// 	}
	// 	m.debugMutex.Unlock()
	// }()

	m.debugMutex.Lock()

	m.RWMutex.Lock()

	m.purpose = purpose
	waitDuration := time.Since(waitStart)
	if waitDuration > m.lockWaitTimeout {
		fmt.Printf("\n=== LOCK ACQUIRED TOO SLOW ===\n")
		fmt.Printf("Been waiting for mutex lock for %v\n", waitDuration)
		fmt.Printf("Purpose: %s\n", purpose)
		fmt.Printf("Caller: %s\n", lockCaller)
		fmt.Printf("Last lock holder: %s\n", m.lastLockHolder)
		fmt.Printf("Last lock time: %v ago\n", time.Since(m.lastLockTime))
		fmt.Printf("Current stack trace:\n%s", filterStack(stackTrace))
		fmt.Printf("========================\n\n")
	}

	m.lastLockTime = time.Now()
	m.lastLockHolder = getCallerInfo0()
	m.debugMutex.Unlock()
}

// Unlock releases an exclusive lock.
// if the lock was held for too long, it prints a warning
func (m *InstrumentedRWMutex) Unlock() {

	waitDuration := time.Since(m.lastLockTime)
	if waitDuration > m.lockWaitTimeout {
		fmt.Printf("\n=== LOCK HELD TOO LONG ===\n")
		fmt.Printf("Been holding mutex lock for %v\n", waitDuration)
		fmt.Printf("Purpose: %s\n", m.purpose)
		fmt.Printf("Caller: %s\n", m.lastLockHolder)
		fmt.Printf("Last lock time: %v ago\n", time.Since(m.lastLockTime))
		fmt.Printf("Current stack trace:\n%s", filterStack(debug.Stack()))
		fmt.Printf("========================\n\n")
	}

	m.RWMutex.Unlock()
}

// RLock acquires a shared read lock and monitors the wait time.
// Similar to Lock, it will emit warnings if the wait time exceeds lockWaitTimeout.
func (m *InstrumentedRWMutex) RLock() {
	waitStart := time.Now()

	done := make(chan bool)
	go func() {
		select {
		case <-done:
			return
		case <-time.After(m.lockWaitTimeout):
			_, file, line, _ := runtime.Caller(2)

			m.debugMutex.Lock()
			waitDuration := time.Since(waitStart)
			lastHolder := m.lastLockHolder
			lastTime := m.lastLockTime
			m.debugMutex.Unlock()

			fmt.Printf("\n=== RLOCK WAIT WARNING ===\n")
			fmt.Printf("Waiting for mutex read lock for %v\n", waitDuration)
			fmt.Printf("Caller: %s:%d\n", file, line)
			fmt.Printf("Last lock holder: %s\n", lastHolder)
			fmt.Printf("Last lock time: %v ago\n", time.Since(lastTime))
			fmt.Printf("Current stack trace:\n%s", filterStack(debug.Stack()))
			fmt.Printf("========================\n\n")
		}
	}()

	m.RWMutex.RLock()
	close(done)
}

// // shouldIncludeLine returns true if the line should be included in stack traces
// // and caller information, filtering out runtime, testing, and debug-related frames.
// func shouldIncludeLine(line string) bool {

// 	// we want to see the test code in the stack trace
// 	if strings.Contains(line, "rwmutexplus_test.go") ||
// 		strings.Contains(line, "rwmutexplus/examples") {
// 		return true
// 	}
// 	return !strings.Contains(line, "runtime/") &&
// 		!strings.Contains(line, "runtime.") &&
// 		!strings.Contains(line, "testing/") &&
// 		!strings.Contains(line, "testing.") &&
// 		!strings.Contains(line, "rwmutexplus") &&
// 		!strings.Contains(line, "debug.Stack") &&
// 		!strings.Contains(line, "debug/stack")
// }

// // filterStack removes unnecessary runtime and testing lines from stack traces
// // to make the output more readable and focused on application code.
// func filterStack(stack []byte) string {
// 	lines := strings.Split(string(stack), "\n")
// 	var filtered []string
// 	for i, line := range lines {
// 		if shouldIncludeLine(line) {
// 			filtered = append(filtered, lines[i])
// 		}
// 	}
// 	return strings.Join(filtered, "\n")
// }

// // getCallerInfo returns the caller information for the function that called Lock or RLock.
// // It skips the runtime, testing, and debug frames to focus on application code.
// // except for tests where we want to see the test code in the stack trace
// func getCallerInfo0() string {
// 	var pcs [32]uintptr
// 	n := runtime.Callers(0, pcs[:]) // Increase skip count to 3 to get past runtime frames
// 	frames := runtime.CallersFrames(pcs[:n])

// 	// Get the first non-runtime frame
// 	for {
// 		frame, more := frames.Next()
// 		if shouldIncludeLine(frame.File) {
// 			// we want to the last part of the function name after the last /
// 			funcName := strings.Split(frame.Function, "/")[len(strings.Split(frame.Function, "/"))-1]
// 			// fileName := strings.Split(frame.File, "/")[len(strings.Split(frame.File, "/"))-1]
// 			return fmt.Sprintf("%s\n\t%s:%d %s", funcName, frame.File, frame.Line, funcName)
// 		}
// 		if !more {
// 			break
// 		}
// 	}
// 	return "unknown"
// }
