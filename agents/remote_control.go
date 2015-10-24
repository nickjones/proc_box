package agents

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/streadway/amqp"
)

// RemoteControl is a container for the AMQP connection for remote process control
type RemoteControl struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	tag      string
	done     chan error
	Commands chan string
}

// NewRemoteControl creates a new watcher for external commands through AMQP
// to control the boxed process.
func NewRemoteControl(amqp *amqp.Connection, routingKey string, exchange string) (*RemoteControl, error) {
	var err error

	rc := &RemoteControl{
		amqp,
		nil,
		"proc_box_remote_control", // consumerTag
		nil,
		make(chan string),
	}

	rc.channel, err = rc.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("Unable to open a channel on AMQP connection: %s", err)
	}

	remCtrlQueue, err := rc.channel.QueueDeclare(
		"",    // name of queue
		false, // durable
		true,  // delete when unused
		false, // exclusive
		false, // nowait
		nil,   // arguments
	)

	if err != nil {
		return nil, fmt.Errorf("Failed to acquire a remote control queue: %s", err)
	}

	if err = rc.channel.QueueBind(
		remCtrlQueue.Name, // name of the queue
		routingKey,        // bindingkey
		exchange,          // sourceexchange
		false,             // nowait
		nil,               // arguments
	); err != nil {
		return nil, fmt.Errorf("Failed to bind the queue: %s", err)
	}

	deliveries, err := rc.channel.Consume(
		remCtrlQueue.Name,         // name
		"proc_box_remote_control", // consumertag
		false, // noack
		false, // exclusive
		false, // nolocal
		false, // nowait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("Failed to consume queue: %s", err)
	}

	go handle(deliveries, rc.done, rc.Commands)

	return rc, nil
}

// Shutdown gracefully stops incoming command traffic and closes the channel
func (rc *RemoteControl) Shutdown() error {
	if err := rc.channel.Cancel(rc.tag, true); err != nil {
		return fmt.Errorf("RemoteControl cancel failed: %s", err)
	}
	return <-rc.done
}

func handle(deliveries <-chan amqp.Delivery, done chan error, commands chan string) {
	for d := range deliveries {
		log.Info(fmt.Sprintf("%s", d.Body))
		commands <- fmt.Sprintf("%q", d.Body)
		d.Ack(false)
	}
	done <- nil
}
