.PHONY: build test test-integration lint fmt docker-build docker-run help

BINARY_NAME=synccal
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -s -w"

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/synccal

test:
	go test -v -race -coverprofile=coverage.out ./internal/...

test-integration:
	INTEGRATION_TESTS=1 go test -v -timeout 10m ./tests/integration/...

test-all: test test-integration

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	goimports -w .

docker-build:
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

docker-run:
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for services to be healthy..."
	@sleep 30
	@echo "Services ready. Source: http://localhost:8081, Dest: http://localhost:8082, ICS: http://localhost:8083"

docker-stop:
	docker-compose -f docker-compose.test.yml down -v

docker-logs:
	docker-compose -f docker-compose.test.yml logs -f

run: build
	./bin/$(BINARY_NAME) -config config.yaml

dev: docker-run
	@sleep 5
	go run ./cmd/synccal -config config.yaml

clean:
	rm -rf bin/ coverage.out

help:
	@echo "Available targets:"
	@echo "  build           - Build the binary"
	@echo "  test            - Run unit tests"
	@echo "  test-integration - Run integration tests (requires Docker)"
	@echo "  test-all        - Run all tests"
	@echo "  lint            - Run linter"
	@echo "  fmt             - Format code"
	@echo "  docker-build    - Build Docker image"
	@echo "  docker-run      - Start test infrastructure"
	@echo "  docker-stop     - Stop test infrastructure"
	@echo "  docker-logs     - View test infrastructure logs"
	@echo "  run             - Build and run with config.yaml"
	@echo "  dev             - Start infra and run in dev mode"
	@echo "  clean           - Clean build artifacts"