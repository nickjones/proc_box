package main

import (
	"flag"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/agents"
	"github.com/streadway/amqp"
)

var (
	uri           = flag.String("uri", "amqp://guest:guest@localhost:5672", "AMQP connection URI.")
	exchange      = flag.String("exchange", "amq.topic", "AMQP exchange to bind.")
	rmtKey        = flag.String("rmt_key", "proc_box.remote_control", "AMQP routing key for remote process control.")
	procStatsKey  = flag.String("proc_stats_key", "proc_box.stats", "AMQP routing key prefix for proc stats.")
	statsInterval = flag.Duration("proc_stats_interval", 5*time.Minute, "Interval to emit process statistics.")
	debugMode     = flag.Bool("debug", false, "Debug logging enable")
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
		log.Fatalf("Failed to connect to AMQP: %s\n", err)
	}

	// Establish remote control channel prior to execution
	rc, err := agents.NewRemoteControl(amqpConn, *rmtKey, *exchange)
	if err != nil {
		log.Fatalf("NewRemoteControl failed: %s\n", err)
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

	log.Debugf("cmd: %s cmdArgs: %q\n", cmd, cmdArgs)

	done := make(chan error)

	// Initialize job
	job, err := agents.NewControlledProcess(cmd, args, done)
	if err != nil {
		log.Fatalf("Failed to create a NewControlledProcess: %s\n", err)
		return
	}

	// Establish process statistics gathering agent
	// Agent will need to initially wait for the process to start, but should
	// establish an AMQP channel for message generation prior to the process
	// starting.
	stats, err := agents.NewProcessStats(
		amqpConn,
		*procStatsKey,
		*exchange,
		&job,
		*statsInterval,
	)

	log.Debugf("%#v\n", stats)

	log.Debugf("%#v\n", job)

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
			case "suspend":
				log.Debugln("RemoteCommand: Suspend")
				job.Suspend()
			case "resume":
				log.Debugln("RemoteCommand: Resume")
				job.Resume()
			case "kill":
				log.Debugln("RemoteCommand: Kill")
				var args int64
				var err error
				if len(cmd.Arguments) == 0 {
					args = -9
				} else {
					args, err = strconv.ParseInt(cmd.Arguments[0], 10, 32)
					if err != nil {
						log.Warnf("Unable to parse kill command argument[0] into int: %s\n", err)
						args = -9
					}
				}
				job.Kill(args)
				done <- err
			case "stop":
				var err error
				log.Debugln("RemoteCommand: Stop")
				if err := job.Stop(); err != nil {
					log.Fatalf("Error received while stopping sub-process: %s\n", err)
				}
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
