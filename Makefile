VERSION=`git describe --tags`
BUILDDATE=`date +%FT%T%z`

LINK_FLAGS=-ldflags "-w -s -X main.VERSION=${VERSION} -X main.BUILDDATE=${BUILDDATE}"
build:
	go build ${LINK_FLAGS}

install:
	go install ${LINK_FLAGS}
