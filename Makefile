.PHONY: all build test test-race bench install clean fmt vet

BINARY_NAME=autologin
BUILD_DIR=dist
VERSION ?= 2.2.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: test build

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./src

install:
	go build -ldflags="$(LDFLAGS)" -o ~/.local/bin/$(BINARY_NAME) ./src

test:
	go test -v ./...

test-race:
	go test -race ./...

bench:
	go test -bench=. -benchmem -run=^$$ ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
