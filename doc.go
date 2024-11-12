/*
Package rwmutexplus provides an instrumented version of sync.RWMutex that helps
detect potential deadlocks and lock contention issues in Go applications.

Key Features:
  - Monitors lock wait times and emits warnings for potential deadlocks
  - Tracks lock holders with function names and file locations
  - Provides detailed stack traces when lock contention occurs
  - Filters runtime noise from stack traces for better readability

Basic Usage:

	mutex := rwmutexplus.NewInstrumentedRWMutex(time.Second)

	// Use like a regular RWMutex
	mutex.Lock()
	// ... critical section ...
	mutex.Unlock()

	mutex.RLock()
	// ... read-only critical section ...
	mutex.RUnlock()

The mutex will automatically emit warnings to stdout when locks are held longer
than the specified timeout, including information about the current lock holder
and stack traces to help diagnose the issue.

Warning Format:
When a lock wait timeout occurs, the output includes:
  - Wait duration
  - Caller information (file and line)
  - Last lock holder details
  - Filtered stack trace focusing on application code

This package is particularly useful during development and testing to identify
potential deadlocks and performance issues related to lock contention.
*/
package rwmutexplus
