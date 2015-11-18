package agents

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
	"github.com/streadway/amqp"
)

// ProcessStats provides an interface for collecting statistics about the pid
type ProcessStats interface {
	Sample() error // Take a statistical sample and emit it on AMQP
}

// ProcessStatCollector is a container for internal state
type ProcessStatCollector struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	routingKey string
	exchange   string
	job        *JobControl
	ticker     <-chan time.Time
}

// ProcessStatSample contains a single sample of the underlying process system usage.
// See gopsutil for defintions of these structures and support for particular OSes.
type ProcessStatSample struct {
	Host       host.HostInfoStat       // See gopsutil for description
	Memory     process.MemoryInfoStat  // See gopsutil for description
	CPUTimes   cpu.CPUTimesStat        // See gopsutil for description
	IOCounters process.IOCountersStat  // See gopsutil for description
	OpenFiles  []process.OpenFilesStat // See gopsutil for description
	NumThreads int32                   // Number of threads in use by the child processes (if supported)
	Pid        int32                   // Process ID for the group
}

// NewProcessStats establishes a new AMQP channel and configures sampling period
func NewProcessStats(amqp *amqp.Connection, routingKey string,
	exchange string, job *JobControl, interval time.Duration) (ProcessStats, error) {

	var err error
	psc := &ProcessStatCollector{
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

	stat.Pid = proc.Pid

	hostinfo, err := host.HostInfo()
	var hostnameRoutingKey string
	if err != nil {
		log.Warnf("Error encountered collecting host stats: %s", err)
		hostnameRoutingKey = ""
	} else {
		stat.Host = *hostinfo
		hostnameRoutingKey = fmt.Sprintf(".%s", stat.Host.Hostname)
	}

	meminfo, err := proc.MemoryInfo()
	if err != nil {
		log.Warnf("Error encountered collecting memory stats: %s", err)
	} else {
		stat.Memory = *meminfo
	}

	cputimes, err := proc.CPUTimes()
	if err != nil {
		log.Warnf("Error encountered collecting CPU stats: %s", err)
	} else {
		stat.CPUTimes = *cputimes
	}

	iocnt, err := proc.IOCounters()
	if err != nil {
		log.Warnf("Error encountered collecting I/O stats: %s", err)
	} else {
		stat.IOCounters = *iocnt
	}

	stat.OpenFiles, err = proc.OpenFiles()
	if err != nil {
		log.Warnf("Error encountered collecting open files stats: %s", err)
	}

	stat.NumThreads, err = proc.NumThreads()
	if err != nil {
		log.Warnf("Error encountered collecting thread count stats: %s", err)
	}

	log.Debugf("Sample: %#v\n", stat)

	var body []byte
	body, err = json.Marshal(stat)

	routingKey := fmt.Sprintf("%s%s", ps.routingKey, hostnameRoutingKey)
	err = ps.channel.Publish(
		ps.exchange, // publish to an exchange
		routingKey,  // routing to queues
		false,       // mandatory
		false,       // immediate
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
