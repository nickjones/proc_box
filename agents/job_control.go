package agents

import (
	"fmt"
	"os/exec"

	log "github.com/Sirupsen/logrus"
	"github.com/shirou/gopsutil/process"
)

// JobControl provides a command interface for control
type JobControl interface {
	Stop() error
	Suspend() error
	Resume() error
	Kill() error
	Done() chan error
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
	j.Cmd.Args = arguments
	log.Debugf("%#v\n", j.Cmd)

	// Start the sub-process but don't wait for completion to pickup the Pid
	// for resource monitoring.
	err := j.Cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("Failed to execute sub-process: %s", err)
	}

	pid := int32(j.Cmd.Process.Pid)

	j.Proc, err = process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("Unable to create process.NewProcess: %s", err)
	}

	// Background waiting for the job to finish and emit a done channel message
	// when complete.
	go func(j *Job) {
		err := j.Cmd.Wait()
		log.Debugf("Job finished: %q\n", err)
		j.done <- err
	}(j)

	return j, nil
}

// Stop gracefully ends the process
func (j *Job) Stop() error {
	// TODO: Implement me!
	return nil
}

// Resume continues a suspended process
func (j *Job) Resume() error {
	// TODO: Implement me!
	return nil
}

// Suspend pauses a running process
func (j *Job) Suspend() error {
	// TODO: Implement me!
	return nil
}

// Kill forcefully stops a process
func (j *Job) Kill() error {
	// TODO: Implement me!
	return nil
}

// Done returns the channel used when the process is finished
func (j *Job) Done() chan error {
	return j.done
}
