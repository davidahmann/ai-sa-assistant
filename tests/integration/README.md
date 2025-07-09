# End-to-End Integration Tests

This directory contains comprehensive end-to-end integration tests for the AI SA Assistant project. These tests validate all 4 demo scenarios work correctly from Teams message input to Adaptive Card response.

## Test Structure

### Files

- `demo_scenarios_test.go` - Main E2E test suite for all 4 demo scenarios
- `helpers.go` - Test utilities and helper functions
- `integration_test.go` - Basic service integration tests
- `README.md` - This documentation file

### Test Categories

1. **Demo Scenarios E2E Tests** - Tests all 4 demo scenarios:
   - AWS Lift-and-Shift Migration
   - Azure Hybrid Architecture
   - Azure Disaster Recovery
   - AWS Security Compliance

2. **Failure Handling Tests** - Tests error scenarios:
   - Invalid JSON requests
   - Empty queries
   - Very long queries
   - Non-SA queries

3. **Performance Tests** - Tests performance characteristics:
   - Response time validation (< 30 seconds)
   - Concurrent request handling
   - Load testing

4. **Pipeline Validation Tests** - Tests internal pipeline components:
   - Retrieval service metadata filtering
   - Web search service freshness detection
   - Synthesis service integration
   - Teams webhook response structure

## Prerequisites

### Required Services

All services must be running before executing tests:

```bash
# Start ChromaDB
docker run -p 8000:8000 chromadb/chroma:latest

# Start all microservices
docker-compose up
```

### Environment Variables

```bash
export OPENAI_API_KEY="your-openai-api-key" # pragma: allowlist secret
export TEAMS_WEBHOOK_URL="your-teams-webhook-url"
export TEST_SERVICES_BASE_URL="http://localhost"  # optional
export TEST_CHROMADB_URL="http://localhost:8000"  # optional
export TEST_TIMEOUT="60s"                        # optional
export TEST_VERBOSE="true"                        # optional
export TEST_SKIP_SLOW="false"                     # optional
```

### Configuration

Ensure `configs/config.yaml` is properly configured:

```yaml
openai:
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4"
  embedding_model: "text-embedding-3-small"

chromadb:
  url: "http://localhost:8000"
  collection_name: "ai_sa_assistant"

services:
  retrieve:
    port: 8081
  websearch:
    port: 8083
  synthesize:
    port: 8082
  teamsbot:
    port: 8080

teams:
  webhook_url: "${TEAMS_WEBHOOK_URL}"

logging:
  level: "info"
  format: "json"
```

## Running Tests

### Quick Test (Basic Integration)

```bash
# Run basic integration tests
go test -v -tags=integration ./tests/integration/integration_test.go
```

### Full Demo Scenarios Test

```bash
# Run all demo scenarios E2E tests
go test -v -tags=integration ./tests/integration/demo_scenarios_test.go -timeout=15m
```

### All Integration Tests

```bash
# Run all integration tests
go test -v -tags=integration ./tests/integration/... -timeout=20m
```

### With Coverage

```bash
# Run tests with coverage
go test -v -tags=integration -coverprofile=coverage.out ./tests/integration/...
go tool cover -html=coverage.out -o coverage.html
```

### Specific Test Cases

```bash
# Run specific demo scenario
go test -v -tags=integration ./tests/integration/demo_scenarios_test.go -run TestDemoScenariosE2E/AWS_Lift_and_Shift_Migration

# Run failure handling tests
go test -v -tags=integration ./tests/integration/demo_scenarios_test.go -run TestDemoScenariosFailureHandling

# Run performance tests
go test -v -tags=integration ./tests/integration/demo_scenarios_test.go -run TestDemoScenariosPerformance
```

## Test Validation

Each demo scenario test validates:

### 1. Performance Requirements

- Response time < 30 seconds
- Concurrent request handling
- Service health checks

### 2. Response Structure

- Teams Adaptive Card format
- Required fields present
- Valid JSON structure

### 3. Content Validation

- Expected keywords in response
- Mermaid diagram syntax
- Code snippet presence
- Source citations

### 4. Pipeline Execution

- Metadata filtering (SQLite)
- Vector search (ChromaDB)
- Web search triggered for freshness
- Synthesis service integration

## Test Data

Tests use synthetic test data:

- **Documents**: Mock playbooks and guides for each scenario
- **Metadata**: Synthetic metadata for filtering tests
- **Embeddings**: Simple mock embeddings for vector search
- **Responses**: Predefined expected response patterns

## Troubleshooting

### Common Issues

1. **Services Not Running**

   ```bash
   # Check service health
   curl http://localhost:8081/health
   curl http://localhost:8082/health
   curl http://localhost:8083/health
   curl http://localhost:8080/health
   ```

2. **ChromaDB Connection Failed**

   ```bash
   # Check ChromaDB
   curl http://localhost:8000/api/v1/heartbeat
   ```

3. **API Key Issues**

   ```bash
   # Verify environment variables
   echo $OPENAI_API_KEY
   echo $TEAMS_WEBHOOK_URL
   ```

4. **Test Timeouts**

   ```bash
   # Increase timeout
   export TEST_TIMEOUT="120s"
   ```

### Debug Mode

```bash
# Enable verbose logging
export TEST_VERBOSE="true"

# Skip slow tests
export TEST_SKIP_SLOW="true"

# Run with debug output
go test -v -tags=integration ./tests/integration/... -timeout=30m -args -test.v
```

## CI/CD Integration

Tests are integrated into GitHub Actions:

- **PR Tests**: Run on all pull requests
- **Daily Tests**: Run daily at 2 AM UTC
- **Coverage**: Generate and upload coverage reports
- **Artifacts**: Upload test results and logs

### GitHub Actions Configuration

See `.github/workflows/integration-tests.yml` for the complete CI/CD pipeline.

### Required Secrets

```bash
# GitHub repository secrets
OPENAI_API_KEY=your-openai-api-key
TEAMS_WEBHOOK_URL=your-teams-webhook-url
```

## Contributing

When adding new tests:

1. Follow existing test patterns
2. Use helper functions from `helpers.go`
3. Add appropriate test data cleanup
4. Update documentation
5. Ensure tests pass in CI/CD

### Test Naming Convention

- `TestDemoScenariosE2E` - Main E2E scenario tests
- `TestDemoScenariosFailureHandling` - Error handling tests
- `TestDemoScenariosPerformance` - Performance tests
- `Test<Component><Functionality>` - Component-specific tests

### Helper Functions

- `NewTestDataManager()` - Test data setup/teardown
- `NewServiceHealthChecker()` - Service health validation
- `NewTestResultValidator()` - Response validation
- `NewFileSystemHelper()` - File system operations

## Monitoring

Test results are monitored through:

- GitHub Actions workflow status
- Coverage reports via Codecov
- Test artifacts and logs
- Performance metrics over time

## Demo Preparation

For demo preparation:

1. Run all tests to ensure system stability
2. Verify response times are acceptable
3. Check that all expected content is generated
4. Validate Teams card rendering
5. Test failure scenarios for graceful handling

```bash
# Demo readiness check
go test -v -tags=integration ./tests/integration/demo_scenarios_test.go -run TestDemoScenariosE2E -timeout=15m
```
