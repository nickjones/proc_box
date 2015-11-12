package agents

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/shirou/gopsutil/process"
)

// JobControl provides a command interface for control
type JobControl interface {
	Start() error
	Stop() error
	Suspend() error
	Resume() error
	Kill(int64) error
	Done() chan error
	Process() *process.Process
}

// Job contains a handle to the actual process struct.
type Job struct {
	JobControl
	Cmd  *exec.Cmd
	Proc *process.Process
	done chan error
}

// NewControlledProcess creates the child proc.
// TODO: Add log limiting support by byte counting stdout
func NewControlledProcess(cmd string, arguments []string, doneChan chan error) (JobControl, error) {
	j := &Job{
		nil,
		nil,
		nil,
		doneChan,
	}

	j.Cmd = exec.Command(cmd)

	// Collect stdout from the process to redirect to real stdout
	stdout, err := j.Cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire stdout: %s", err)
	}
	stderr, err := j.Cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to acquire stderr: %s", err)
	}

	// Map all child processes under this tree so Kill really ends everything.
	j.Cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	j.Cmd.Args = arguments
	log.Debugf("%#v\n", j.Cmd)

	// Start the sub-process but don't wait for completion to pickup the Pid
	// for resource monitoring.
	err = j.Cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("Failed to execute sub-process: %s\n", err)
	}

	pid := int32(j.Cmd.Process.Pid)

	j.Proc, err = process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("Unable to create process.NewProcess: %s\n", err)
	}

	go stdRedirect(stdout)
	go stdRedirect(stderr)

	// Background waiting for the job to finish and emit a done channel message
	// when complete.
	go func(j *Job) {
		err := j.Cmd.Wait()
		log.Debugf("Job finished: %q\n", err)
		j.done <- err
	}(j)

	return j, nil
}

func stdRedirect(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Printf("%s\n", scanner.Text())
	}
}

// Stop gracefully ends the process
func (j *Job) Stop() error {
	if err := j.Proc.Terminate(); err != nil {
		log.Warnf("Error received calling terminate on sub-process: %s\n", err)
		return err
	}
	return nil
}

// Resume continues a suspended process
func (j *Job) Resume() error {
	if err := j.Proc.Resume(); err != nil {
		log.Warnf("Error received calling resume on sub-process: %s", err)
		return err
	}
	return nil
}

// Suspend pauses a running process
func (j *Job) Suspend() error {
	if err := j.Proc.Suspend(); err != nil {
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
		err = j.Proc.Kill()
	default:
		signal := syscall.Signal(sig)
		err = j.Proc.SendSignal(signal)
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
