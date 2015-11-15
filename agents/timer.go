package agents

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"time"
)

type Timer interface {
	Reset() error
	Start() error
	Stop() error
	Resume() error
	Done() chan error
}

type WallclockTimer struct {
	elapsedTime  time.Duration
	previousTime time.Time
	timeoutTime  time.Duration
	command      chan string
	ticker       *time.Ticker
	tick         bool
	done         chan error
}

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

func (timer *WallclockTimer) Reset() error {
	timer.command <- "reset"
	return nil
}

func (timer *WallclockTimer) Start() error {
	timer.command <- "start"
	return nil
}

func (timer *WallclockTimer) Stop() error {
	timer.command <- "stop"
	return nil
}

func (timer *WallclockTimer) Resume() error {
	timer.command <- "resume"
	return nil
}

func (timer *WallclockTimer) Done() chan error {
	return timer.done
}
