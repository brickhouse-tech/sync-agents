VERSION := $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' package.json | head -1)
LDFLAGS := -ldflags "-X github.com/brickhouse-tech/sync-agents/internal/version.Version=$(VERSION)"

# Node-style triple → (GOOS, GOARCH, exe-suffix). Keep this list in sync
# with the optionalDependencies in package.json and the directories under
# npm/.
PLATFORMS := darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64

.PHONY: install build build-all dist test test-go clean

install:
	cd go && go mod tidy -e && go mod vendor

build:
	cd go && go build $(LDFLAGS) -o ../bin/sync-agents ./cmd/sync-agents/

# Cross-compile a binary for every supported triple into the matching
# npm/<triple>/bin/ directory so each platform package is ready to publish.
build-all:
	@set -e; for triple in $(PLATFORMS); do \
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
			go build $(LDFLAGS) -o ../$$out ./cmd/sync-agents/); \
	done

# Sync each platform package's version field with the root package.json so
# `npm publish` in those subdirs ships the right version. Idempotent.
dist: build-all
	@for triple in $(PLATFORMS); do \
		node -e "const f='npm/$$triple/package.json'; const p=require('fs').readFileSync(f,'utf8'); const j=JSON.parse(p); j.version='$(VERSION)'; require('fs').writeFileSync(f, JSON.stringify(j,null,2)+'\n');"; \
	done

test: build
	SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats

test-go: test

clean:
	rm -f bin/sync-agents
	rm -rf npm/*/bin
