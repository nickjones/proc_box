package main

import (
	"os"
	"testing"

	"github.com/nickjones/proc_box/agents"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/streadway/amqp"
)

var amqpConn *amqp.Connection
var err error

// TestIntegrationDialAMQP tests a connection to an AMQP broker and quits
func TestIntegrationDialAMQP(t *testing.T) {
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
		So(amqpConn, ShouldNotBeNil)
		Convey("Given an exchange and routing key", func() {
			exchange := "amq.topic"
			rmtKey := "testing.proc_box"
			Convey("When agent.NewRemoteControl is invoked", func() {
				rc, err := agents.NewRemoteControl(amqpConn, rmtKey, exchange)
				Convey("The handle should not be nil and the err is nil", func() {
					So(rc, ShouldNotBeNil)
					So(err, ShouldBeNil)
				})
			})
		})
	})
}
