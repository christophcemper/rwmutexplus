package main

import (
	"fmt"
	"time"

	"github.com/christophcemper/rwmutexplus"
)

func mylocker2(mutex *rwmutexplus.InstrumentedRWMutex) {
	mutex.Lock() // Acquire lock but do not set purpose
	// mutex.LockPurpose("mylocker2")
	fmt.Printf("mylocker2 lock acquired\n")
	time.Sleep(500 * time.Millisecond) // Hold lock for 500ms
	mutex.Unlock()
	fmt.Printf("mylocker2 lock released\n")
}

func main() {

	mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

	// Create lock contention*
	mylocker1 := func() {
		mutex.LockPurpose("mylocker1")
		fmt.Printf("mylocker1 lock acquired\n")
		time.Sleep(300 * time.Millisecond) // Hold lock for 300ms
		mutex.Unlock()
		fmt.Printf("mylocker1 lock released\n")
	}

	go func(mutex *rwmutexplus.InstrumentedRWMutex) {
		mylocker2(mutex)
	}(mutex)

	time.Sleep(10 * time.Millisecond) // Let goroutine acquire lock

	go func() {
		mylocker1()
	}()

	time.Sleep(10 * time.Millisecond) // Let goroutine acquire lock

	// This will trigger a warning as the goroutine is holding the lock still
	mutex.LockPurpose("main")
	fmt.Printf("Main Lock acquired\n")
	time.Sleep(50 * time.Millisecond) // pretend  to add some work in critical section
	mutex.Unlock()
	fmt.Printf("Main Lock released\n")

}
