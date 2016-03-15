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
	NewTicker(time.Duration)
	ReinitializeConnection(*amqp.Connection) error
}

// ProcessStatCollector is a container for internal state
type ProcessStatCollector struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	routingKey string
	exchange   string
	job        *JobControl
	ticker     <-chan time.Time
	msgTimeout time.Duration
	userJSON   map[string]interface{}
}

// ProcessStatSample contains a single sample of the underlying process system usage.
// See gopsutil for defintions of these structures and support for particular OSes.
type ProcessStatSample struct {
	Host        host.HostInfoStat       // See gopsutil for description
	Memory      process.MemoryInfoStat  // See gopsutil for description
	CPUTimes    cpu.CPUTimesStat        // See gopsutil for description
	IOCounters  process.IOCountersStat  // See gopsutil for description
	OpenFiles   []process.OpenFilesStat // See gopsutil for description
	NumThreads  int32                   // Number of threads in use by the child processes (if supported)
	Pid         int32                   // Process ID for the group
	ChildPids   []int32                 // Children process IDs
	CPUPercent  float64                 // Sum of parent and children CPU percent usage
	TimeUTC     time.Time               // Timestamp of collection with location set to UTC
	TimeUnix    int64                   // Timestamp of collection based on seconds elapsed since the unix epoch.
	StdoutBytes int64                   // Running total of bytes emitted via STDOUT by the child process
	UserData    map[string]interface{}  // User provided JSON from invocation (not validated in proc_box!)
}

// ProcessStatCommand is used to communicate sample commands and ticker updates
// with a channel implementation to the ProcessStats agent.
type ProcessStatCommand struct {
	TimeUpdate bool
	NewTime    time.Duration
}

// NewProcessStats establishes a new AMQP channel and configures sampling period
func NewProcessStats(
	amqp *amqp.Connection,
	routingKey string,
	exchange string,
	job *JobControl,
	interval time.Duration,
	msgTimeout time.Duration,
	userJSON string,
) (ProcessStats, error) {

	var err error
	var extraJSON map[string]interface{}

	if len(userJSON) > 0 {
		err = json.Unmarshal([]byte(userJSON), &extraJSON)
		if err != nil {
			log.Warnf("Error encountered trying to unmarshal user provided JSON: %s\n", err)
		}
	}

	psc := &ProcessStatCollector{
		amqp,
		nil,
		routingKey,
		exchange,
		job,
		nil,
		msgTimeout,
		extraJSON,
	}

	err = psc.ReinitializeConnection(amqp)
	if err != nil {
		return nil, fmt.Errorf("Unable to open a channel on AMQP connection: %s\n", err)
	}

	psc.NewTicker(interval)

	return psc, nil
}

// ReinitializeConnection reconnects to the AMQP exchange
func (ps *ProcessStatCollector) ReinitializeConnection(newConn *amqp.Connection) error {
	var err error

	if newConn == nil {
		return fmt.Errorf("nil amqp.Connection argument")
	}

	ps.connection = newConn
	ps.channel, err = ps.connection.Channel()
	if err != nil {
		return fmt.Errorf("Unable to open a channel on AMQP connection: %s\n", err)
	}

	if err = ps.channel.ExchangeDeclare(
		ps.exchange, // name
		"topic",     // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // nowait
		nil,         // arguments
	); err != nil {
		return fmt.Errorf("Unable to declare the exchange: %s", err)
	}
	return nil
}

// Sample collects process statistcs and emits them.
func (ps *ProcessStatCollector) Sample() error {
	// Start a race between the timeout (if applicable) and the message
	// collection.

	done := make(chan error, 1)
	go func() {
		done <- ps.collectSample()
	}()

	var timerDone <-chan time.Time
	if ps.msgTimeout > 0*time.Second {
		timerDone = time.After(ps.msgTimeout)
	}

	select {
	case err := <-done:
		return err
	case <-timerDone:
		log.Warnln("Sample collection exceeded timeout")
		return fmt.Errorf("Sample collection exceeded timeout")
	}
}

func (ps *ProcessStatCollector) collectSample() error {
	var err error
	job := *ps.job
	proc := job.Process()

	stat := ProcessStatSample{}

	stat.Pid = proc.Pid

	curTime := time.Now()
	stat.TimeUTC = curTime.UTC()
	stat.TimeUnix = curTime.Unix()

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

	stat.StdoutBytes = job.StdoutByteCount()

	stat.UserData = ps.userJSON

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

// NewTicker creates a new time.Ticker instance with the passed in duration
// Used to dynamically change the sampling interval
func (ps *ProcessStatCollector) NewTicker(d time.Duration) {
	ps.ticker = time.NewTicker(d).C
	// Construct a new goroutine to handle ticking since the prior channel closed
	go func(psc *ProcessStatCollector) {
		for {
			select {
			case _ = <-psc.ticker:
				psc.Sample()
			}
		}
	}(ps)
}

func (p *ProcessStatSample) aggregateStatForProc(proc *process.Process) {
	// Split individual stat calls into functions for panic recovery
	// without losing the entire statistic.

	p.collectMemInfo(proc)

	p.collectCPUTimes(proc)

	p.collectIOCounters(proc)

	p.collectOpenFiles(proc)

	// NOTE: This will end up counting separate pids as "threads"
	p.collectNumThreads(proc)

	p.collectCPUPercent(proc)
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

func (p *ProcessStatSample) collectMemInfo(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on memory stats collection. Maybe unsupported on this platform.")
		}
	}()
	meminfo, err := proc.MemoryInfo()
	if err != nil {
		log.Warnf("Error encountered collecting memory stats: %s", err)
	} else {
		src := reflect.ValueOf(meminfo).Elem()
		dest := reflect.ValueOf(&p.Memory).Elem()
		sum(&src, &dest)
	}
}

func (p *ProcessStatSample) collectCPUTimes(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on CPU times collection. Maybe unsupported on this platform.")
		}
	}()
	cputimes, err := proc.CPUTimes()
	if err != nil {
		log.Warnf("Error encountered collecting CPU stats: %s", err)
	} else {
		src := reflect.ValueOf(cputimes).Elem()
		dest := reflect.ValueOf(&p.CPUTimes).Elem()
		sum(&src, &dest)
	}
}

func (p *ProcessStatSample) collectIOCounters(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on IO counters collection. Maybe unsupported on this platform.")
		}
	}()
	iocnt, err := proc.IOCounters()
	if err != nil {
		log.Warnf("Error encountered collecting I/O stats: %s", err)
	} else {
		src := reflect.ValueOf(iocnt).Elem()
		dest := reflect.ValueOf(&p.IOCounters).Elem()
		sum(&src, &dest)
	}
}

func (p *ProcessStatSample) collectOpenFiles(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on Open Files collection. Maybe unsupported on this platform.")
		}
	}()
	openFiles, err := proc.OpenFiles()
	if err != nil {
		log.Warnf("Error encountered collecting open files stats: %s", err)
	} else {
		p.OpenFiles = append(p.OpenFiles, openFiles...)
	}
}

func (p *ProcessStatSample) collectNumThreads(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on Number of Threads collection. Maybe unsupported on this platform.")
		}
	}()
	numThreads, err := proc.NumThreads()
	if err != nil {
		log.Warnf("Error encountered collecting thread count stats: %s", err)
	} else {
		p.NumThreads += numThreads
	}
}

func (p *ProcessStatSample) collectCPUPercent(proc *process.Process) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("Recovered from panic on CPU Percent collection. Maybe unsupported on this platform.")
		}
	}()
	// Use 0 interval to get difference since the last call
	cpuPercent, err := proc.CPUPercent(0 * time.Second)
	if err != nil {
		log.Warnf("Error encountered collecting CPU percent: %s", err)
	} else {
		p.CPUPercent += cpuPercent
	}
}
