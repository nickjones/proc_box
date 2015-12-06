package agents

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	log "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/shirou/gopsutil/cpu"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/shirou/gopsutil/host"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/shirou/gopsutil/process"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/streadway/amqp"
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
	ChildPids  []int32                 // Children process IDs
	CPUPercent float64                 // Sum of parent and children CPU percent usage
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

	// Collect for parent
	stat.aggregateStatForProc(proc)

	children, err := proc.Children()
	// Collect on children (if any)
	if err == nil {
		for _, cproc := range children {
			stat.ChildPids = append(stat.ChildPids, cproc.Pid)
			stat.aggregateStatForProc(cproc)
		}
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

func (p *ProcessStatSample) aggregateStatForProc(proc *process.Process) {
	meminfo, err := proc.MemoryInfo()
	if err != nil {
		log.Warnf("Error encountered collecting memory stats: %s", err)
	} else {
		src := reflect.ValueOf(meminfo).Elem()
		dest := reflect.ValueOf(&p.Memory).Elem()
		sum(&src, &dest)
	}

	cputimes, err := proc.CPUTimes()
	if err != nil {
		log.Warnf("Error encountered collecting CPU stats: %s", err)
	} else {
		src := reflect.ValueOf(cputimes).Elem()
		dest := reflect.ValueOf(&p.CPUTimes).Elem()
		sum(&src, &dest)
	}

	iocnt, err := proc.IOCounters()
	if err != nil {
		log.Warnf("Error encountered collecting I/O stats: %s", err)
	} else {
		src := reflect.ValueOf(iocnt).Elem()
		dest := reflect.ValueOf(&p.IOCounters).Elem()
		sum(&src, &dest)
	}

	openFiles, err := proc.OpenFiles()
	if err != nil {
		log.Warnf("Error encountered collecting open files stats: %s", err)
	} else {
		p.OpenFiles = append(p.OpenFiles, openFiles...)
	}

	// NOTE: This will end up counting separate pids as "threads"
	numThreads, err := proc.NumThreads()
	if err != nil {
		log.Warnf("Error encountered collecting thread count stats: %s", err)
	} else {
		p.NumThreads += numThreads
	}

	// Use 0 interval to get difference since the last call
	cpuPercent, err := proc.CPUPercent(0 * time.Second)
	if err != nil {
		log.Warnf("Error encountered collecting CPU percent: %s", err)
	} else {
		p.CPUPercent += cpuPercent
	}
}

func sum(src *reflect.Value, dest *reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		switch src.Field(i).Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			var total uint64
			total += src.Field(i).Uint()
			total += dest.Field(i).Uint()
			dest.Field(i).SetUint(total)
		case reflect.Float32, reflect.Float64:
			var total float64
			total += src.Field(i).Float()
			total += dest.Field(i).Float()
			dest.Field(i).SetFloat(total)
		}
	}
}
