# RWMutexPlus

An instrumented version of sync.RWMutex with debugging capabilities for Go applications.
It should help us spot timing and concurrency issue better.

## Author
Christoph C. Cemper

## Features

- Monitors lock wait times and emits warnings for potential deadlocks
- Tracks lock holders with function names and file locations
- Prints warnings when locks are held too long
- Prints warnings whne locks are being waited for too long
- Provides detailed stack traces when lock contention occurs
- Filters runtime noise from stack traces for better readability

## Example Output

When lock contention is detected, you'll see output like this:

```log
=== LOCK WAIT WARNING ===
Waiting for mutex lock for 102.251ms
Caller: main.processData at /app/main.go:42
Last lock holder: main.updateCache at /app/cache.go:156
Last lock time: 101.438125ms ago
Current stack trace:
goroutine 155 [running]:
main.(*processData).Lock()
    /app/main.go:42
main.updateCache()
    /app/cache.go:156
=======================
```


## Installation

```bash
go get github.com/christophcemper/rwmutexplus
```


## Usage

```
package main

import (
    "time"
    "github.com/christophcemper/rwmutexplus"
)

func main() {
    mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)
    
    // Will trigger warning after 100ms of waiting
    mutex.Lock()
    defer mutex.Unlock()
    
    // Do something...
}

```


## Running Examples

To run the included examples:

```bash
go test -v -run=Example
```

Or take a look at the example/main.go that demonstrates lock contention with multiple goroutines.

```
package main

import (
	"fmt"
	"time"

	"github.com/christophcemper/rwmutexplus"
)

func mylocker2(mutex *rwmutexplus.InstrumentedRWMutex) {
	mutex.Lock()
	fmt.Printf("mylocker2 lock acquired\n")
	time.Sleep(500 * time.Millisecond) // Hold lock for 500ms
	mutex.Unlock()
	fmt.Printf("mylocker2 lock released\n")
}

func main() {

	mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

	// Create lock contention*
	mylocker1 := func() {
		mutex.Lock()
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
	mutex.Lock()
	fmt.Printf("Main Lock acquired\n")
	time.Sleep(50 * time.Millisecond) // pretend  to add some work in critical section
	mutex.Unlock()
	fmt.Printf("Main Lock released\n")

}

```

The output of the above [examples/main.go](examples/main.go) will be something like this

`go run -race  example/main.go`
```

mylocker2 lock acquired

=== LONG LOCK HOLD WARNING ===
Still waiting in mutex lock for 101.18725ms
Too long lock holder: main.mylocker2
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:11 main.mylocker2
Last lock time: 101.301375ms ago
========================


=== LOCK WAIT TIMEOUT WARNING ===
Waiting for mutex lock for 100.57475ms
Caller: main.main.func1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 main.main.func1
Last lock holder: main.mylocker2
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:11 main.mylocker2
Last lock time: 111.576667ms ago
Current stack trace:
goroutine 33 [running]:
main.main.func1()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 +0x30
main.main.func3()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:38 +0x34
created by main.main in goroutine 1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:37 +0x1f8
========================


=== LOCK WAIT TIMEOUT WARNING ===
Waiting for mutex lock for 100.830541ms
Caller: main.main
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:44 main.main
Last lock holder: main.mylocker2
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:11 main.mylocker2
Last lock time: 122.719584ms ago
Current stack trace:
goroutine 1 [running]:
main.main()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:44 +0x20c
========================


=== LOCK HELD TOO LONG! ===
Been holding mutex lock for 501.099625ms
Caller: main.mylocker2
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:11 main.mylocker2
Last lock time: 501.2425ms ago
Current stack trace:
goroutine 18 [running]:
main.mylocker2(0xc000060798?)
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:14 +0x7c
main.main.func2(0x0?)
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:32 +0x2c
created by main.main in goroutine 1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:31 +0x188
========================

mylocker2 lock released

=== LOCK ACQUIRED TOO SLOW ===
Been waiting for mutex lock for 490.646958ms
Caller: main.main.func1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 main.main.func1
Last lock holder: main.mylocker2
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:11 main.mylocker2
Last lock time: 501.533292ms ago
Current stack trace:
goroutine 33 [running]:
main.main.func1()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 +0x30
main.main.func3()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:38 +0x34
created by main.main in goroutine 1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:37 +0x1f8
========================

mylocker1 lock acquired

=== LOCK HELD TOO LONG! ===
Been holding mutex lock for 301.126916ms
Caller: main.main.func1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 main.main.func1
Last lock time: 301.201208ms ago
Current stack trace:
goroutine 33 [running]:
main.main.func1()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:27 +0x80
main.main.func3()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:38 +0x34
created by main.main in goroutine 1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:37 +0x1f8
========================

mylocker1 lock released

=== LOCK ACQUIRED TOO SLOW ===
Been waiting for mutex lock for 781.077708ms
Caller: main.main
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:44 main.main
Last lock holder: main.main.func1
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:24 main.main.func1
Last lock time: 301.303916ms ago
Current stack trace:
goroutine 1 [running]:
main.main()
        /Users/christophc/Workspace/common/CaddyLRT/rwmutexplus/examples/main.go:44 +0x20c
========================

Main Lock acquired
Main Lock released
```

This example demonstrates how

* mylocker2 is holding the RW lock way too long (500ms)
* mylocker1 is getting warnings and has to wait of course, but STILL also holds it for too long - 300ms ("LOCK ACQUIRED TOO SLOW", then "LOCK HELD TOO LONG")
* the poor main function trying to acquired the RW lock has to wait a total of 781ms ("LOCK ACQUIRED TOO SLOW")


## License

MIT License. See LICENSE file for details.

## Disclaimer

This software is provided "AS IS", without warranty of any kind. The author shall not be liable for any claim, damages or other liability arising from the use of the software.
