package agents

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewControlledProcess(t *testing.T) {
	Convey("Given an empty command and a done channel", t, func() {
		cmd := ""
		args := []string{}
		done := make(chan error)
		Convey("When agents.NewControlledProcess is invoked", func() {
			jc, err := NewControlledProcess(cmd, args, done)
			Convey("The handle should be nil and the err is not nil", func() {
				So(jc, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})
	})
	Convey("Given a command and done channel", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "5"}
		done := make(chan error)
		Convey("When agents.NewControlledProcess is invoked", func() {
			jc, err := NewControlledProcess(cmd, args, done)
			Convey("The handle should not be nil and the error is nil", func() {
				So(jc, ShouldNotBeNil)
				So(err, ShouldBeNil)
			})
			jc, err = NewControlledProcess(cmd, args, done)
			Convey("The done channel should get a nil error when the job completes", func() {
				output := <-done
				So(output, ShouldBeNil)
			})
		})
	})
}

func TestKillProcess(t *testing.T) {
	Convey("Given a long command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "60"}
		done := make(chan error)
		Convey("When agents.NewControlledProcess is invoked", func() {
			timeStart := time.Now()
			jc, _ := NewControlledProcess(cmd, args, done)
			Convey("Calling Kill should immediately end the process", func() {
				jc.Kill(-9)
				timeEnd := time.Now()
				So(timeEnd.Unix()-timeStart.Unix(), ShouldBeLessThan, 60)
			})
		})
	})
}

func TestStopProcess(t *testing.T) {
	Convey("Given a long command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "60"}
		done := make(chan error)
		Convey("When agents.NewControlledProcess is invoked", func() {
			timeStart := time.Now()
			jc, _ := NewControlledProcess(cmd, args, done)
			Convey("Calling Stop should end the process", func() {
				jc.Stop()
				timeEnd := time.Now()
				So(timeEnd.Unix()-timeStart.Unix(), ShouldBeLessThan, 60)
			})
		})
	})
}

// TODO: For some reason this timing scheme doesn't work even though the
// TODO: process is suspended.
// func TestSuspendProcess(t *testing.T) {
// 	Convey("Given a command", t, func() {
// 		cmd := "sleep"
// 		args := []string{"sleep", "20"}
// 		done := make(chan error)
// 		Convey("When agents.NewControlledProcess is invoked", func() {
// 			timeStart := time.Now()
// 			jc, _ := NewControlledProcess(cmd, args, done)
// 			Convey("Calling Kill should immediately end the process", func() {
// 				jc.Suspend()
// 				time.Sleep(10 * time.Second)
// 				jc.Resume()
// 				_ = <-done
// 				timeEnd := time.Now()
// 				So(timeEnd.Unix()-timeStart.Unix(), ShouldBeGreaterThan, 30)
// 			})
// 		})
// 	})
// }
