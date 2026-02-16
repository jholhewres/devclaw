.PHONY: build run serve setup chat test test-v lint clean install init help web-install web-build web-dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
CONFIG  ?= $(wildcard config.yaml)
VERBOSE ?=

# Build flags for serve
SERVE_FLAGS :=
ifneq ($(CONFIG),)
  SERVE_FLAGS += --config $(CONFIG)
endif
ifneq ($(VERBOSE),)
  SERVE_FLAGS += -v
endif

## web-install: Install frontend dependencies
web-install:
	cd web && npm install

## web-build: Build the React frontend
web-build:
	cd web && npm run build
	@# Copy dist into the Go embed directory
	rm -rf pkg/goclaw/webui/dist
	cp -r web/dist pkg/goclaw/webui/dist

## web-dev: Start Vite dev server (proxies /api to :8090)
web-dev:
	cd web && npm run dev

## build: Build the binary (includes frontend if dist/ exists)
build: web-build
	CGO_ENABLED=1 go build -tags 'sqlite_fts5' $(LDFLAGS) -o bin/copilot ./cmd/copilot

## build-go: Build only the Go binary (skip frontend)
build-go:
	CGO_ENABLED=1 go build -tags 'sqlite_fts5' $(LDFLAGS) -o bin/copilot ./cmd/copilot

## run: Build and start copilot serve
run: build
	./bin/copilot serve $(SERVE_FLAGS)

## serve: Alias for run
serve: run

## dev: Start both Vite dev server and Go server
dev:
	@echo "Starting Go server and Vite dev server..."
	@echo "  Go API: http://localhost:8090"
	@echo "  Vite:   http://localhost:3000"
	@$(MAKE) -j2 build-go web-dev _dev-serve

_dev-serve: build-go
	./bin/copilot serve $(SERVE_FLAGS)

## setup: Interactive setup wizard
setup: build-go
	./bin/copilot setup

## init: Create default config.yaml (non-interactive)
init: build-go
	./bin/copilot config init

## validate: Validate the configuration
validate: build-go
	./bin/copilot config validate $(if $(CONFIG),--config $(CONFIG))

## chat: Send a single message (usage: make chat MSG="hello")
chat: build-go
	./bin/copilot chat "$(MSG)"

## test: Run all unit tests
test:
	CGO_ENABLED=1 go test -tags 'sqlite_fts5' -count=1 -race ./pkg/goclaw/copilot/ ./pkg/goclaw/copilot/security/ ./pkg/goclaw/skills/

## test-v: Run all unit tests (verbose)
test-v:
	CGO_ENABLED=1 go test -tags 'sqlite_fts5' -count=1 -race -v ./pkg/goclaw/copilot/ ./pkg/goclaw/copilot/security/ ./pkg/goclaw/skills/

## lint: Run linter
lint:
	golangci-lint run ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ web/dist/ pkg/goclaw/webui/dist/

## install: Install binary to GOPATH
install: web-build
	CGO_ENABLED=1 go install -tags 'sqlite_fts5' $(LDFLAGS) ./cmd/copilot

## docker-build: Build Docker image
docker-build:
	docker compose build

## docker-up: Start via Docker Compose
docker-up:
	docker compose up -d

## docker-down: Stop containers
docker-down:
	docker compose down

## help: Show available commands
help:
	@echo "Usage:"
	@echo "  make setup             # Interactive setup wizard"
	@echo "  make run               # Build frontend + Go + serve"
	@echo "  make dev               # Dev mode (Vite HMR + Go server)"
	@echo "  make run VERBOSE=1     # Build + serve with debug logs"
	@echo "  make run CONFIG=x.yaml # Build + serve with specific config"
	@echo "  make init              # Create default config.yaml (non-interactive)"
	@echo "  make validate          # Validate configuration"
	@echo "  make chat MSG=\"hello\"  # Send a single message"
	@echo ""
	@echo "Frontend:"
	@echo "  make web-install       # Install npm dependencies"
	@echo "  make web-build         # Build React frontend"
	@echo "  make web-dev           # Start Vite dev server"
	@echo ""
	@echo "All commands:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | sort
