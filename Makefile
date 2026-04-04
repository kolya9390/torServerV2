.PHONY: build build-all build-prometheus test lint lint-strict lint-fix e2e docker docker-build docker-push clean help web web-dev

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOCLEAN=$(GOCMD) clean

# Linter
GOLANGCI_LINT=golangci-lint
LINT_CONFIG=server/.golangci-strict.yml

# Binary name
BINARY_NAME=torrserver
BINARY_DIR=dist

# Docker parameters
DOCKER_TAG=latest

# Build flags
LDFLAGS=-ldflags '-w -s'

all: build

## build: Build the server binary (default, no Prometheus)
build:
	@echo "Building $(BINARY_NAME) (without Prometheus)..."
	@mkdir -p $(BINARY_DIR)
	cd server && $(GOBUILD) $(LDFLAGS) -o ../$(BINARY_DIR)/$(BINARY_NAME) ./cmd

## build-all: Build binaries for all platforms
build-all:
	@echo "Building for all platforms..."
	@bash ./build-all.sh

## web: Build web assets
web:
	@echo "Building web assets..."
	@go run gen_web.go

## web-dev: Build web assets (clean first)
web-clean:
	@echo "Building web assets (clean)..."
	@go run gen_web.go --clean

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

## lint: Run linter with strict config
lint:
	@echo "Running linter (strict mode)..."
	cd server && $(GOLANGCI_LINT) run -c $(LINT_CONFIG) ./...

## lint-strict: Run linter with strict config and fix auto-fixable issues
lint-fix:
	@echo "Running linter (strict mode, auto-fix)..."
	cd server && $(GOLANGCI_LINT) run -c $(LINT_CONFIG) --fix ./...

## lint-legacy: Run legacy linter script (deprecated)
lint-legacy:
	@echo "Running legacy linter script..."
	cd server && ./scripts/lint.sh

## e2e: Run E2E smoke tests
e2e:
	@echo "Running E2E smoke tests..."
	cd server && ./scripts/e2e_smoke.sh

## docker-build: Build Docker image for current platform
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## docker-build-multiarch: Build Docker image for multiple architectures
docker-build-multiarch:
	@echo "Building multi-arch Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## docker-push: Build and push multi-arch Docker image
docker-push:
	@echo "Building and pushing multi-arch Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .

## docker-run: Run Docker container locally
docker-run:
	@echo "Running Docker container..."
	docker run --rm -d --name torrserver -p 8090:8090 $(DOCKER_IMAGE):$(DOCKER_TAG)

## docker-stop: Stop running Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop torrserver 2>/dev/null || true

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BINARY_DIR)/
	cd server && $(GOCLEAN) -cache
	rm -f coverage.out coverage.html
	rm -rf server/artifacts/e2e-smoke/

## help: Show this help message
help:
	@echo "TorrServer Build System"
	@echo "======================="
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@cat Makefile | grep '^[a-z].*:' | grep -v '^\.' | sed 's/:.*//' | xargs -I {} sh -c 'grep "^## {}: " Makefile | sed "s/^## {}: /  {}  /"'
