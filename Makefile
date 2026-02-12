.PHONY: build run test lint clean install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

## build: Compila o binário
build:
	go build $(LDFLAGS) -o bin/copilot ./cmd/copilot

## run: Compila e executa o copilot serve
run: build
	./bin/copilot serve

## chat: Compila e executa uma mensagem de chat
chat: build
	./bin/copilot chat "$(MSG)"

## test: Executa os testes
test:
	go test ./... -v -race

## lint: Executa o linter
lint:
	golangci-lint run ./...

## clean: Remove artefatos de build
clean:
	rm -rf bin/ dist/

## install: Instala o binário
install:
	go install $(LDFLAGS) ./cmd/copilot

## docker-build: Build da imagem Docker
docker-build:
	docker compose build

## docker-up: Inicia via Docker Compose
docker-up:
	docker compose up -d

## docker-down: Para os containers
docker-down:
	docker compose down

## help: Exibe esta mensagem de ajuda
help:
	@echo "Comandos disponíveis:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | sort
