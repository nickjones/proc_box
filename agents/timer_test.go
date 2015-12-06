package agents

import (
	"testing"
	"time"

	. "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/smartystreets/goconvey/convey"
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

func TestReset(t *testing.T) {
	Convey("Given a timer instance", t, func() {
		timer, _ := NewTimer(30 * time.Second)
		Convey("It should return an ElapsedTime of 0 if Reset", func() {
			timer.Start()
			time.Sleep(3 * time.Second)
			timer.Stop()
			timer.Reset()
			time.Sleep(1 * time.Second)
			elapsed, err := timer.ElapsedTime()
			So(elapsed, ShouldEqual, 0)
			So(err, ShouldBeNil)
		})
	})
}

func TestResume(t *testing.T) {
	Convey("Given a timer instance", t, func() {
		timer, _ := NewTimer(5 * time.Second)
		Convey("It should take longer than 5 seconds to timeout if it was stopped and resumed", func() {
			timeStart := time.Now()
			timer.Start()
			timer.Stop()
			time.Sleep(2 * time.Second)
			timer.Resume()
			_ = <-timer.Done()
			timeEnd := time.Now()
			So(timeEnd.Unix()-timeStart.Unix(), ShouldBeGreaterThan, 6)
			elapsed, err := timer.ElapsedTime()
			So(err, ShouldBeNil)
			So(elapsed, ShouldAlmostEqual, 5*time.Second)
		})
	})
}
