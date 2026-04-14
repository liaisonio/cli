BINARY := liaison
MODULE := github.com/liaison-cloud/cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/internal/cli.Version=$(VERSION)

.PHONY: build install test clean tidy fmt vet

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/liaison

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/liaison

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/
