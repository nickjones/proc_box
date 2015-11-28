# proc_box

proc_box is an open source, way of containerizing a process into a resource
limited box suitable for batch systems.  The primary goal is to limit resource
explosion and causing other users of the batch system to receive unexpected
resource pressure.  This project also provides remote control of the process
through AMQP JSON messages.  Also, proc_box will emit statistical usage
measurements of the contained process for analysis by other tools.

[![Build Status](https://travis-ci.org/nickjones/proc_box.svg)](https://travis-ci.org/nickjones/proc_box)
[![GoDoc](https://godoc.org/github.com/nickjones/proc_box?status.svg)](https://godoc.org/github.com/nickjones/proc_box)

[![Go Report Card](http://goreportcard.com/badge/nickjones/proc_box)](http://goreportcard.com/report/nickjones/proc_box)
[![Average time to resolve an issue](http://isitmaintained.com/badge/resolution/nickjones/proc_box.svg)](http://isitmaintained.com/project/nickjones/proc_box "Average time to resolve an issue")
[![Percentage of issues still open](http://isitmaintained.com/badge/open/nickjones/proc_box.svg)](http://isitmaintained.com/project/nickjones/proc_box "Percentage of issues still open")

## Features
- Connects to an AMQP broker
- Starts the contained process
- Gracefully exits with either a simple remote control JSON or the process
naturally existing.
- Basic suspend/resume/quit/kill remote control support.
- Controllable wall-clock timer to end runaway processes
(also controlled over AMQP).
- Captures SIGINT/etc. at proc_box level and issues appropriate signals to child
process.
- Periodic statistical samples of process usage (aggregated parent and children)
emitted on AMQP.
- Arguments directly on the command line or provided through environment variables.
Command line overrides environment variables.

## Requirements
Go; that's about it.  We suggest the rabbitmq:3-management Docker container for
development purposes.

## Documentation
Use [Godoc documentation](https://godoc.org/github.com/nickjones/proc_box/agents) under
the agents package for reference.

## Running
Generally, only specifying both the AMQP broker and the command to be contained
is necessary.
```
./proc_box -uri <amqp_uri> <cmd> <args>
```

### Remote Commands
Remote commands can be issued through the AMQP broker to the topic exchange specified
(or default proc_box.remote_control).  Messages should be in JSON of the form:
```
{
    "command": "<cmd>",
    "arguments": ["<arg1>", "<arg2>", ...],
}
```
Supported commands:
* **stop**: Issue SIGQUIT to the process.
* **resume**: Resume suspended process execution (and timeout timer).
* **suspend**: Suspend the process execution.
* **kill**: Issue SIGKILL to the process.  Optional single argument is the signal to send (9 is default).
* **sample**: Force a process statistics sample.
* **timer_reset**: Disables timer and sets ElapsedTime to zero.
* **timer_start**: Enables timer ticking from an ElapsedTime of zero.
* **timer_stop**: Disables timer
* **timer_resume**: Continues ticking time without resetting.

## License
proc_box is licensed under the MIT License.
