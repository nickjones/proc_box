// Package agents provides individual handlers for managing portions of controlling or observing a contained process.
package agents

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	log "github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/aybabtme/iocontrol"
	"github.com/nickjones/proc_box/Godeps/_workspace/src/github.com/shirou/gopsutil/process"
)

// JobControl provides a command interface for control
type JobControl interface {
	Stop() error               // Gracefully end process
	Suspend() error            // Stop scheduling process to be resumed later
	Resume() error             // Resume a previously stopped process
	Kill(int64) error          // Send SIGKILL (or arg os.Signal)
	Done() chan error          // Reflects the child process is done
	Process() *process.Process // Handle to gopsutil process struct for stats
	StdoutByteCount() int64    // Return the number of bytes emitted to STDOUT
}

// Job contains a handle to the actual process struct.
type Job struct {
	Cmd                  *exec.Cmd                 // Handle to the forked process
	Proc                 *process.Process          // gopsutil process handle for later stats gathering
	Pid                  int                       // Process ID, not particularlly useful
	Pgid                 int                       // Process Group ID, used for stats/control of the entire tree created
	done                 chan error                // Output for normal exiting of the child
	stdoutByteCountLimit int64                     // Threshold for limiting stdout (0 means infinite)
	stdoutReader         *iocontrol.MeasuredReader // Used by accessor method to get the total for stats
}

// NewControlledProcess creates the child proc.
func NewControlledProcess(cmd string, arguments []string, doneChan chan error, stdoutLimit int64) (JobControl, error) {
	var err error

	j := &Job{
		nil,
		nil,
		0,
		0,
		doneChan,
		stdoutLimit,
		nil,
	}

	// Drop command from cmdline arguments and pass the rest as arguments separately
	var args []string
	if len(arguments) > 0 {
		args = arguments[1:]
	}
	j.Cmd = exec.Command(cmd, args...)

	// Collect stdout from the process to redirect to real stdout
	stdoutpipe, err := j.Cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire stdout: %s", err)
	}
	stdout := iocontrol.NewMeasuredReader(stdoutpipe)
	j.stdoutReader = stdout

	var wg sync.WaitGroup

	stdin, err := j.Cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire stdin: %s", err)
	}

	stderr, err := j.Cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire stderr: %s", err)
	}

	// Map all child processes under this tree so Kill really ends everything.
	j.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Set process group ID
	}

	log.Debugf("%#v\n", j.Cmd)

	// Start the sub-process but don't wait for completion to pickup the Pid
	// for resource monitoring.
	err = j.Cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("Failed to execute sub-process: %s\n", err)
	}

	j.Pid = j.Cmd.Process.Pid
	j.Pgid, err = syscall.Getpgid(j.Pid)
	if err != nil {
		return nil, fmt.Errorf("Failed syscall.Getpgid: %s\n", err)
	}

	j.Proc, err = process.NewProcess(int32(j.Pgid))
	if err != nil {
		return nil, fmt.Errorf("Unable to create process.NewProcess: %s\n", err)
	}

	wg.Add(1)
	go func(wg *sync.WaitGroup, r io.Reader) {
		defer wg.Done()
		io.Copy(os.Stdout, r)
		log.Debugln("child closed stdout")
	}(&wg, stdout)

	go func(w io.WriteCloser) {
		io.Copy(w, os.Stdin)
	}(stdin)

	wg.Add(1)
	go func(wg *sync.WaitGroup, r io.Reader) {
		defer wg.Done()
		io.Copy(os.Stderr, r)
		log.Debugln("child closed stderr")
	}(&wg, stderr)

	// Background waiting for the job to finish and emit a done channel message
	// when complete.
	go func(wg *sync.WaitGroup, j *Job) {
		log.Debugln("Waiting on wg.Wait()")
		wg.Wait()
		log.Debugln("Waiting on Cmd.Wait()")
		err := j.Cmd.Wait()
		log.Debugf("Job finished: %q\n", err)
		j.done <- err
	}(&wg, j)

	return j, nil
}

// Stop gracefully ends the process
func (j *Job) Stop() error {
	err := syscall.Kill(-j.Pgid, syscall.SIGTERM)
	if err != nil {
		log.Warnf("Error received calling stop on sub-process: %s", err)
		return err
	}
	return nil
}

// Resume continues a suspended process
func (j *Job) Resume() error {
	if err := syscall.Kill(-j.Pgid, syscall.SIGCONT); err != nil {
		log.Warnf("Error received calling resume on sub-process: %s", err)
		return err
	}
	return nil
}

// Suspend pauses a running process
func (j *Job) Suspend() error {
	if err := syscall.Kill(-j.Pgid, syscall.SIGSTOP); err != nil {
		log.Warnf("Error received calling suspend on sub-process: %s", err)
		return err
	}
	return nil
}

// Kill forcefully stops a process
func (j *Job) Kill(sig int64) error {
	var err error

	switch sig {
	case -9:
		log.Debugln("Sending process Kill (-9) signal")
		err = syscall.Kill(-j.Pgid, syscall.SIGKILL)
	default:
		signal := syscall.Signal(sig)
		err = syscall.Kill(-j.Pgid, signal)
	}

	if err != nil {
		log.Warnf("Error received calling kill on sub-process: %s", err)
		return err
	}

	return nil
}

// Done returns the channel used when the process is finished
func (j *Job) Done() chan error {
	return j.done
}

// Process returns a handle to the underlying process through gopsutil.
func (j *Job) Process() *process.Process {
	return j.Proc
}

// StdoutByteCount returns the number of emitted bytes to STDOUT by the child process.
func (j *Job) StdoutByteCount() int64 {
	return int64(j.stdoutReader.Total())
}
