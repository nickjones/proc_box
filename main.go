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
	var cmdArgs []string
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	} else {
		log.Fatal("Did you forget a command to run?")
		return
	}

	log.Debugf("cmd: %s cmdArgs: %q", cmd, cmdArgs)

	done := make(chan error)

	job, err := agents.NewControlledProcess(cmd, args, done)
	if err != nil {
		log.Fatalf("Failed to create a NewControlledProcess: %s", err)
		return
	}

	fmt.Printf("%#v\n", job)

	go monitor(rc, job, done)

	_ = <-done
}

func monitor(rc *agents.RemoteControl, job agents.JobControl, done chan error) {
	var err error
	for {
		select {
		case cmd := <-rc.Commands:
			log.Debugf("Got a command %#v\n", cmd)
			switch cmd.Command {
			case "done":
				fmt.Println("Ending task")
				done <- err
			default:
				log.Debugf("Unknown command: %s\n", cmd)
			}
		case _ = <-job.Done():
			log.Debugln("Command exited gracefully; shutting down.")
			done <- err
		}
	}
}
