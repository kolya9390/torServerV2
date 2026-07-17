.PHONY: build build-server build-cli build-all test test-build test-release lint lint-fix e2e e2e-race perf-config run-debug perf-capture docker docker-build docker-push clean clean-binaries help generate-mocks swagger

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

# Linter
GOLANGCI_LINT=golangci-lint

# Binary names
BINARY_NAME=torrserver
CLI_BINARY_NAME=torrctl
BINARY_DIR=dist
BINARY_DIR_ABS=$(abspath $(BINARY_DIR))
SERVER_PACKAGE=./cmd/torrserver
CLI_PACKAGE=./cmd/torrctl

# Docker parameters
DOCKER_TAG=latest

# Performance profiling parameters
PERF_VARIANT?=low-cpu-candidate
PERF_CONFIG_OUT_DIR?=artifacts/perf-configs

# Build flags
export VERSION COMMIT BUILD_TIME DIRTY
METADATA_SCRIPT=./.github/scripts/build-metadata.sh
METADATA_LDFLAGS:=$(shell $(METADATA_SCRIPT) ldflags)
METADATA_VERSION:=$(shell $(METADATA_SCRIPT) env | sed -n 's/^version=//p')
METADATA_COMMIT:=$(shell $(METADATA_SCRIPT) env | sed -n 's/^commit=//p')
METADATA_BUILD_TIME:=$(shell $(METADATA_SCRIPT) env | sed -n 's/^build_time=//p')
METADATA_DIRTY:=$(shell $(METADATA_SCRIPT) env | sed -n 's/^dirty=//p')
LDFLAGS=-ldflags '-w -s $(METADATA_LDFLAGS)'
DOCKER_BUILD_METADATA=--build-arg VERSION=$(METADATA_VERSION) --build-arg COMMIT=$(METADATA_COMMIT) --build-arg BUILD_TIME=$(METADATA_BUILD_TIME) --build-arg DIRTY=$(METADATA_DIRTY)

all: build

## build: Build the daemon and management CLI binaries
build: build-server build-cli

## build-server: Build the torrserver daemon binary
build-server:
	@mkdir -p "$(BINARY_DIR_ABS)"
	@rm -f "$(BINARY_DIR_ABS)/$(BINARY_NAME)"
	cd server && $(GOBUILD) $(LDFLAGS) -o "$(BINARY_DIR_ABS)/$(BINARY_NAME)" $(SERVER_PACKAGE)

## build-cli: Build the torrctl management CLI binary
build-cli:
	@mkdir -p "$(BINARY_DIR_ABS)"
	@rm -f "$(BINARY_DIR_ABS)/$(CLI_BINARY_NAME)" "$(BINARY_DIR_ABS)/tsctl"
	cd server && $(GOBUILD) $(LDFLAGS) -o "$(BINARY_DIR_ABS)/$(CLI_BINARY_NAME)" $(CLI_PACKAGE)

## build-all: Build binaries for all platforms
build-all:
	@echo "Building for all platforms..."
	@bash ./build-all.sh

## swagger: Generate Swagger API documentation
swagger:
	@echo "Generating Swagger docs..."
	cd server && swag init -g cmd/main.go --parseDependency
	@echo "Done! Open http://localhost:8090/swagger/index.html"

## test-release: Validate the lean SemVer and release-asset helpers
test-release:
	@echo "Testing release helpers..."
	@./.github/scripts/validate-release-tag_test.sh
	@./.github/scripts/build-metadata_test.sh
	@./.github/scripts/build-release-binary_test.sh
	@./.github/scripts/prepare-release-assets_test.sh
	@./.github/scripts/verify-release-binaries_test.sh
	@./.github/scripts/docker-release_test.sh
	@./.github/scripts/release-workflow_test.sh
	@./.github/scripts/extract-release-notes_test.sh

## test-build: Validate native and cross-platform build helpers
test-build:
	@echo "Testing build helpers..."
	@./.github/scripts/local-build_test.sh
	@./.github/scripts/build-all_test.sh

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

## e2e: Run the isolated torrserver/torrctl process E2E test
e2e:
	@echo "Running isolated split-process E2E test..."
	cd server && $(GOTEST) -tags=e2e -run '^TestSplitProcessWorkflow$$' -count=1 ./e2e

## e2e-race: Run the isolated process E2E test under the race detector
e2e-race:
	@echo "Running isolated split-process E2E test with race detection..."
	cd server && $(GOTEST) -race -tags=e2e -run '^TestSplitProcessWorkflow$$' -count=1 ./e2e

## perf-config: Generate a local debug profiling config (PERF_VARIANT=low-cpu-candidate)
perf-config:
	@echo "Generating local profiling config: $(PERF_VARIANT)"
	@PERF_CONFIG_OUT_DIR="$(PERF_CONFIG_OUT_DIR)" ./server/scripts/perf_config_variant.sh --variant "$(PERF_VARIANT)"

## run-debug: Build and run TorrServer with a generated debug profiling config
run-debug: build-server
	@config_path=$$(PERF_CONFIG_OUT_DIR="$(PERF_CONFIG_OUT_DIR)" ./server/scripts/perf_config_variant.sh --variant "$(PERF_VARIANT)"); \
	echo "Running TorrServer with debug profiling config: $$config_path"; \
	echo "Debug endpoints will be available only for this local run."; \
	TS_CONFIG="$$config_path" "$(BINARY_DIR_ABS)/$(BINARY_NAME)"

## perf-capture: Capture profiles from a running TorrServer (PERF_LABEL=one-stream PERF_DURATION=30)
perf-capture:
	@echo "Capturing TorrServer performance profiles..."
	./server/scripts/perf_capture.sh

## docker-build: Build Docker image for current platform
docker-build:
	@echo "Building Docker image..."
	docker build $(DOCKER_BUILD_METADATA) -t $(BINARY_NAME):$(DOCKER_TAG) .

## docker-build-multiarch: Build Docker image for multiple architectures
docker-build-multiarch:
	@echo "Building multi-arch Docker image..."
	docker buildx build $(DOCKER_BUILD_METADATA) --platform linux/amd64,linux/arm64 -t $(BINARY_NAME):$(DOCKER_TAG) .

## docker-push: Build and push multi-arch Docker image
docker-push:
	@echo "Building and pushing multi-arch Docker image..."
	docker buildx build $(DOCKER_BUILD_METADATA) --platform linux/amd64,linux/arm64 -t $(BINARY_NAME):$(DOCKER_TAG) --push .

## docker-run: Run Docker container locally
docker-run:
	@echo "Running Docker container..."
	docker run --rm -d --name $(BINARY_NAME) -p 8090:8090 $(BINARY_NAME):$(DOCKER_TAG)

## docker-stop: Stop running Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BINARY_NAME) 2>/dev/null || true

## clean-binaries: Remove local binary artifacts without touching the shared Go cache
clean-binaries:
	@echo "Cleaning..."
	rm -rf "$(BINARY_DIR_ABS)"

## clean: Clean project-generated artifacts
clean: clean-binaries
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
