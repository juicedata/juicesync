.PHONY: juicesync all
export GO111MODULE=on

all: juicesync

REPO := github.com/juicedata/juicesync
REVISION := $(shell git rev-parse --short HEAD || unknown)
REVISIONDATE := $(shell git log -1 --pretty=format:'%ad' --date short)
VERSION := $(shell git describe --tag)
LDFLAGS ?= -s -w -X $(REPO)/versioninfo.REVISION=$(REVISION) \
		        -X $(REPO)/versioninfo.REVISIONDATE=$(REVISIONDATE) \
		        -X $(REPO)/versioninfo.VERSION=$(VERSION)

juicesync:
	go build -ldflags="$(LDFLAGS)" -o juicesync

juicesync.exe: *.go utils/*.go versioninfo/*.go
	GOOS=windows go build -ldflags="$(LDFLAGS)" -buildmode exe -o juicesync.exe

juicesync.linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o juicesync

test:
	go test ./...
