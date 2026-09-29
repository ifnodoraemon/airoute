export PATH := /usr/local/go/bin:$(PATH)

.PHONY: all build run test tidy clean

APP_NAME = airoute
BIN_DIR = bin
ENTRY = cmd/airoute/main.go
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

tidy:
	@echo "==> Tidying dependencies..."
	go mod tidy

clean:
	@rm -rf $(BIN_DIR)
	@echo "==> Cleaned."
