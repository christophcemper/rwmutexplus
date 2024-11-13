// registry_test.go
package rwmutexplus

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	// Create multiple mutexes
	mutex1 := NewRWMutexPlus("mutex1", 50*time.Millisecond, nil)
	mutex2 := NewRWMutexPlus("mutex2", 50*time.Millisecond, nil)

	// Verify registration
	mutexes := globalRegistry.getAllMutexes()
	if len(mutexes) != 2 {
		t.Errorf("Expected 2 registered mutexes, got %d", len(mutexes))
	}

	// Verify mutex names are registered correctly
	names := make(map[string]bool)
	for _, m := range mutexes {
		names[m.name] = true
	}
	if !names["mutex1"] || !names["mutex2"] {
		t.Error("Not all expected mutex names were registered")
	}

	// Verify unregistration
	mutex1.Close()
	mutexes = globalRegistry.getAllMutexes()
	if len(mutexes) != 1 {
		t.Errorf("Expected 1 registered mutex after Close(), got %d", len(mutexes))
	}
	if mutexes[0].name != "mutex2" {
		t.Errorf("Expected remaining mutex to be 'mutex2', got '%s'", mutexes[0].name)
	}

	mutex2.Close()
	mutexes = globalRegistry.getAllMutexes()
	if len(mutexes) != 0 {
		t.Errorf("Expected 0 registered mutexes after Close(), got %d", len(mutexes))
	}
}
func TestDumpAllLockInfo(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	// Create test mutexes with different states
	mutex1 := NewRWMutexPlus("test-mutex-1", 50*time.Millisecond, nil)
	mutex2 := NewRWMutexPlus("test-mutex-2", 50*time.Millisecond, nil)

	var wg sync.WaitGroup

	// Test scenario 1: Active write lock on mutex1
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex1.LockWithPurpose("active-write")
		time.Sleep(100 * time.Millisecond)
		mutex1.Unlock()
	}()

	// Wait for the write lock to be acquired
	time.Sleep(10 * time.Millisecond)

	// Test scenario 2: Pending write lock on mutex1
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex1.LockWithPurpose("pending-write")
		time.Sleep(10 * time.Millisecond)
		mutex1.Unlock()
	}()

	// Test scenario 3: Active read locks on mutex2
	readWg := sync.WaitGroup{}
	readWg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer readWg.Done()
			mutex2.RLockWithPurpose("active-read")
			time.Sleep(100 * time.Millisecond)
			mutex2.RUnlock()
		}()
	}

	// Give time for locks to settle
	time.Sleep(20 * time.Millisecond)

	// Get the dump output
	output := DumpAllLockInfo()

	fmt.Println("--------------------------------")
	fmt.Println("--------------------------------")
	fmt.Println("------ ALL LOCK INFO DUMP ------")
	fmt.Println("--------------------------------")
	fmt.Println(output)
	fmt.Println("--------------------------------")
	fmt.Println("--------------------------------")
	fmt.Println("--------------------------------")

	// Verify dump contains expected information
	expectedStrings := []string{
		"Total Registered Mutexes: 2",
		"test-mutex-1",
		"test-mutex-2",
		"Pending Writes",
		"Active Writes",
		"Active Reads",
		"Purpose 'active-write'",
		"Purpose 'active-read'",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected dump to contain '%s'", expected)
		}
	}

	// Wait for all operations to complete
	wg.Wait()
	readWg.Wait()

	// Clean up
	mutex1.Close()
	mutex2.Close()
}

func TestDumpAllLockInfoWithFilters(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	mutex1 := NewRWMutexPlus("test-mutex-1", 50*time.Millisecond, nil)
	mutex2 := NewRWMutexPlus("test-mutex-2", 50*time.Millisecond, nil)

	var wg sync.WaitGroup

	// Setup test scenario
	// 1. Active write lock on mutex1
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex1.LockWithPurpose("active-write")
		time.Sleep(200 * time.Millisecond)
		mutex1.Unlock()
	}()
	time.Sleep(10 * time.Millisecond)

	// 2. Pending write lock on mutex1
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex1.LockWithPurpose("pending-write")
		time.Sleep(10 * time.Millisecond)
		mutex1.Unlock()
	}()

	// 3. Active read locks on mutex2
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mutex2.RLockWithPurpose("active-read")
			time.Sleep(100 * time.Millisecond)
			mutex2.RUnlock()
		}()
	}

	// Give time for locks to settle
	time.Sleep(20 * time.Millisecond)

	// Test different filter combinations
	testCases := []struct {
		name        string
		filters     []LockFilter
		expected    []string
		notExpected []string
	}{
		{
			name:        "Only Pending Writes",
			filters:     []LockFilter{ShowPendingWrites},
			expected:    []string{"Pending Writes", "pending-write"},
			notExpected: []string{"Active Writes", "Active Reads", "Pending Reads"},
		},
		{
			name:        "Active Locks Only",
			filters:     []LockFilter{ShowActiveReads, ShowActiveWrites},
			expected:    []string{"Active Reads", "Active Writes", "active-write", "active-read"},
			notExpected: []string{"Pending Writes", "Pending Reads"},
		},
		{
			name:        "All Read Operations",
			filters:     []LockFilter{ShowPendingReads, ShowActiveReads},
			expected:    []string{"Active Reads", "active-read"},
			notExpected: []string{"Active Writes", "Pending Writes"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := DumpAllLockInfo(tc.filters...)

			fmt.Println("--------------------------------")
			fmt.Println("--------------------------------")
			fmt.Printf("\n--- LOCK INFO DUMP %s ---", tc.name)
			fmt.Println("--------------------------------")
			fmt.Println(output)
			fmt.Println("--------------------------------")
			fmt.Println("--------------------------------")
			fmt.Println("--------------------------------")

			// Check expected strings are present
			for _, exp := range tc.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("Expected output to contain '%s', but it didn't.\nOutput: %s", exp, output)
				}
			}

			// Check unexpected strings are absent
			for _, notExp := range tc.notExpected {
				if strings.Contains(output, notExp) {
					t.Errorf("Expected output to NOT contain '%s', but it did.\nOutput: %s", notExp, output)
				}
			}
		})
	}

	// Wait for all operations to complete
	wg.Wait()

	// Clean up
	mutex1.Close()
	mutex2.Close()
}

func TestDumpAllLockInfoNoFilters(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	mutex := NewRWMutexPlus("test-mutex", 50*time.Millisecond, nil)

	// Generate some lock activity
	mutex.LockWithPurpose("test-write")

	output := DumpAllLockInfo() // No filters = show everything

	expectedStrings := []string{
		"Active Writes",
		"test-write",
		"Active Filters: PendingReads, PendingWrites, ActiveReads, ActiveWrites",
	}

	for _, exp := range expectedStrings {
		if !strings.Contains(output, exp) {
			t.Errorf("Expected output to contain '%s', but it didn't", exp)
		}
	}

	mutex.Unlock()
	mutex.Close()
}

func TestRegistryConcurrency(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	var wg sync.WaitGroup
	// Create and close mutexes concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("mutex-%d", i)
			mutex := NewRWMutexPlus(name, 50*time.Millisecond, nil)
			time.Sleep(time.Millisecond)
			mutex.Close()
		}(i)
	}

	// Concurrently read registry while modifications are happening
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = DumpAllLockInfo()
		}()
	}

	wg.Wait()
}

func TestRegistryDuplicateNames(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	// Create mutex with the same name
	mutex1 := NewRWMutexPlus("same-name", 50*time.Millisecond, nil)
	mutex2 := NewRWMutexPlus("same-name", 50*time.Millisecond, nil)

	// Verify only the latest registration is kept
	mutexes := globalRegistry.getAllMutexes()
	if len(mutexes) != 1 {
		t.Errorf("Expected 1 registered mutex with duplicate names, got %d", len(mutexes))
	}

	// Cleanup
	mutex1.Close()
	mutex2.Close()
}

func TestRegistryEmptyDump(t *testing.T) {
	// Clear any existing registrations
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}

	output := DumpAllLockInfo()

	if !strings.Contains(output, "Total Registered Mutexes: 0") {
		t.Error("Expected empty registry dump to show zero mutexes")
	}
}
