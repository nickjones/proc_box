package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/agents"
	"github.com/streadway/amqp"
)

var (
	uri              = flag.String("uri", "amqp://guest:guest@localhost:5672", "AMQP connection URI.")
	exchange         = flag.String("exchange", "amq.topic", "AMQP exchange to bind.")
	rmtKey           = flag.String("rkey", "proc_box.remote_control", "AMQP routing key for remote process control.")
	procStatsKey     = flag.String("skey", "proc_box.stats", "AMQP routing key prefix for proc stats.")
	statsInterval    = flag.Duration("sint", 1*time.Minute, "Interval to emit process statistics.")
	wallclockTimeout = flag.Duration("timeout", 10*time.Minute, "The time until wallclock timeout of the process.")
	debugMode        = flag.Bool("debug", false, "Debug logging enable")
	noWarn           = flag.Bool("nowarn", false, "Disable warnings on stats collection.")
)

func init() {
	flag.Parse()

	// Create a map of pointers to all the current flags
	flags := map[string]*flag.Flag{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f
	})

	// Remove the flags that were set on the command line
	flag.Visit(func(f *flag.Flag) {
		delete(flags, f.Name)
	})

	// Now for the flags that weren't set on the cli,
	// Look at the env for 'PBOX_<uppercase flag name>'
	// If it exists, use it to set the corresponding flag.
	for _, f := range flags {
		var buffer bytes.Buffer
		buffer.WriteString("PBOX_")
		buffer.WriteString(strings.ToUpper(f.Name))
		if os.Getenv(buffer.String()) != "" {
			f.Value.Set(os.Getenv(buffer.String()))
		}
	}
}

func main() {
	if *debugMode {
		log.SetLevel(log.DebugLevel)
	} else if *noWarn {
		log.SetLevel(log.ErrorLevel)
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

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	timer, err := agents.NewTimer(*wallclockTimeout)
	log.Debugf("Starting timer with timeout of: %v\n", *wallclockTimeout)
	timer.Start()

	go monitor(signals, rc, job, stats, timer, done)

	_ = <-done

	elapsedTime, _ := timer.ElapsedTime()
	// Print to standard out
	fmt.Printf("Task elapsed time: %.2f seconds.\n", elapsedTime.Seconds())
}

func monitor(
	signals chan os.Signal,
	rc *agents.RemoteControl,
	job agents.JobControl,
	ps agents.ProcessStats,
	timer agents.Timer,
	done chan error,
) {
	var err error
	for {
		select {
		// Catch incoming signals and operate on them as if they were remote commands
		case sig := <-signals:
			switch sig {
			case syscall.SIGINT:
				log.Debugln("Caught SIGINT, graceful shutdown")
				ps.Sample()
				err = job.Stop()
				done <- err
			case syscall.SIGTERM:
				log.Debugln("Caught SIGTERM, end abruptly")
				job.Kill(-9)
				done <- err
			case syscall.SIGHUP:
				log.Debugln("Caught SIGHUP, emit stats")
				ps.Sample()
			case syscall.SIGQUIT:
				log.Debugln("Caught SIGQUIT, graceful shutdown")
				ps.Sample()
				err = job.Stop()
				done <- err
			}
		// Process incoming remote commands, toss unknown requests
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
				ps.Sample()
				if err := job.Stop(); err != nil {
					log.Fatalf("Error received while stopping sub-process: %s\n", err)
				}
				done <- err
			case "sample":
				log.Debugln("RemoteCommand: Sample")
				ps.Sample()
			case "timer_reset":
				log.Debugln("RemoteCommand: Timer Reset")
				if err := timer.Reset(); err != nil {
					log.Fatalf("Error received from timer calling Reset: %s\n", err)
				}
			case "timer_start":
				log.Debugln("RemoteCommand: Timer Start")
				if err := timer.Start(); err != nil {
					log.Fatalf("Error received from timer calling Start: %s\n", err)
				}
			case "timer_stop":
				log.Debugln("RemoteCommand: Timer Stop")
				if err := timer.Stop(); err != nil {
					log.Fatalf("Error received from timer calling Stop: %s\n", err)
				}
			case "timer_resume":
				log.Debugln("RemoteCommand: Timer Resume")
				if err := timer.Resume(); err != nil {
					log.Fatalf("Error received from timer calling Resume: %s\n", err)
				}
			default:
				log.Debugf("Unknown command: %s\n", cmd)
			}
		case timeoutMsg := <-timer.Done():
			log.Debugf("Timer timeout message: %s\n", timeoutMsg)
			if err := job.Stop(); err != nil {
				log.Fatalf("Error received while stopping sub-process: %v\n", err)
				// If there was an error stopping the process, kill the porcess.
				job.Kill(-9)
			}
			done <- err
		case _ = <-job.Done():
			log.Debugln("Command exited gracefully; shutting down.")
			done <- err
		}
	}
}
