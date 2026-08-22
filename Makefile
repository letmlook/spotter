.PHONY: test build agent agent-all agent-linux-arm64 agent-linux-x64 client release clean

GO ?= go
GOFLAGS ?= -trimpath

test:
	$(GO) test ./... -race -count=1

build: agent client

# Native build for the host OS/arch (handy for `go run ./cmd/agent`
# during development on Linux/macOS dev boxes).
agent:
	$(GO) build $(GOFLAGS) -o bin/spotterd ./cmd/agent

# All supported device targets. spotterd runs on any Linux with
# systemd; the two matrix entries below cover the deployed fleet
# (ARM64 SBCs + x86_64 servers/workstations).
agent-all: agent-linux-arm64 agent-linux-x64

agent-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) \
		-o bin/spotterd-linux-arm64 ./cmd/agent

agent-linux-x64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-o bin/spotterd-linux-x64 ./cmd/agent

WAILS := $(shell command -v wails 2>/dev/null)

# spotter-client is a Wails desktop GUI. The same source produces
# native binaries for Windows, macOS and Linux; the active GOOS at
# build time selects which.
client:
ifneq ($(WAILS),)
	$(WAILS) build
else
	@echo "warning: wails CLI not found; falling back to 'go build' (will NOT produce a .app bundle on macOS)" >&2
	$(GO) build $(GOFLAGS) -o bin/spotter-client .
endif

clean:
	rm -rf bin/

# One-click cross-platform release build. Wraps scripts/build-all.sh
# which produces both Linux device binaries and (best-effort) client
# builds for every Wails target, then packages everything under dist/.
# For flag-forwarding variants (`--agent-only`, `--host-only`,
# `--no-cross`), invoke the script directly:
#
#   ./scripts/build-all.sh --agent-only
#
release:
	./scripts/build-all.sh