# Integration Test Infrastructure

This directory contains the integration test infrastructure for the AI SA Assistant project.

## Overview

The integration test infrastructure provides:
- ChromaDB test instance for vector database testing
- Service health checks and availability detection
- Test data seeding and isolation
- Makefile targets for easy test execution

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.23.5
- Make (for using Makefile targets)

### Running Integration Tests

#### 1. Start Test Infrastructure

```bash
# Start ChromaDB test instance
make start-test-infra

# Check service status
make status
```

#### 2. Run ChromaDB-Only Tests

```bash
# Run tests that only require ChromaDB
make test-integration-chromadb-only

# Or run with infrastructure startup
make test-integration-with-infra
```

#### 3. Run Full Integration Tests

```bash
# Start all services
make start-services

# Run full integration test suite
make test-integration
```

## Test Infrastructure Components

### ChromaDB Test Instance

- **Port**: 8001 (to avoid conflicts with production ChromaDB on 8000)
- **Docker Service**: `chromadb-test` 
- **Health Check**: Available at `http://localhost:8001/api/v1/heartbeat`
- **Data Persistence**: Isolated test volume

### Test Environment Variables

- `CHROMADB_ONLY_TESTS=true` - Enable ChromaDB-only test mode
- `OPENAI_API_KEY` - Required for some tests (can be mocked)

### Test Data

- **Test Collection**: `test_demo_collection`
- **Sample Documents**: AWS, Azure, and security-related content
- **Test Isolation**: Each test creates unique collections with timestamps

## Available Make Targets

```bash
# Test Infrastructure
make start-test-infra        # Start ChromaDB test instance
make stop-test-infra         # Stop test infrastructure  
make status                  # Check service status

# Testing
make test-integration-chromadb-only    # ChromaDB-only tests
make test-integration-with-infra       # Start infra + run tests
make test-integration                  # Full integration tests
make test                              # Unit tests only

# Services
make start-services          # Start all services
make stop-services           # Stop all services
make clean                   # Clean up containers and volumes
```

## Test Modes

### ChromaDB-Only Mode

When `CHROMADB_ONLY_TESTS=true` is set:
- Only ChromaDB test instance is required
- Other service tests are skipped
- Focuses on vector database functionality

### Full Integration Mode

When all services are available:
- Tests complete end-to-end workflows
- Service interaction validation
- Performance and resilience testing

## Troubleshooting

### ChromaDB Not Starting

```bash
# Check container logs
docker-compose -f docker-compose.test.yml logs chromadb-test

# Common issues:
# - Port 8001 already in use
# - Docker daemon not running
# - Network connectivity issues
```

### Tests Being Skipped

```bash
# Check service availability
make status

# Enable ChromaDB-only mode
export CHROMADB_ONLY_TESTS=true
```

## Configuration

### Environment Variables

```bash
# Required for full testing
export OPENAI_API_KEY=your_api_key_here

# Optional - enable ChromaDB-only mode
export CHROMADB_ONLY_TESTS=true

# Optional - increase test timeouts
export TEST_TIMEOUT=10m
export INTEGRATION_TEST_TIMEOUT=30m
```

## Test Results

Based on the current implementation, the following test infrastructure is working:

### ✅ Working Components

- **ChromaDB Test Instance**: Successfully starts on port 8001
- **Health Checks**: ChromaDB heartbeat endpoint working
- **Collection Management**: Create, get, and delete collections
- **Collection Metadata**: Support for custom metadata
- **Test Isolation**: Unique collection names per test
- **Service Detection**: Automatic service availability checks
- **Makefile Integration**: Easy commands for test execution

### ⚠️ Known Issues

- **Document Addition**: ChromaDB API calls for adding documents may need adjustment
- **Test Data Seeding**: Some document operations timeout
- **API Compatibility**: ChromaDB 0.5.0 API may have changed

### 🔄 Partial Implementation

- **Basic Tests**: Health checks and collection management work
- **Advanced Tests**: Document search and filtering need refinement
- **Performance Tests**: Infrastructure ready, specific tests need tuning

## Usage Examples

### Basic Health Check

```bash
make start-test-infra
curl http://localhost:8001/api/v1/heartbeat
```

### Run ChromaDB Tests

```bash
make test-integration-with-infra
```

### Check Service Status

```bash
make status
```

### Cleanup

```bash
make stop-test-infra
```