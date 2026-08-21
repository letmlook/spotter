.PHONY: test build agent client clean

GO ?= go
GOFLAGS ?= -trimpath

test:
	$(GO) test ./... -race -count=1

build: agent client

agent:
	$(GO) build $(GOFLAGS) -o bin/spotterd ./cmd/agent

agent-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) \
		-o bin/spotterd-linux-arm64 ./cmd/agent

client:
	$(GO) build $(GOFLAGS) -o bin/spotter-client ./cmd/client

clean:
	rm -rf bin/