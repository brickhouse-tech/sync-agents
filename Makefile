VERSION := $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' package.json | head -1)
LDFLAGS := -ldflags "-X github.com/brickhouse-tech/sync-agents/internal/version.Version=$(VERSION)"

.PHONY: build test test-go clean

install:
	cd go && go mod tidy -e && go mod vendor

build:
	cd go && go build $(LDFLAGS) -o ../bin/sync-agents ./cmd/sync-agents/

test: build
	SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats

clean:
	rm -f bin/sync-agents
