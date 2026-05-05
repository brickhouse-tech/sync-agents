VERSION := $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' package.json | head -1)
LDFLAGS := -ldflags "-X github.com/brickhouse-tech/sync-agents/internal/version.Version=$(VERSION)"

# Node-style triples that match the optionalDependencies in package.json
# and the directories under npm/.
PLATFORMS := darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64

.PHONY: install build build-platform build-all test test-go clean

install:
	cd go && go mod tidy -e && go mod vendor

build:
	cd go && go build $(LDFLAGS) -o ../bin/sync-agents ./cmd/sync-agents/

# Cross-compile a single platform binary into npm/$(PLATFORM)/bin/. Invoked
# by each platform package's `prepack` script via scripts/build-platform.js
# so `npm publish` (or `npm pack`) on a platform package builds its binary
# automatically. PLATFORM must be one of $(PLATFORMS).
build-platform: install
	@if [ -z "$(PLATFORM)" ]; then echo "PLATFORM= is required (one of: $(PLATFORMS))"; exit 1; fi
	@triple="$(PLATFORM)"; \
	 node_os=$${triple%%-*}; \
	 node_arch=$${triple##*-}; \
	 case "$$node_os" in \
	   win32) goos=windows; ext=.exe ;; \
	   *)     goos=$$node_os; ext= ;; \
	 esac; \
	 case "$$node_arch" in \
	   x64) goarch=amd64 ;; \
	   *)   goarch=$$node_arch ;; \
	 esac; \
	 out=npm/$$triple/bin/sync-agents$$ext; \
	 echo "==> $$triple ($$goos/$$goarch) -> $$out"; \
	 mkdir -p npm/$$triple/bin; \
	 (cd go && CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch \
	   go build $(LDFLAGS) -o ../$$out ./cmd/sync-agents/)

# Convenience: build every platform's binary in one shot (e.g. for local
# verification). CI relies on each platform package's prepack instead.
build-all:
	@for p in $(PLATFORMS); do $(MAKE) build-platform PLATFORM=$$p; done

test: build
	SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats

test-go: test

clean:
	rm -f bin/sync-agents
	rm -rf npm/*/bin
