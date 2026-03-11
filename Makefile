.PHONY: build build-linux run build-run serve setup chat test test-v lint clean install init help web-install web-build web-dev release release-metadata release-package docker-go-latest

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
RELEASE_LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -extldflags '-static'"
BUILDVCS := -buildvcs=false
CONFIG  ?= $(wildcard config.yaml)
VERBOSE ?=
DIST_DIR := dist

#AWS
S3_BUCKET ?= assets.devclaw.dev

#SonarQube variables
stage=dev
m_gopath=$(shell go env GOPATH 2>/dev/null || echo "/var/go")
repo_name=$(shell egrep -oi "latamd.*" .git/config 2>/dev/null | cut -d\/ -f2 | head -1 | sed 's/.git*//g' || echo "devclaw")
branch=$(shell git branch 2>/dev/null | egrep '^\*' | cut -d" " -f2 || echo "unknown")
commit=$(shell git rev-parse --short HEAD 2> /dev/null | sed "s/\(.*\)/@\1/" || echo "@unknown")


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

## web-build: Build the React frontend (installs deps if needed)
web-build: web-install
	cd web && npm run build
	@# Copy dist into the Go embed directory
	rm -rf pkg/devclaw/webui/dist
	cp -r web/dist pkg/devclaw/webui/dist

## web-dev: Start Vite dev server (proxies /api to :47716)
web-dev:
	cd web && npm run dev

## build: Build the binary (includes frontend if dist/ exists)
build: web-build
	CGO_ENABLED=1 go build $(BUILDVCS) -tags 'sqlite_fts5' $(LDFLAGS) -o bin/devclaw ./cmd/devclaw

## build-go: Build only the Go binary (skip frontend)
build-go:
	CGO_ENABLED=1 go build $(BUILDVCS) -tags 'sqlite_fts5' $(LDFLAGS) -o bin/devclaw ./cmd/devclaw

## build-linux: Cross-compile for Linux AMD64 (for VM deploy)
build-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(BUILDVCS) -tags 'sqlite_fts5' $(LDFLAGS) -o bin/devclaw-linux-amd64 ./cmd/devclaw

## run: Start devclaw serve (uses existing binary)
run:
	./bin/devclaw serve $(SERVE_FLAGS)

## build-run: Build and start devclaw serve
build-run: build
	./bin/devclaw serve $(SERVE_FLAGS)

## serve: Alias for run
serve: run

## dev: Start both Vite dev server and Go server
dev:
	@echo "Starting Go server and Vite dev server..."
	@echo "  Go API: http://localhost:47716"
	@echo "  Vite:   http://localhost:3000"
	@$(MAKE) -j2 build-go web-dev _dev-serve

_dev-serve: build-go
	./bin/devclaw serve $(SERVE_FLAGS)

## setup: Interactive setup wizard
setup: build-go
	./bin/devclaw setup

## init: Create default config.yaml (non-interactive)
init: build-go
	./bin/devclaw config init

## validate: Validate the configuration
validate: build-go
	./bin/devclaw config validate $(if $(CONFIG),--config $(CONFIG))

## chat: Send a single message (usage: make chat MSG="hello")
chat: build-go
	./bin/devclaw chat "$(MSG)"

## test: Run all unit tests
test:
	CGO_ENABLED=1 go test -tags 'sqlite_fts5' -count=1 -race ./pkg/devclaw/copilot/ ./pkg/devclaw/copilot/security/ ./pkg/devclaw/skills/

## test-v: Run all unit tests (verbose)
test-v:
	CGO_ENABLED=1 go test -tags 'sqlite_fts5' -count=1 -race -v ./pkg/devclaw/copilot/ ./pkg/devclaw/copilot/security/ ./pkg/devclaw/skills/

## lint: Run linter
lint:
	golangci-lint run ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/ web/dist/ pkg/devclaw/webui/dist/

## install: Install binary to GOPATH
install: web-build
	CGO_ENABLED=1 go install $(BUILDVCS) -tags 'sqlite_fts5' $(LDFLAGS) ./cmd/devclaw

## docker-build: Build Docker image
docker-build:
	docker compose build

## docker-up: Start via Docker Compose
docker-up:
	docker compose up -d

## docker-down: Stop containers
docker-down:
	docker compose down

## docker-go-latest: Run commands in golang:1.24-bookworm container with Node.js (used by CI/Jenkins)
docker-go-latest:
	docker run \
    --rm \
    -v "${HOME}/.ssh/id_rsa:/root/.ssh/id_rsa" \
    -v "${HOME}/.ssh/known_hosts:/root/.ssh/known_hosts" \
    -v "${PWD}":/app \
    -v "${m_gopath}":/go \
    --workdir="/app" \
    golang:1.24-bookworm \
    bash -c '${COMMAND}'

## release: Build static binary for current platform (includes frontend)
release: web-build
	@echo "=== Building release binary ==="
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 go build $(BUILDVCS) -tags 'sqlite_fts5' $(RELEASE_LDFLAGS) -o $(DIST_DIR)/devclaw ./cmd/devclaw
	@chmod +x $(DIST_DIR)/devclaw

## release-linux: Cross-compile static binary for Linux AMD64 (includes frontend)
release-linux: web-build
	@echo "=== Building Linux AMD64 release binary ==="
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 go build $(BUILDVCS) -tags 'sqlite_fts5' $(RELEASE_LDFLAGS) -o $(DIST_DIR)/devclaw ./cmd/devclaw

## release-metadata: Generate metadata.json for release
release-metadata:
	@mkdir -p $(DIST_DIR)
	@echo '{"version": "$(VERSION)", "binary": "devclaw", "platform": "unix", "updated": "$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"}' > $(DIST_DIR)/metadata.json

## release-package: Create distributable .zip packages (versioned + latest) using Docker
release-package:
	@make docker-go-latest COMMAND='\
	rm -rf dist/ bin/ web/dist/ pkg/devclaw/webui/dist/ && \
	apt-get update && apt-get install -y curl zip && \
	curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
	apt-get install -y nodejs && \
	make release VERSION=$(VERSION) && \
	make release-metadata VERSION=$(VERSION) && \
	cp install/unix/ecosystem.config.js dist/ && \
	cp install/unix/install.sh dist/ && \
	chmod +x dist/install.sh && \
	cd dist && \
	zip devclaw-$(VERSION).zip devclaw ecosystem.config.js metadata.json && \
	cp devclaw-$(VERSION).zip latest.zip && \
	echo "$(VERSION)" > latest.txt && \
	echo "" && \
	echo "=== Verification ===" && \
	echo "Versioned zip:" && \
	unzip -l devclaw-$(VERSION).zip && \
	echo "" && \
	echo "Latest zip:" && \
	unzip -l latest.zip && \
	echo "" && \
	echo "MD5 checksums:" && \
	md5sum devclaw-$(VERSION).zip latest.zip && \
	rm -f devclaw ecosystem.config.js metadata.json && \
	chown -R $(shell id -u):$(shell id -g) /app/dist && \
	echo "" && \
	echo "Packages created:" && \
	echo "  - dist/devclaw-$(VERSION).zip (versioned)" && \
	echo "  - dist/latest.zip (points to $(VERSION))" && \
	echo "  - dist/latest.txt (contains: $(VERSION))" && \
	echo "  - dist/install.sh (separate install script)"'
	@echo ""
	@echo "Release packages created in dist/"

## help: Show available commands
help:
	@echo "Usage:"
	@echo "  make setup             # Interactive setup wizard"
	@echo "  make run               # Start server (uses existing binary)"
	@echo "  make build-run         # Build frontend + Go + serve"
	@echo "  make dev               # Dev mode (Vite HMR + Go server)"
	@echo "  make run VERBOSE=1     # Serve with debug logs"
	@echo "  make run CONFIG=x.yaml # Serve with specific config"
	@echo "  make init              # Create default config.yaml (non-interactive)"
	@echo "  make validate          # Validate configuration"
	@echo "  make chat MSG=\"hello\"  # Send a single message"
	@echo ""
	@echo "Frontend:"
	@echo "  make web-install       # Install npm dependencies"
	@echo "  make web-build         # Build React frontend"
	@echo "  make web-dev           # Start Vite dev server"
	@echo ""
	@echo "Release:"
	@echo "  make release           # Build binary for distribution"
	@echo "  make release-package   # Create .zip package"
	@echo ""
	@echo "All commands:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | sort

sonar-scan:
	docker run --network="host" --rm -v "${PWD}:/usr/src" -v "${HOME}/.sonar/":"/opt/sonar-scanner/.sonar/" \
	--dns 8.8.8.8 \
	sonarsource/sonar-scanner-cli \
	  -Dsonar.projectKey=${repo_name} \
	  -Dsonar.login=${SONAR_LOGIN} \
	  -Dsonar.projectVersion="${commit} - ${branch}"

s3-deploy:
	@echo "=== Uploading versioned files (immutable, long cache) ==="
	docker run --rm \
    --env AWS_ACCESS_KEY_ID \
    --env AWS_SECRET_ACCESS_KEY \
    --env AWS_SESSION_TOKEN \
    -v "${PWD}:/app" \
    --workdir="/app" \
	amazon/aws-cli:latest s3 sync dist s3://${S3_BUCKET}/ \
	  --exclude "latest.zip" --exclude "latest.txt" --exclude "metadata.json" --exclude "install.sh" \
	  --cache-control "public, max-age=31536000, immutable"
	@echo "=== Uploading mutable files (no cache) ==="
	docker run --rm \
    --env AWS_ACCESS_KEY_ID \
    --env AWS_SECRET_ACCESS_KEY \
    --env AWS_SESSION_TOKEN \
    -v "${PWD}:/app" \
    --workdir="/app" \
	amazon/aws-cli:latest s3 sync dist s3://${S3_BUCKET}/ \
	  --exclude "*" --include "latest.zip" --include "latest.txt" --include "metadata.json" --include "install.sh" \
	  --cache-control "no-cache, no-store, must-revalidate"
