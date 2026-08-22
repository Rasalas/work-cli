BINDIR ?= $(shell go env GOPATH)/bin

.PHONY: install build test lint fmt vet clean

install:
	go build -o "$(BINDIR)/work" .

build:
	go build -o work .

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -l -w .

vet:
	go vet ./...

clean:
	rm -rf dist work
