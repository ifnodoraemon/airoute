export PATH := /usr/local/go/bin:$(PATH)

.PHONY: all build build-web run test test-web test-integration test-e2e test-e2e-go tidy clean docker-build docker-rebuild check-deploy-drift

APP_NAME = airoute
BIN_DIR = bin
ENTRY = ./cmd/airoute
CONFIG = configs/config.yaml

# Resolve releases and development builds consistently with CI.
VERSION ?= $(shell sh scripts/build-version.sh)
LDFLAGS = -X main.Version=$(VERSION)

all: build

build-web: web/node_modules
	@echo "==> Building React Web UI..."
	@cd web && npm run build

# Declared as file targets so a fresh clone bootstraps itself in the right order
# (dependencies, then the bundle) and repeat runs do nothing.
web/node_modules: web/package.json web/package-lock.json
	@echo "==> Installing Web UI dependencies..."
	@cd web && npm ci

# The gateway embeds the console (see web/web.go) and dist is not tracked, so it
# must exist before anything compiles the Go packages. `build` and `run` rebuild
# the UI unconditionally so a stale bundle is never shipped or served; the Go
# tests only need the package to compile, so they use this file target.
web/dist/index.html: web/node_modules
	@echo "==> Web UI is not built yet; building it before the Go build..."
	@cd web && npm run build

build: build-web
	@mkdir -p $(BIN_DIR)
	@echo "==> Building $(APP_NAME) single binary ($(VERSION))..."
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) $(ENTRY)

run: build-web
	@echo "==> Running $(APP_NAME)..."
	go run -ldflags "$(LDFLAGS)" $(ENTRY) -config $(CONFIG)

# Docker cluster targets. The image bakes in $(VERSION) the same way CI does
# (see x-gateway-build in docker-compose.yml), so a bare `docker compose build`
# reports the development default while these targets stay in sync with the
# checkout. `docker-rebuild` also rolls both replicas one at a time.
docker-build:
	@echo "==> Building Docker image ($(VERSION))..."
	@VERSION="$(VERSION)" docker compose build

docker-rebuild:
	@sh scripts/docker-rebuild.sh

# Read-only: detects postgres/redis/lb containers that predate the compose
# ulimits/command. Run after an upgrade; it never mutates anything.
check-deploy-drift:
	@echo "==> Checking dependency container drift..."
	@sh scripts/check-deploy-drift.sh

test: web/dist/index.html
	@echo "==> Running tests..."
	go test -v -race $$(go list ./... | grep -v '/web/node_modules')

test-web:
	@echo "==> Running Web UI tests..."
	@cd web && npm run test

# Integration tests need a real PostgreSQL and Redis. They skip themselves when
# the DSNs below are unset, so this target checks first rather than reporting a
# misleading pass. Start the dependencies with:
#   docker run -d -p 5432:5432 -e POSTGRES_USER=airoute -e POSTGRES_PASSWORD=airoute \
#     -e POSTGRES_DB=airoute postgres:16-alpine
#   docker run -d -p 6379:6379 redis:7-alpine
test-integration:
	@test -n "$(AIROUTE_TEST_DATABASE_URL)" || { echo "AIROUTE_TEST_DATABASE_URL is required"; exit 1; }
	@test -n "$(AIROUTE_TEST_REDIS_URL)" || { echo "AIROUTE_TEST_REDIS_URL is required"; exit 1; }
	@echo "==> Running integration tests (PostgreSQL + Redis)..."
	go test -race -tags=integration ./internal/storage/ ./internal/distributed/

test-e2e:
	@echo "==> Running End-to-End (E2E) integration tests..."
	@bash scripts/test-e2e.sh

test-e2e-go:
	@echo "==> Running Go E2E test suite..."
	go test -v -tags=e2e ./tests/e2e/...

tidy:
	@echo "==> Tidying dependencies..."
	go mod tidy

clean:
	@rm -rf $(BIN_DIR)
	@echo "==> Cleaned."
