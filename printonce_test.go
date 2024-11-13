package rwmutexplus

import (
	"testing"
)

func TestPrintOnce(t *testing.T) {
	ResetPrintOnce()

	// First time should return true
	if !PrintOnce("test message") {
		t.Error("First PrintOnce should return true")
	}

	// Second time should return false
	if PrintOnce("test message") {
		t.Error("Second PrintOnce should return false")
	}

	// Different message should return true
	if !PrintOnce("different message") {
		t.Error("Different message should return true")
	}
}

func TestPrintOncef(t *testing.T) {
	ResetPrintOnce()

	// First time should return true
	if !PrintOncef("test %s %d", "message", 1) {
		t.Error("First PrintOncef should return true")
	}

	// Same format and args should return false
	if PrintOncef("test %s %d", "message", 1) {
		t.Error("Second PrintOncef should return false")
	}

	// Different format should return true
	if !PrintOncef("different %s", "message") {
		t.Error("Different format should return true")
	}
}

func TestResetPrintOnce(t *testing.T) {
	// Print a message
	PrintOnce("test message")

	// Reset
	ResetPrintOnce()

	// Should return true after reset
	if !PrintOnce("test message") {
		t.Error("PrintOnce should return true after reset")
	}
}
