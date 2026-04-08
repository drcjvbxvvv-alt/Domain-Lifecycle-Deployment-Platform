BINARY_DIR  := bin
MODULE      := domain-platform
GO          := go
GOFLAGS     := -trimpath

.PHONY: all build server worker scanner migrate test lint web migrate-up migrate-down clean dev

all: build

## ── Build ────────────────────────────────────────────────────────────────────
build: server worker scanner migrate

server:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/server ./cmd/server

worker:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/worker ./cmd/worker

scanner:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/scanner-linux-amd64 ./cmd/scanner

migrate:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/migrate ./cmd/migrate

## ── Test & Lint ──────────────────────────────────────────────────────────────
test:
	$(GO) test ./... -race -timeout 60s

lint:
	golangci-lint run ./...

## ── Database ─────────────────────────────────────────────────────────────────
migrate-up:
	$(BINARY_DIR)/migrate up

migrate-down:
	$(BINARY_DIR)/migrate down

## ── Frontend ─────────────────────────────────────────────────────────────────
web:
	cd web && npm run build

## ── Dev ──────────────────────────────────────────────────────────────────────
dev:
	air -c .air.toml

## ── Cleanup ──────────────────────────────────────────────────────────────────
clean:
	rm -rf $(BINARY_DIR)
