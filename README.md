# proc_box

proc_box is an open source, way of containerizing a process into a resource
limited box suitable for batch systems.  The primary goal is to limit resource
explosion and causing other users of the batch system to receive unexpected
resource pressure.  This project also provides remote control of the process
through AMQP JSON messages.  Also, proc_box will emit statistical usage
measurements of the contained process for analysis by other tools.

[![Build Status](https://travis-ci.org/nickjones/proc_box.svg)](https://travis-ci.org/Masterminds/glide) [![Go Report Card](http://goreportcard.com/badge/nickjones/proc_box)](http://goreportcard.com/report/nickjones/proc_box)

## Features
This project is still very much alpha and likely is unusable in it's current
state, however...
- Connects to an AMQP broker
- Starts the contained process
- Gracefully exits with either a simple remote control JSON or the process
naturally existing.

## Requirements
Go; that's about it.  We suggest the rabbitmq:3-management Docker container for
development purposes.

## Running
Generally, only specifying both the AMQP broker and the command to be contained
is necessary.
```
./proc_box -uri <amqp_uri> <cmd> <args>
```

## License
proc_box is licensed under the MIT License.
