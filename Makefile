# Makefile for AI SA Assistant

# Go settings
GO_VERSION := 1.23.5
GO_FILES := $(shell find . -name '*.go' | grep -v vendor)
GOLANGCI_LINT_VERSION := v1.61.0

# Docker settings
DOCKER_COMPOSE_FILE := docker-compose.yml
DOCKER_COMPOSE_TEST_FILE := docker-compose.test.yml

# Test settings
TEST_TIMEOUT := 10m
INTEGRATION_TEST_TIMEOUT := 30m

.PHONY: help
help: ## Show this help message
	@echo "AI SA Assistant - Available targets:"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Format Go code
	@echo "🔧 Formatting Go code..."
	@go fmt ./...

.PHONY: lint
lint: ## Run linter
	@echo "🔍 Running linter..."
	@golangci-lint run

.PHONY: test
test: ## Run unit tests
	@echo "🧪 Running unit tests..."
	@go test -v -timeout $(TEST_TIMEOUT) ./...

.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage
	@echo "🧪 Running unit tests with coverage..."
	@go test -v -timeout $(TEST_TIMEOUT) -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-integration
test-integration: ## Run integration tests (requires infrastructure)
	@echo "🧪 Running integration tests..."
	@go test -v -tags=integration -timeout $(INTEGRATION_TEST_TIMEOUT) ./tests/integration/...

.PHONY: test-integration-chromadb-only
test-integration-chromadb-only: ## Run integration tests with ChromaDB only
	@echo "🧪 Running ChromaDB-only integration tests..."
	@CHROMADB_ONLY_TESTS=true go test -v -tags=integration -timeout $(INTEGRATION_TEST_TIMEOUT) ./tests/integration/...

.PHONY: test-integration-with-infra
test-integration-with-infra: start-test-infra test-integration-chromadb-only ## Start test infrastructure and run integration tests
	@echo "✅ Integration tests completed with infrastructure"

.PHONY: start-test-infra
start-test-infra: ## Start test infrastructure (ChromaDB)
	@echo "🚀 Starting test infrastructure..."
	@docker-compose -f $(DOCKER_COMPOSE_TEST_FILE) up -d chromadb-test
	@echo "⏳ Waiting for ChromaDB test instance to be ready..."
	@for i in {1..30}; do \
		if curl -s http://localhost:8001/api/v1/heartbeat > /dev/null 2>&1; then \
			echo "✅ ChromaDB test instance ready on port 8001"; \
			break; \
		fi; \
		if [ $$i -eq 30 ]; then \
			echo "❌ ChromaDB test instance failed to start"; \
			exit 1; \
		fi; \
		echo "⏳ Attempt $$i/30 - waiting for ChromaDB..."; \
		sleep 2; \
	done

.PHONY: stop-test-infra
stop-test-infra: ## Stop test infrastructure
	@echo "🛑 Stopping test infrastructure..."
	@docker-compose -f $(DOCKER_COMPOSE_TEST_FILE) down -v

.PHONY: start-services
start-services: ## Start all services
	@echo "🚀 Starting all services..."
	@docker-compose -f $(DOCKER_COMPOSE_FILE) up -d

.PHONY: stop-services
stop-services: ## Stop all services
	@echo "🛑 Stopping all services..."
	@docker-compose -f $(DOCKER_COMPOSE_FILE) down

.PHONY: restart-services
restart-services: stop-services start-services ## Restart all services

.PHONY: logs
logs: ## Show logs for all services
	@docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f

.PHONY: logs-test
logs-test: ## Show logs for test services
	@docker-compose -f $(DOCKER_COMPOSE_TEST_FILE) logs -f

.PHONY: clean
clean: ## Clean up containers and volumes
	@echo "🧹 Cleaning up containers and volumes..."
	@docker-compose -f $(DOCKER_COMPOSE_FILE) down -v --remove-orphans
	@docker-compose -f $(DOCKER_COMPOSE_TEST_FILE) down -v --remove-orphans
	@docker system prune -f

.PHONY: build
build: ## Build all services
	@echo "🔨 Building all services..."
	@docker-compose -f $(DOCKER_COMPOSE_FILE) build

.PHONY: build-test
build-test: ## Build test services
	@echo "🔨 Building test services..."
	@docker-compose -f $(DOCKER_COMPOSE_TEST_FILE) build

.PHONY: check
check: fmt lint test ## Run all checks (format, lint, test)

.PHONY: check-all
check-all: fmt lint test test-integration-with-infra ## Run all checks including integration tests

.PHONY: pre-commit
pre-commit: check ## Run pre-commit checks

.PHONY: dev-setup
dev-setup: ## Set up development environment
	@echo "🔧 Setting up development environment..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "✅ Development environment setup complete"

.PHONY: seed-test-data
seed-test-data: start-test-infra ## Seed test data into ChromaDB test instance
	@echo "🌱 Seeding test data into ChromaDB..."
	@go run ./scripts/seed-test-data.go

.PHONY: status
status: ## Show status of all services
	@echo "📊 Service Status:"
	@echo "=================="
	@echo "ChromaDB (prod): $$(curl -s http://localhost:8000/api/v1/heartbeat > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "ChromaDB (test): $$(curl -s http://localhost:8001/api/v1/heartbeat > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "Retrieve: $$(curl -s http://localhost:8081/health > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "Synthesize: $$(curl -s http://localhost:8082/health > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "WebSearch: $$(curl -s http://localhost:8083/health > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "TeamsBot: $$(curl -s http://localhost:8080/health > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"
	@echo "WebUI: $$(curl -s http://localhost:8084/health > /dev/null 2>&1 && echo "✅ Ready" || echo "❌ Not ready")"

.PHONY: demo
demo: clean build start-services ## Clean, build, and start full demo environment
	@echo "🎯 Demo environment ready!"
	@echo "📱 WebUI: http://localhost:8084"
	@echo "🤖 Teams Bot: http://localhost:8080"
	@echo "🔍 Retrieve API: http://localhost:8081"
	@echo "🧠 Synthesize API: http://localhost:8082"
	@echo "🌐 WebSearch API: http://localhost:8083"
	@echo "📊 ChromaDB: http://localhost:8000"