.PHONY: build test test-integration fmt vet lint run clean

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go settings
GOBIN := bin
BINARY := $(GOBIN)/agent-chat

build:
	@mkdir -p $(GOBIN)
	go build $(LDFLAGS) -o $(BINARY) ./cmd/agent

run: build
	$(BINARY) $(ARGS)

test:
	go test -race -count=1 ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -tags=integration -count=1 ./test/integration/...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

clean:
	rm -rf $(GOBIN) coverage.out
