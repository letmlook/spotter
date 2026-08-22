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

WAILS := $(shell command -v wails 2>/dev/null)

client:
ifneq ($(WAILS),)
	$(WAILS) build
else
	@echo "warning: wails CLI not found; falling back to 'go build' (will NOT produce a .app bundle on macOS)" >&2
	$(GO) build $(GOFLAGS) -o bin/spotter-client .
endif

clean:
	rm -rf bin/