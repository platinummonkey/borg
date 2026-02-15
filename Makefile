.PHONY: build build-agent build-provision provision test test-integration test-e2e test-podman fmt vet lint run run-example clean

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Go settings
GOBIN := bin
BINARY := $(GOBIN)/borg
PROVISION_BINARY := $(GOBIN)/assimilate
MANAGER_BINARY := $(GOBIN)/queen

build: build-agent build-provision build-manager

build-agent:
	@mkdir -p $(GOBIN)
	go build $(LDFLAGS) -o $(BINARY) ./cmd/agent

provision build-provision:
	@mkdir -p $(GOBIN)
	go build $(LDFLAGS) -o $(PROVISION_BINARY) ./cmd/provision

build-manager:
	@mkdir -p $(GOBIN)
	go build $(LDFLAGS) -o $(MANAGER_BINARY) ./cmd/manager

run: build-agent
	$(BINARY) $(ARGS)

test:
	go test -race -count=1 ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

test-integration:
	go test -race -tags=integration -count=1 ./test/integration/...

test-e2e:
	go test -race -tags=integration -count=1 -v ./test/integration/...

test-podman:
	go test -race -tags=podman -count=1 -v -timeout=120s ./test/integration/...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

run-example:
	go run ./examples/multi-agent/

clean:
	rm -rf $(GOBIN) coverage.out

.PHONY: build-manager
