# ==============================================================================
# Stage 1: Build Web Frontend (React 19 + Tailwind CSS)
# ==============================================================================
FROM node:25-alpine AS web-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ==============================================================================
# Stage 2: Build Go Single Binary with Embedded Web Assets
# ==============================================================================
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

# Copy source code and embedded web dist
COPY internal/ internal/
COPY cmd/ cmd/
COPY configs/ configs/
COPY web/web.go web/web.go
COPY --from=web-builder /app/web/dist web/dist

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/airoute ./cmd/airoute

# ==============================================================================
# Stage 3: Minimal Production Image
# ==============================================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S gateway && adduser -S gateway -G gateway

WORKDIR /app

COPY --from=go-builder /bin/airoute /app/airoute
COPY configs/config.yaml /app/configs/config.yaml

RUN mkdir -p /app/data && chown -R gateway:gateway /app

USER gateway

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:${GATEWAY_PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/airoute"]
CMD ["-config", "/app/configs/config.yaml", "-db", "/app/data/gateway.db"]
