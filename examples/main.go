package main

import (
	"time"

	"github.com/christophcemper/rwmutexplus"
)

func main() {

	mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

	// Create lock contention*
	go func() {
		mutex.Lock()
		time.Sleep(200 * time.Millisecond) // Hold lock for 200ms
		mutex.Unlock()

	}()

	time.Sleep(10 * time.Millisecond) // Let goroutine acquire lock

	// This will trigger a warning as the goroutine is holding the lock still
	mutex.Lock()
	time.Sleep(50 * time.Millisecond) // pretend  to add some work in critical section
	mutex.Unlock()

}
