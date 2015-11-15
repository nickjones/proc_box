package agents

import (
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	timer, err := NewTimer(2 * time.Second)
	if err != nil {
		t.Error("Error creating new timer")
	}
	// Start the timer.
	timer.Start()
	// Block until done received
	timeout := make(chan bool, 1)
	// Wait 3 sec and see if we get a timeout at 2 sec.
	go func() {
		time.Sleep(3 * time.Second)
		timeout <- true
	}()
	select {
	case <-timer.Done():
		// timer timed out.
		// Test passes
	case <-timeout:
		t.Error("Timeout from timer not received")
	}
}
