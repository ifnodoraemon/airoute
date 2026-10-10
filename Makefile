export PATH := /usr/local/go/bin:$(PATH)

.PHONY: all build run test test-e2e test-e2e-go tidy clean

APP_NAME = airoute
BIN_DIR = bin
ENTRY = ./cmd/airoute
CONFIG = configs/config.yaml

all: build

build-web:
	@echo "==> Building React Web UI..."
	@cd web && npm run build

build: build-web
	@mkdir -p $(BIN_DIR)
	@echo "==> Building $(APP_NAME) single binary..."
	go build -o $(BIN_DIR)/$(APP_NAME) $(ENTRY)

run:
	@echo "==> Running $(APP_NAME)..."
	go run $(ENTRY) -config $(CONFIG)

test:
	@echo "==> Running tests..."
	go test -v -race ./...

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
