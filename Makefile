BINDIR ?= $(shell go env GOPATH)/bin

.PHONY: install test

install:
	go build -o "$(BINDIR)/work" .

test:
	go test ./...
