package agents

import (
	"fmt"
	"github.com/shirou/gopsutil/process"
	"os/exec"
)

type Job struct {
	Cmd  *exec.Cmd
	Proc *process.Process
	Done chan error
}

// TODO: Add log limiting support by byte counting stdout
// NewControlledProcess creates the child proc.
func NewControlledProcess(cmd string, arguments []string) (*Job, error) {
	c := &Job{}

	c.Cmd = exec.Command(cmd)
	c.Cmd.Args = arguments

	pid := int32(c.Cmd.Process.Pid)

	var err error
	c.Proc, err = process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("Unable to create process.NewProcess: %s", err)
	}

	return c, nil
}
