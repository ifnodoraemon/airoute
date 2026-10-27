# ==============================================================================
# Base images
#
# Tags are declared in one place so a version bump is a single-line change and CI
# can see exactly which upstream images the shipping artifact is built from.
# They are intentionally floating minor tags (`node:22-alpine`) for reproducible
# local builds: pinning a digest here would break `docker build` for anyone whose
# local/cached image was pulled from a different platform digest. For a fully
# reproducible release, CI can override them with digest-pinned values:
#   docker build --build-arg NODE_IMAGE=node:22-alpine@sha256:... .
# ==============================================================================
ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.26-alpine
ARG RUNTIME_IMAGE=alpine:3.24

# ==============================================================================
# Stage 1: Build Web Frontend (React 19 + Tailwind CSS)
# ==============================================================================
FROM ${NODE_IMAGE} AS web-builder

WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ==============================================================================
# Stage 2: Build Go Single Binary with Embedded Web Assets
# ==============================================================================
FROM ${GO_IMAGE} AS go-builder

# Injected so the binary reports the same version as the image tag it is
# published under (see .github/workflows/docker.yml). The default keeps a plain
# `docker build .` working without build arguments.
ARG VERSION=1.1.0-dev
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

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

# Build statically linked binary.
#
# -trimpath removes the build machine's absolute paths from the binary (and the
# panic/stack traces it emits), so the same source produces the same binary
# regardless of where it was built.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o /bin/airoute ./cmd/airoute

# ==============================================================================
# Stage 3: Minimal Production Image
# ==============================================================================
FROM ${RUNTIME_IMAGE}

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S gateway && adduser -S gateway -G gateway

WORKDIR /app

COPY --from=go-builder /bin/airoute /app/airoute
COPY configs/config.yaml /app/configs/config.yaml

RUN mkdir -p /app/data && chown -R gateway:gateway /app

USER gateway

EXPOSE 8080

# Readiness, not bare liveness: Docker exposes a single health status and
# `docker compose`/scripts/docker-rebuild.sh gate rollouts on it, so it must reflect
# whether this replica can reach its storage/Redis.
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=3 \
  CMD curl -f http://127.0.0.1:${GATEWAY_PORT:-8080}/health/ready || exit 1

ENTRYPOINT ["/app/airoute"]
CMD ["-config", "/app/configs/config.yaml", "-db", "/app/data/gateway.db"]
