package agents

import (
	"testing"
	"time"

	"github.com/shirou/gopsutil/process"
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
	// Provides coverage of stdRedirect
	Convey("Stdout should be printed normally", t, func() {
		cmd := "echo"
		args := []string{"echo", "Hello testing"}
		done := make(chan error)
		_, _ = NewControlledProcess(cmd, args, done)
	})
}

func TestGetDoneChannel(t *testing.T) {
	Convey("Given a command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "5"}
		done := make(chan error)
		Convey("Calling Done() on the returned JobControl should return the done channel", func() {
			jc, _ := NewControlledProcess(cmd, args, done)
			So(jc, ShouldNotBeNil)
			So(done, ShouldEqual, jc.Done())
		})
	})
}

func TestGetProcessHandle(t *testing.T) {
	Convey("Given a command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "5"}
		done := make(chan error)
		Convey("Process() should return a non-nil handle to the process data", func() {
			jc, _ := NewControlledProcess(cmd, args, done)
			proc := jc.Process()
			var testProc *process.Process
			So(proc, ShouldNotBeNil)
			So(proc, ShouldHaveSameTypeAs, testProc)
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
	Convey("Given a short command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "1"}
		done := make(chan error)
		jc, _ := NewControlledProcess(cmd, args, done)
		Convey("Calling Kill after it completes should return an error", func() {
			time.Sleep(2 * time.Second)
			err := jc.Kill(15)
			So(err, ShouldNotBeNil)
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
	Convey("Given a short command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "1"}
		done := make(chan error)
		jc, _ := NewControlledProcess(cmd, args, done)
		Convey("When Stop() is called after it completes should return an error", func() {
			time.Sleep(2 * time.Second)
			err := jc.Stop()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSuspendProcess(t *testing.T) {
	Convey("Given a command", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "5"}
		done := make(chan error)
		Convey("Calling Suspend() should pause execution of the process and take longer to finish", func() {
			timeStart := time.Now()
			jc, _ := NewControlledProcess(cmd, args, done)
			jc.Suspend()
			time.Sleep(10 * time.Second)
			jc.Resume()
			_ = <-done
			timeEnd := time.Now()
			So(timeEnd.Unix()-timeStart.Unix(), ShouldBeGreaterThan, 10)
		})
		Convey("Calling Suspend() after the process has ended should return an error", func() {
			jc, _ := NewControlledProcess(cmd, args, done)
			_ = <-done
			err := jc.Suspend()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestResumeDeadProcess(t *testing.T) {
	Convey("Given a command that will complete before Resume() is called", t, func() {
		cmd := "sleep"
		args := []string{"sleep", "1"}
		done := make(chan error)
		jc, _ := NewControlledProcess(cmd, args, done)
		_ = <-done
		err := jc.Resume()
		So(err, ShouldNotBeNil)
	})
}
