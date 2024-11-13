package main

import (
	"fmt"
	"sync"
	"time"

	. "github.com/christophcemper/rwmutexplus"
)

func main() {
	var wg sync.WaitGroup

	// Launch multiple goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer ReleaseGoroutineID()
			id := GetGoroutineID()
			fmt.Printf("Goroutine %d got ID: %d\n", i, id)

			// Verify ID stays consistent within goroutine
			time.Sleep(10 * time.Millisecond)
			id2 := GetGoroutineID()
			if id != id2 {
				fmt.Printf("Warning: ID changed from %d to %d\n", id, id2)
			}
		}(i)
	}

	wg.Wait()

	// Cleanup when done
	CleanupGoroutineIDs()
	fmt.Printf("Remaining stored IDs: %d\n", GetStoredGoroutineCount())
}
