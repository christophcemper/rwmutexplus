package rwmutexplus

import (
	"strings"
	"testing"
	"time"
)

func TestNewInstrumentedRWMutex(t *testing.T) {
	timeout := 100 * time.Millisecond
	mutex := NewInstrumentedRWMutex(timeout)

	if mutex.lockWaitTimeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, mutex.lockWaitTimeout)
	}
}

func TestLockWarning(t *testing.T) {
	mutex := NewInstrumentedRWMutex(50 * time.Millisecond)

	// Lock the mutex in a goroutine and hold it
	go func() {
		mutex.Lock()
		time.Sleep(200 * time.Millisecond)
		mutex.Unlock()
	}()

	// Wait a bit for the goroutine to acquire the lock
	time.Sleep(10 * time.Millisecond)

	// Try to lock and verify warning
	t.Log("Locking")
	mutex.Lock()
	t.Log("Unlocked")
	mutex.Unlock()
}

func TestRLockWarning(t *testing.T) {
	mutex := NewInstrumentedRWMutex(50 * time.Millisecond)

	// Lock the mutex in a goroutine and hold it
	go func() {
		mutex.Lock()
		time.Sleep(200 * time.Millisecond)
		mutex.Unlock()
	}()

	// Wait a bit for the goroutine to acquire the lock
	time.Sleep(10 * time.Millisecond)

	// Try to RLock and verify warning
	mutex.RLock()
	time.Sleep(50 * time.Millisecond) // pretend to add some work in critical section
	mutex.RUnlock()
}

func TestGetCallerInfo(t *testing.T) {
	info := getCallerInfo()

	// Verify it contains the test function name
	if !strings.Contains(info, "TestGetCallerInfo") {
		t.Errorf("Expected caller info to contain TestGetCallerInfo, got %s", info)
	}

	// Verify it contains file and line information
	if !strings.Contains(info, "rwmutexplus_test.go") {
		t.Errorf("Expected caller info to contain rwmutexplus_test.go, got %s", info)
	}
}

func TestFilterStack(t *testing.T) {
	stack := `goroutine 1 [running]:
runtime.example()
	/usr/local/go/src/runtime/debug.go:123
testing.tRunner()
	/usr/local/go/src/testing/testing.go:456
main.actualFunction()
	/path/to/main.go:789`

	filtered := filterStack([]byte(stack))

	// Verify runtime and testing lines are removed
	if strings.Contains(filtered, "runtime.") {
		t.Error("Filtered stack should not contain runtime functions")
	}
	if strings.Contains(filtered, "testing.") {
		t.Error("Filtered stack should not contain testing functions")
	}

	// Verify actual function remains
	if !strings.Contains(filtered, "main.actualFunction") {
		t.Error("Filtered stack should contain actual functions")
	}
}

// Example of concurrent usage
func ExampleInstrumentedRWMutex() {
	mutex := NewInstrumentedRWMutex(100 * time.Millisecond)
	data := make(map[string]int)

	// Writer
	go func() {
		mutex.Lock()
		data["key"] = 42
		time.Sleep(200 * time.Millisecond) // Simulate work
		mutex.Unlock()
	}()

	// Reader
	go func() {
		mutex.RLock()
		_ = data["key"]
		mutex.RUnlock()
	}()

	// Let the example run
	time.Sleep(3000 * time.Millisecond)
}
