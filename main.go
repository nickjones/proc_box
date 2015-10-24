package main

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/agents"
	"github.com/streadway/amqp"
)

var (
	uri          = flag.String("uri", "amqp://guest:guest@localhost:5672", "AMQP connection URI.")
	exchange     = flag.String("exchange", "amq.topic", "AMQP exchange to bind.")
	rmtKey       = flag.String("rmt_key", "proc_box.remote_control", "AMQP routing key for remote process control.")
	procStatsKey = flag.String("proc_stats_key", "proc_box.stats", "AMQP routing key prefix for proc stats.")
	debugMode    = flag.Bool("debug", false, "Debug logging enable")
)

func init() {
	flag.Parse()
}

func main() {
	if *debugMode {
		log.SetLevel(log.DebugLevel)
	}

	amqpConn, err := amqp.Dial(*uri)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to connect to AMQP: %s", err))
	}

	rc, err := agents.NewRemoteControl(amqpConn, *rmtKey, *exchange)
	if err != nil {
		log.Fatal(fmt.Sprintf("NewRemoteControl failed: %s", err))
	}

	args := flag.Args()
	job, err := agents.NewControlledProcess(args[0], args[1:])

	select {
	case cmd := <-rc.Commands:
		fmt.Println("Got a command ", cmd)
	}

	done := make(chan error)

	go monitor(rc, job, done)

	_ = <-done
	return
}

func monitor(rc *agents.RemoteControl, job *agents.Job, done chan error) {

}
