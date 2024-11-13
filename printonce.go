package rwmutexplus

import (
	"fmt"
	"sync"
)

var (
	// Global map to track printed messages
	printedMessages sync.Map
)

// PrintOnce prints a message only once during the process lifetime
// Returns true if the message was printed, false if it was already printed before
func PrintOnce(msg string) bool {
	_, loaded := printedMessages.LoadOrStore(msg, true)
	return !loaded
}

// PrintOncef formats and prints a message only once during the process lifetime
// Returns true if the message was printed, false if it was already printed before
func PrintOncef(format string, args ...interface{}) bool {
	msg := fmt.Sprintf(format, args...)
	return PrintOnce(msg)
}

// ResetPrintOnce clears all tracked messages (mainly for testing)
func ResetPrintOnce() {
	printedMessages = sync.Map{}
}
