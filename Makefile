build:
	cd go && go build -o ../bin/sync-agents ./cmd/sync-agents/

test:
	npx bats test/sync-agents.bats

test-go: build
	SYNC_AGENTS_BIN=bin/sync-agents npx bats test/sync-agents.bats
