package agents

import (
	"testing"

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
		args := []string{"sleep", "10"}
		done := make(chan error)
		Convey("When agents.NewControlledProcess is invoked", func() {
			jc, err := NewControlledProcess(cmd, args, done)
			Convey("The handle should not be nil and the error is nil", func() {
				So(jc, ShouldNotBeNil)
				So(err, ShouldBeNil)
			})
		})
	})
}
