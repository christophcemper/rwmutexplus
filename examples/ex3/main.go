package main

import (
	"fmt"
	"time"

	"github.com/christophcemper/rwmutexplus"
)

func mylocker2(mutex *rwmutexplus.RWMutexPlus) {
	defer rwmutexplus.ReleaseGoroutineID()
	// Changed Lock() to LockWithPurpose()
	mutex.LockWithPurpose("mylocker2")
	fmt.Printf("mylocker2 lock acquired %d \n %v\n", rwmutexplus.GetGoroutineID(), mutex.GetActiveLocksInfo())
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("mylocker2 TRY TO UNLOCK %d \n %v\n", rwmutexplus.GetGoroutineID(), mutex.GetActiveLocksInfo())
	mutex.Unlock()
	fmt.Printf("mylocker2 lock released %d \n %v\n", rwmutexplus.GetGoroutineID(), mutex.GetActiveLocksInfo())
}

func main() {
	mutex := rwmutexplus.NewRWMutexPlus(
		"main mutex",
		100*time.Millisecond,
		nil,
	).WithDebugLevel(0).WithVerboseLevel(0).WithCallerInfoLines(5)

	mutex.PrintLockInfo("pos1")

	mylocker1 := func() {
		// Changed Lock() to LockWithPurpose()
		mutex.LockWithPurpose("mylocker1")
		fmt.Printf("mylocker1 lock acquired\n")
		time.Sleep(300 * time.Millisecond)
		mutex.Unlock()
		fmt.Printf("mylocker1 lock released\n")
	}

	go mylocker2(mutex) // Simplified goroutine syntax

	time.Sleep(10 * time.Millisecond)
	mutex.PrintLockInfo("pos2")

	go mylocker1() // Simplified goroutine syntax

	mutex.PrintLockInfo("pos3")

	time.Sleep(10 * time.Millisecond)

	// Changed Lock() to LockWithPurpose()
	mutex.LockWithPurpose("main")
	fmt.Printf("Main Lock acquired\n")
	time.Sleep(50 * time.Millisecond)
	mutex.Unlock()
	fmt.Printf("Main Lock released\n")

	mutex.PrintLockInfo("back to main - pos4")

	mutex.DebugPrintAllLockInfo("back to main - pos5")

	// Wait for goroutines to complete
	time.Sleep(1 * time.Second)
}
