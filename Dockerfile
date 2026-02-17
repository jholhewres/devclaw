# ── Frontend build stage ──
FROM node:22-alpine AS frontend

WORKDIR /app/web

# Cache de dependências npm
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund

# Build da SPA React
COPY web/ .
RUN npm run build

# ── Go build stage ──
FROM golang:1.24-alpine AS builder

RUN apk --no-cache add ca-certificates git gcc musl-dev

WORKDIR /app

# Cache de dependências Go
COPY go.mod go.sum ./
RUN go mod download

# Copiar código Go
COPY . .

# Copiar dist do frontend para o embed directory
COPY --from=frontend /app/web/dist ./pkg/devclaw/webui/dist/

# Build do binário com SQLite FTS5
RUN CGO_ENABLED=1 GOOS=linux go build \
    -tags 'sqlite_fts5' \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o copilot ./cmd/copilot

# ── Runtime stage ──
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

# Cria usuário não-root para segurança
RUN addgroup -S copilot && adduser -S copilot -G copilot

USER copilot
WORKDIR /home/copilot

# Copia binário e config de exemplo
COPY --from=builder /app/copilot /usr/local/bin/copilot
COPY --from=builder /app/configs/copilot.example.yaml /etc/copilot/config.example.yaml

# Volumes para persistência de sessões e dados
VOLUME ["/home/copilot/sessions", "/home/copilot/data"]

# Expor portas: gateway (8080) e webui (8090)
EXPOSE 8080 8090

# Health check via comando `copilot health`.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["copilot", "health"]

ENTRYPOINT ["copilot"]
CMD ["serve", "--config", "/etc/copilot/config.yaml"]
