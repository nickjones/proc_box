package agents

import (
	"os"
	"testing"

	. "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/smartystreets/goconvey/convey"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/streadway/amqp"
)

// TestIntegrationDialAMQP tests a connection to an AMQP broker and quits
func TestDialAMQP(t *testing.T) {
	var amqpConn *amqp.Connection
	var err error
	Convey("Given an AMQP URI", t, func() {
		uri := os.Getenv("AMQP_URI")
		Convey("The connection should be returned and err is nil", func() {
			amqpConn, err = amqp.Dial(uri)
			So(err, ShouldBeNil)
		})
	})
}

func TestNewRemoteControl(t *testing.T) {
	Convey("Given a non-nil AMQP connection", t, func() {
		uri := os.Getenv("AMQP_URI")
		var amqpConn *amqp.Connection
		var err error
		Convey("The connection should be returned and err is nil", func() {
			amqpConn, err = amqp.Dial(uri)
			So(err, ShouldBeNil)
			So(amqpConn, ShouldNotBeNil)
			Convey("Given an exchange and routing key", func() {
				exchange := "amq.topic"
				rmtKey := "testing.proc_box"
				Convey("When agents.NewRemoteControl is invoked", func() {
					rc, err := NewRemoteControl(amqpConn, rmtKey, exchange)
					Convey("The handle should not be nil and the err is nil", func() {
						So(rc, ShouldNotBeNil)
						So(err, ShouldBeNil)
					})
				})
			})
			Convey("Given a non-existent exchange", func() {
				exchange := "this.doesnt.exist"
				rmtKey := "testing"
				Convey("When agents.NewRemoteControl is invoked", func() {
					rc, err := NewRemoteControl(amqpConn, rmtKey, exchange)
					Convey("The handle should be nil and the error is not nil", func() {
						So(rc, ShouldBeNil)
						So(err, ShouldNotBeNil)
					})
				})
			})
		})
	})
	Convey("When agents.NewRemoteControl is invoked with a nil AMQP connection", t, func() {
		rc, err := NewRemoteControl(nil, "testing", "amq.topic")
		Convey("The handle should be nil and the err is not nil", func() {
			So(rc, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}
