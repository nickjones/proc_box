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

	log "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/streadway/amqp"
	"github.com/nickjones/proc_box/agents"
)

var (
	uri              = flag.String("uri", "amqp://guest:guest@localhost:5672", "AMQP connection URI.")
	exchange         = flag.String("exchange", "amq.topic", "AMQP exchange to bind.")
	rmtKey           = flag.String("rkey", "proc_box.remote_control", "AMQP routing key for remote process control.")
	procStatsKey     = flag.String("skey", "proc_box.stats", "AMQP routing key prefix for proc stats.")
	statsInterval    = flag.Duration("sint", 1*time.Minute, "Interval to emit process statistics.")
	wallclockTimeout = flag.Duration("timeout", 10*time.Minute, "The time until wallclock timeout of the process.")
	stdoutByteLimit  = flag.Int64("stdoutlimit", 100*1024*1024*1024, "Threshold of emitted bytes to STDOUT by the child before calling Stop on the process (default 100GB).")
	debugMode        = flag.Bool("debug", false, "Debug logging enable")
	noWarn           = flag.Bool("nowarn", false, "Disable warnings on stats collection.")
	// Deprecated but left for backwards compatability for now.  Assumed this is
	// enabled by default.
	runAnyway  = flag.Bool("runanyway", false, "Ignore all AMQP errors (connection, message generation, etc.).")
	msgTimeout = flag.Duration("msgtimeout", 30*time.Second, "The time allowed for a statistics mesage to be sent before giving up. (0 means never)")
	userJSON   = flag.String("json", "", "User provided JSON to include in the statistic sample.")
)

type session struct {
	amqpConn     *amqp.Connection
	job          agents.JobControl
	exchange     string
	rcRoutingKey string
	psChan       chan agents.ProcessStatCommand
	rcChan       chan chan agents.RemoteControlCommand
	amqpConfig   amqp.Config
}

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

	// Create channel for ProcessStats to trigger a sample
	psChan := make(chan agents.ProcessStatCommand)
	// Incoming remote command channel (new with each reconnect)
	rcChan := make(chan chan agents.RemoteControlCommand)

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
	job, err := agents.NewControlledProcess(cmd, args, done, *stdoutByteLimit)
	if err != nil {
		log.Fatalf("Failed to create a NewControlledProcess: %s\n", err)
		return
	}

	log.Debugf("%#v\n", job)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	timer, err := agents.NewTimer(*wallclockTimeout)
	log.Debugf("Starting timer with timeout of: %v\n", *wallclockTimeout)
	timer.Start()

	sess := session{
		job:          job,
		exchange:     *exchange,
		rcRoutingKey: *rmtKey,
		psChan:       psChan,
		rcChan:       rcChan,
		amqpConfig: amqp.Config{
			Properties: amqp.Table{
				"product": "proc_box",
				"version": "master",
			},
		},
	}
	go redial(sess)

	go monitor(signals, rcChan, job, psChan, timer, done)

	_ = <-done

	elapsedTime, _ := timer.ElapsedTime()
	// Print to standard out
	fmt.Printf("Task elapsed time: %.2f seconds.\n", elapsedTime.Seconds())
}

func redial(sess session) {
	var err error
	var stats agents.ProcessStats
	var rc *agents.RemoteControl

	// Initialize mini-router for incoming stats agent requests
	go func() {
		for s := range sess.psChan {
			if stats == nil {
				log.Warnln("No stats agents available (yet), dropping request")
			} else if s.TimeUpdate {
				stats.NewTicker(s.NewTime)
			} else {
				stats.Sample()
			}
		}
	}()

	for {
		sess.amqpConn, err = amqp.DialConfig(*uri, sess.amqpConfig)

		if err != nil {
			log.Warnf("Failed to connect to AMQP: %q", err)
		} else {
			rc, err = agents.NewRemoteControl(sess.amqpConn, *rmtKey, *exchange)
			if err != nil {
				log.Warnf("Failed creating NewRemoteControl: %s", err)
			} else {
				sess.rcChan <- rc.Commands
			}

			if stats == nil {
				// initial setup
				stats, err = agents.NewProcessStats(
					sess.amqpConn,
					*procStatsKey,
					*exchange,
					&sess.job,
					*statsInterval,
					*msgTimeout,
					*userJSON,
				)
				if err != nil {
					log.Warnf("Failed creating NewProcessStats: %s", err)
				}
			} else {
				err = stats.ReinitializeConnection(sess.amqpConn)
				if err != nil {
					log.Warnf("Failed to reinitialize process stats: %s", err)
				}
			}
		}

		closings := sess.amqpConn.NotifyClose(make(chan *amqp.Error))

		// Wait for close notification and loop back around to reconnect
		_ = <-closings
		log.Debugln("Saw a notification for closed connection, looping")
	}
}

func monitor(
	signals chan os.Signal,
	rcChanChan chan chan agents.RemoteControlCommand,
	job agents.JobControl,
	psChan chan agents.ProcessStatCommand,
	timer agents.Timer,
	done chan error,
) {

	// Catch any panics here to ensure we kill the child process before going
	// to our own doom.
	defer func() {
		if e := recover(); e != nil {
			job.Kill(-9)
			panic(e)
		}
	}()
	var err error
	var logSampling <-chan time.Time

	if *stdoutByteLimit > 0 {
		ticker := time.NewTicker(100 * time.Millisecond)
		// Replace the time channel with an actual ticker if this is in use
		logSampling = ticker.C
	}

	var rcChan chan agents.RemoteControlCommand

	for {
		select {
		case rcChan = <-rcChanChan:
		// Catch incoming signals and operate on them as if they were remote commands
		case sig := <-signals:
			switch sig {
			case syscall.SIGINT:
				log.Debugln("Caught SIGINT, graceful shutdown")
				// Initiate non-blocking send
				select {
				case psChan <- agents.ProcessStatCommand{}:
					log.Debugln("Sending psChan a msg to sample")
				default:
					log.Debugln("SIGINT failed to send a sample msg on the psChan")
				}
				err = job.Stop()
			case syscall.SIGTERM:
				log.Debugln("Caught SIGTERM, end abruptly")
				job.Kill(-9)
			case syscall.SIGHUP:
				log.Debugln("Caught SIGHUP, emit stats")
				// Initiate non-blocking send
				select {
				case psChan <- agents.ProcessStatCommand{}:
					log.Debugln("Sending psChan a msg to sample")
				default:
					log.Debugln("SIGHUP failed to send a sample msg on the psChan")
				}
			case syscall.SIGQUIT:
				log.Debugln("Caught SIGQUIT, graceful shutdown")
				select {
				case psChan <- agents.ProcessStatCommand{}:
					log.Debugln("Sending psChan a msg to sample")
				default:
					log.Debugln("SIGQUIT failed to send a sample msg on the psChan")
				}
				err = job.Stop()
			}
		// Process incoming remote commands, toss unknown requests
		case cmd := <-rcChan:
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
			case "stop":
				log.Debugln("RemoteCommand: Stop")
				select {
				case psChan <- agents.ProcessStatCommand{}:
					log.Debugln("Sending psChan a msg to sample")
				default:
					log.Debugln("RC Stop failed to send a sample msg on the psChan")
				}
				if err := job.Stop(); err != nil {
					log.Fatalf("Error received while stopping sub-process: %s\n", err)
				}
			case "sample":
				log.Debugln("RemoteCommand: Sample")
				select {
				case psChan <- agents.ProcessStatCommand{}:
					log.Debugln("Sending psChan a msg to sample")
				default:
					log.Debugln("RC Sample failed to send a sample msg on the psChan")
				}
			case "change_sample_rate":
				log.Debugln("RemoteCommand: Change Stats Sample Rate")
				if len(cmd.Arguments) > 0 {
					log.Debugf("change_sample_rate arg[0]: %s\n", cmd.Arguments[0])
					d, err := time.ParseDuration(cmd.Arguments[0])
					if err == nil {
						select {
						case psChan <- agents.ProcessStatCommand{true, d}:
							log.Debugln("Sending psChan a msg to update the ticker")
						default:
							log.Debugln("RC change_sample_rate failed to send a msg")
						}
					} else {
						log.Warnf("Unparseable duration argument to command change_sample_rate")
					}
				} else {
					log.Warnf("Missing argument to command change_sample_rate")
				}
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
		case _ = <-job.Done():
			log.Debugln("Command exited gracefully; shutting down.")
			done <- err
		case _ = <-logSampling:
			if job.StdoutByteCount() > 2*(*stdoutByteLimit) {
				err = job.Kill(-9)
			} else if job.StdoutByteCount() > *stdoutByteLimit {
				err = job.Stop()
			}
		}
	}
}
