package rwmutexplus_test

import (
	"fmt"
	"time"

	"github.com/christophcemper/rwmutexplus"
)

func Example_lockContention() {
	mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

	fmt.Println("Starting lock contention example...")

	// Create lock contention by holding the lock in a goroutine
	go func() {
		mutex.Lock()
		fmt.Println("Goroutine acquired lock, holding for 200ms...")
		time.Sleep(200 * time.Millisecond)
		mutex.Unlock()
		fmt.Println("Goroutine released lock")
	}()

	// Wait for goroutine to acquire lock
	time.Sleep(10 * time.Millisecond)

	// This will trigger a warning after 100ms
	fmt.Println("Main thread attempting to acquire lock...")
	mutex.Lock()
	fmt.Println("Main thread acquired lock")
	mutex.Unlock()
	fmt.Println("Main thread released lock")

	// Output:
	// Starting lock contention example...
	// Goroutine acquired lock, holding for 200ms...
	// Main thread attempting to acquire lock...
	// === LOCK WAIT WARNING ===
	// Waiting for mutex lock for ...
	// Last lock holder: ...
	// ========================
	// Goroutine released lock
	// Main thread acquired lock
	// Main thread released lock
}

func Example_readLockContention() {
	mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

	fmt.Println("Starting read lock contention example...")

	// Hold write lock
	go func() {
		mutex.Lock()
		fmt.Println("Write lock acquired, holding for 200ms...")
		time.Sleep(200 * time.Millisecond)
		mutex.Unlock()
		fmt.Println("Write lock released")
	}()

	// Wait for write lock to be acquired
	time.Sleep(10 * time.Millisecond)

	// This will trigger a warning
	fmt.Println("Attempting to acquire read lock...")
	mutex.RLock()
	fmt.Println("Read lock acquired")
	mutex.RUnlock()
	fmt.Println("Read lock released")

	// Output:
	// Starting read lock contention example...
	// Write lock acquired, holding for 200ms...
	// Attempting to acquire read lock...
	// Write lock released
	// Read lock acquired
	// Read lock released
}
