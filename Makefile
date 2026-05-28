.PHONY: build build-all test lint lint-fix e2e perf-config run-debug perf-capture docker docker-build docker-push clean help generate-mocks swagger

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean

# Linter
GOLANGCI_LINT=golangci-lint

# Binary name
BINARY_NAME=torrserver
BINARY_DIR=dist

# Docker parameters
DOCKER_TAG=latest

# Performance profiling parameters
PERF_VARIANT?=low-cpu-candidate
PERF_CONFIG_OUT_DIR?=artifacts/perf-configs

# Build flags
LDFLAGS=-ldflags '-w -s'

all: build

## build: Build the server binary (includes CLI)
build:
	@mkdir -p $(BINARY_DIR)
	cd server && $(GOBUILD) $(LDFLAGS) -o ../$(BINARY_DIR)/$(BINARY_NAME) ./cmd

## build-all: Build binaries for all platforms
build-all:
	@echo "Building for all platforms..."
	@bash ./build-all.sh

## swagger: Generate Swagger API documentation
swagger:
	@echo "Generating Swagger docs..."
	cd server && swag init -g cmd/main.go --parseDependency
	@echo "Done! Open http://localhost:8090/swagger/index.html"

## test: Run tests
test:
	@echo "Running tests..."
	cd server && $(GOTEST) -race -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	cd server && $(GOTEST) -race -coverprofile=coverage.out ./...
	cd server && $(GOCMD) tool cover -html=coverage.out -o ../coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linter
lint:
	@echo "Running linter..."
	cd server && $(GOLANGCI_LINT) run ./...

## lint-fix: Run linter with auto-fix
lint-fix:
	@echo "Running linter (auto-fix)..."
	cd server && $(GOLANGCI_LINT) run --fix ./...

## e2e: Run E2E smoke tests
e2e:
	@echo "Running E2E smoke tests..."
	cd server && ./scripts/e2e_smoke.sh

## perf-config: Generate a local debug profiling config (PERF_VARIANT=low-cpu-candidate)
perf-config:
	@echo "Generating local profiling config: $(PERF_VARIANT)"
	@PERF_CONFIG_OUT_DIR="$(PERF_CONFIG_OUT_DIR)" ./server/scripts/perf_config_variant.sh --variant "$(PERF_VARIANT)"

## run-debug: Build and run TorrServer with a generated debug profiling config
run-debug: build
	@config_path=$$(PERF_CONFIG_OUT_DIR="$(PERF_CONFIG_OUT_DIR)" ./server/scripts/perf_config_variant.sh --variant "$(PERF_VARIANT)"); \
	echo "Running TorrServer with debug profiling config: $$config_path"; \
	echo "Debug endpoints will be available only for this local run."; \
	TS_CONFIG="$$config_path" ./$(BINARY_DIR)/$(BINARY_NAME)

## perf-capture: Capture profiles from a running TorrServer (PERF_LABEL=one-stream PERF_DURATION=30)
perf-capture:
	@echo "Capturing TorrServer performance profiles..."
	./server/scripts/perf_capture.sh

## docker-build: Build Docker image for current platform
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(DOCKER_TAG) .

## docker-build-multiarch: Build Docker image for multiple architectures
docker-build-multiarch:
	@echo "Building multi-arch Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(BINARY_NAME):$(DOCKER_TAG) .

## docker-push: Build and push multi-arch Docker image
docker-push:
	@echo "Building and pushing multi-arch Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(BINARY_NAME):$(DOCKER_TAG) --push .

## docker-run: Run Docker container locally
docker-run:
	@echo "Running Docker container..."
	docker run --rm -d --name $(BINARY_NAME) -p 8090:8090 $(BINARY_NAME):$(DOCKER_TAG)

## docker-stop: Stop running Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BINARY_NAME) 2>/dev/null || true

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BINARY_DIR)/
	cd server && $(GOCLEAN) -cache
	rm -f coverage.out coverage.html
	rm -rf server/artifacts/e2e-smoke/

## generate-mocks: Generate mock implementations using mockgen
generate-mocks:
	@echo "Generating mocks with mockgen..."
	@mkdir -p server/internal/mocks
	@cd server && $(HOME)/go/bin/mockgen -source=torr/service.go -destination=internal/mocks/mock_torrent_service.go -package=mocks TorrentService
	@cd server && $(HOME)/go/bin/mockgen -source=settings/provider.go -destination=internal/mocks/mock_settings_provider.go -package=mocks SettingsProvider
	@cd server && $(HOME)/go/bin/mockgen -source=internal/app/contracts/contracts.go -destination=internal/mocks/mock_api_contracts.go -package=mocks -mock_names TorrentHandle=MockAPITorrentHandle,TorrentService=MockAPITorrentService,SettingsService=MockAPISettingsService,ViewedService=MockAPIViewedService,SystemService=MockAPISystemService,SearchService=MockAPISearchService,MediaService=MockAPIMediaService,ModulesService=MockAPIModulesService,StreamService=MockAPIStreamService,PlaybackService=MockAPIPlaybackService TorrentHandle,TorrentService,SettingsService,ViewedService,SystemService,SearchService,MediaService,ModulesService,StreamService,PlaybackService
	@echo "Mocks generated successfully"

## help: Show this help message
help:
	@echo "TorrServer Build System"
	@echo "======================="
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@cat Makefile | grep '^[a-z].*:' | grep -v '^\.' | sed 's/:.*//' | xargs -I {} sh -c 'grep "^## {}: " Makefile | sed "s/^## {}: /  {}  /"'
