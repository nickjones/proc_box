package agents

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/process"
	"github.com/streadway/amqp"
)

// ProcessStats provides an interface for collecting statistics about the pid
type ProcessStats interface {
	Sample() error
	Start() error
	Stop() error
}

// ProcessStatCollector is a container for internal state
type ProcessStatCollector struct {
	ProcessStats
	connection *amqp.Connection
	channel    *amqp.Channel
	routingKey string
	exchange   string
	job        *JobControl
	ticker     <-chan time.Time
}

// ProcessStatSample contains a single sample of the underlying process system usage.
type ProcessStatSample struct {
	Memory     *process.MemoryInfoStat
	CPUTimes   *cpu.CPUTimesStat
	IOCounters *process.IOCountersStat
	OpenFiles  []process.OpenFilesStat
	NumThreads int32
}

// NewProcessStats establishes a new AMQP channel and configures sampling period
func NewProcessStats(amqp *amqp.Connection, routingKey string,
	exchange string, job *JobControl, interval time.Duration) (ProcessStats, error) {

	var err error
	psc := &ProcessStatCollector{
		nil,
		amqp,
		nil,
		routingKey,
		exchange,
		job,
		nil,
	}

	psc.channel, err = psc.connection.Channel()
	if err != nil {
		return nil, fmt.Errorf("Unable to open a channel on AMQP connection: %s\n", err)
	}

	if err = psc.channel.ExchangeDeclare(
		exchange, // name
		"topic",  // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // nowait
		nil,      // arguments
	); err != nil {
		return nil, fmt.Errorf("Unable to declare the exchange: %s", err)
	}

	psc.ticker = time.NewTicker(interval).C

	go func(psc *ProcessStatCollector) {
		for {
			select {
			case _ = <-psc.ticker:
				psc.Sample()
			}
		}
	}(psc)

	return psc, nil
}

// Sample collects process statistcs and emits them.
func (ps *ProcessStatCollector) Sample() error {
	var err error
	job := *ps.job
	proc := job.Process()

	stat := ProcessStatSample{}

	stat.Memory, err = proc.MemoryInfo()
	if err != nil {
		log.Warnf("Error encountered collecting memory stats: %s", err)
	}

	stat.CPUTimes, err = proc.CPUTimes()
	if err != nil {
		log.Warnf("Error encountered collecting CPU stats: %s", err)
	}

	stat.IOCounters, err = proc.IOCounters()
	if err != nil {
		log.Warnf("Error encountered collecting I/O stats: %s", err)
	}

	stat.OpenFiles, err = proc.OpenFiles()
	if err != nil {
		log.Warnf("Error encountered collecting open files stats: %s", err)
	}

	stat.NumThreads, err = proc.NumThreads()
	if err != nil {
		log.Warnf("Error encountered collecting thread count stats: %s", err)
	}

	log.Debugf("Memory sample: %#v\n", stat.Memory)
	log.Debugf("CPU sample: %#v\n", stat.CPUTimes)
	log.Debugf("I/O sample: %#v\n", stat.IOCounters)
	log.Debugf("Files sample: %#v\n", stat.OpenFiles)
	log.Debugf("Threads sample: %#v\n", stat.NumThreads)

	var body []byte
	body, err = json.Marshal(stat)

	err = ps.channel.Publish(
		ps.exchange,   // publish to an exchange
		ps.routingKey, // routing to queues
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/javascript",
			ContentEncoding: "",
			Body:            body,
			DeliveryMode:    amqp.Transient, // non-persistent
			Priority:        0,              // 0-9
		},
	)
	if err != nil {
		log.Warnf("Error publishing statistics sample %s", err)
	}

	return err
}
