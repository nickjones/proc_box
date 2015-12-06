package agents

import (
	"errors"
	"time"

	log "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/Sirupsen/logrus"
)

// Timer provides access to control the timer behavior as a stopwatch.
type Timer interface {
	Reset() error                        // Clear elapsed time
	Start() error                        // Start counting time from zero
	Stop() error                         // Pause timer
	Resume() error                       // Resume previously paused timer
	ElapsedTime() (time.Duration, error) // Return duration of elapsed time
	Done() chan error                    // Timer has hit the maximum value
}

// WallclockTimer provides an internal structure for storing the timer state.
type WallclockTimer struct {
	elapsedTime  time.Duration
	previousTime time.Time
	timeoutTime  time.Duration
	command      chan string
	ticker       *time.Ticker
	tick         bool
	done         chan error
}

// NewTimer initializes a new WallclockTimer struct and provides an interface
// to the new struct.
func NewTimer(timeout time.Duration) (Timer, error) {
	timer := &WallclockTimer{
		time.Duration(0),
		time.Now(),
		time.Duration(timeout),
		make(chan string, 1),
		time.NewTicker(100 * time.Millisecond),
		false,
		make(chan error),
	}
	go timerTicker(timer)
	return timer, nil
}

func timerTicker(timer *WallclockTimer) {
	for t := range timer.ticker.C {
		timer.incrementTimer()
		select {
		case command := <-timer.command:
			log.Debugln("Command tick at: ", t)
			log.Debugln("Command received: %s\n", command)
			switch command {
			case "reset":
				log.Debugln("Received a Reset")
				timer.tick = false
				timer.elapsedTime = time.Duration(0)
			case "start":
				log.Debugln("Received a start")
				timer.tick = true
				timer.elapsedTime = time.Duration(0)
				timer.previousTime = time.Now()
			case "stop":
				log.Debugln("Received a stop")
				timer.tick = false
			case "resume":
				log.Debugln("Received a resume")
				timer.tick = true
				timer.previousTime = time.Now()
			default:
				log.Errorln("Unknown command received")
			}
		default:
			//log.Debugln("No commands received, just keep ticking")
		}
		//log.Debugf("Timer elapsed time is: %v", timer.elapsedTime)
		//log.Debugf("Timer timeout time is: %v", timer.timeoutTime)
		if timer.elapsedTime > timer.timeoutTime {
			log.Debugln("Elapsed time has exceeded timeout time.")
			timer.done <- errors.New("The timer has timed out.")
			timer.tick = false
			// Possible bug? Reset timer to equal timeout
			timer.elapsedTime = timer.timeoutTime
		}
	}
}

func (timer *WallclockTimer) incrementTimer() {
	if timer.tick {
		timer.elapsedTime += time.Since(timer.previousTime)
		timer.previousTime = time.Now()
	}
}

// Reset clears the duration
func (timer *WallclockTimer) Reset() error {
	timer.command <- "reset"
	return nil
}

// Start starts the timer from zero
func (timer *WallclockTimer) Start() error {
	timer.command <- "start"
	return nil
}

// Stop pauses the timer from counting
func (timer *WallclockTimer) Stop() error {
	timer.command <- "stop"
	return nil
}

// Resume continues a paused timer
func (timer *WallclockTimer) Resume() error {
	timer.command <- "resume"
	return nil
}

// ElapsedTime provides the duration of elapsed time counted by the timer
func (timer *WallclockTimer) ElapsedTime() (time.Duration, error) {
	return timer.elapsedTime, nil
}

// Done provides the channel used by the tiemr when it has expired
func (timer *WallclockTimer) Done() chan error {
	return timer.done
}
