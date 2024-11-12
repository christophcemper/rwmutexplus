# RWMutexPlus

An instrumented version of sync.RWMutex with debugging capabilities for Go applications.

## Author
Christoph C. Cemper

## Features

- Monitors lock wait times and emits warnings for potential deadlocks
- Tracks lock holders with function names and file locations
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

Or create your own example that demonstrates lock contention:

```
package main

import (

  "time"

  "github.com/christophcemper/rwmutexplus"

)

func main() {

  mutex := rwmutexplus.NewInstrumentedRWMutex(100 * time.Millisecond)

  

  *// Create lock contention*

  go func() {

​    mutex.Lock()

​    time.Sleep(200 * time.Millisecond) *// Hold lock for 200ms*

​    mutex.Unlock()

  }()

  

  time.Sleep(10 * time.Millisecond) *// Let goroutine acquire lock*

  

  *// This will trigger a warning*

  mutex.Lock()

  mutex.Unlock()

}
```

## License

MIT License. See LICENSE file for details.

## Disclaimer

This software is provided "AS IS", without warranty of any kind. The author shall not be liable for any claim, damages or other liability arising from the use of the software.
