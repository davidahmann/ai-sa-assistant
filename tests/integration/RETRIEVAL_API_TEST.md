# Retrieval API Integration Test

This document describes the comprehensive integration test for the Retrieval API service (`retrieval_api_test.go`).

## Overview

The retrieval API integration test validates the complete retrieval functionality including:

- Metadata filtering (SQLite)
- Vector search (ChromaDB)
- Fallback logic with insufficient results
- Response format validation
- Error handling
- Performance characteristics

## Test Structure

### Main Test Suite: `TestRetrievalAPIIntegration`

The main test runs all sub-tests in sequence:

1. **UnfilteredSearch** - Tests basic search without metadata filters
2. **FilteredSearch** - Tests search with metadata filters (scenario, cloud, etc.)
3. **FallbackLogic** - Tests fallback when filters yield insufficient results
4. **ResponseFormat** - Validates response structure and chunk format
5. **ErrorHandling** - Tests error scenarios (empty query, invalid JSON)
6. **Performance** - Tests response times and concurrent requests

### Test Environment Setup

The test uses:

- ChromaDB container on port 8001 (separate from main service)
- SQLite metadata database for filtering
- Test data from `testdata/` directory
- Mock OpenAI embeddings for vector search

## Prerequisites

### Required Services

Start the test environment:

```bash
# Start ChromaDB test container
docker-compose -f docker-compose.test.yml up -d

# Or use the setup script
./tests/integration/setup_test_env.sh
```

### Environment Variables

```bash
export OPENAI_API_KEY="your-openai-api-key" # pragma: allowlist secret
export TEST_SERVICES_BASE_URL="http://localhost"  # optional
export TEST_CHROMADB_URL="http://localhost:8001"  # optional
export TEST_TIMEOUT="60s"                         # optional
export TEST_VERBOSE="true"                        # optional
```

## Running the Tests

### Full Integration Test

```bash
go test -v -tags=integration ./tests/integration/retrieval_api_test.go -timeout=10m
```

### All Integration Tests

```bash
go test -v -tags=integration ./tests/integration/... -timeout=15m
```

### Specific Test Cases

```bash
# Run only unfiltered search test
go test -v -tags=integration ./tests/integration/retrieval_api_test.go -run TestRetrievalAPIIntegration/UnfilteredSearch

# Run only fallback logic test
go test -v -tags=integration ./tests/integration/retrieval_api_test.go -run TestRetrievalAPIIntegration/FallbackLogic

# Run only performance test
go test -v -tags=integration ./tests/integration/retrieval_api_test.go -run TestRetrievalAPIIntegration/Performance
```

## Test Data

### Documents

The test uses documents from `testdata/`:

- `test-aws-migration.md` - AWS migration scenarios
- `test-azure-hybrid.md` - Azure hybrid architecture
- `test-security-compliance.md` - Security compliance
- `test-fallback-scenario.md` - Fallback testing

### Metadata

Metadata is defined in `testdata/metadata.json` with:

- Document IDs and titles
- Platform (aws, azure, multi-cloud)
- Scenario (migration, hybrid, security-compliance)
- Type (playbook, compliance, test)
- Tags for filtering

## Validation Criteria

### Response Format

Each search response must include:

- `chunks` - Array of search results
- `count` - Number of results
- `query` - Original query string
- `fallback_triggered` - Boolean indicating fallback usage
- `fallback_reason` - Reason for fallback (if triggered)

### Chunk Format

Each chunk must include:

- `text` - Document content
- `score` - Similarity score (0.0-1.0)
- `doc_id` - Document identifier
- `source_id` - Source URL or title
- `metadata` - Document metadata

### Performance Requirements

- Individual search: < 5 seconds
- Concurrent searches: < 10 seconds for 5 concurrent requests
- Memory usage: Within reasonable limits
- No resource leaks

### Fallback Logic

Fallback is triggered when:

- Results count < configured threshold (default: 3)
- Average similarity score < configured threshold (default: 0.7)
- Metadata filters yield insufficient results

## Error Handling

The test validates:

- Empty query returns 400 error
- Invalid JSON returns 400 error
- Service unavailable scenarios
- Graceful degradation when dependencies fail

## Troubleshooting

### Common Issues

1. **ChromaDB Not Running**

   ```bash
   # Check if ChromaDB is accessible
   curl http://localhost:8001/api/v1/heartbeat
   ```

2. **Service Dependencies**

   ```bash
   # Check retrieve service health
   curl http://localhost:8081/health
   ```

3. **Test Data Issues**

   ```bash
   # Verify test data exists
   ls -la tests/integration/testdata/
   ```

### Debug Mode

```bash
# Enable verbose logging
export TEST_VERBOSE="true"

# Run with debug output
go test -v -tags=integration ./tests/integration/retrieval_api_test.go -args -test.v
```

## CI/CD Integration

The test is automatically included in GitHub Actions:

- Runs on PRs and main branch pushes
- Runs daily at 2 AM UTC
- Generates coverage reports
- Uploads test artifacts

## Test Coverage

The integration test provides coverage for:

- Complete retrieval API workflow
- Metadata filtering logic
- Vector search functionality
- Fallback mechanisms
- Error handling paths
- Performance characteristics

## Contributing

When modifying the test:

1. Follow existing test patterns
2. Use helper functions from `helpers.go`
3. Add appropriate cleanup
4. Update this documentation
5. Ensure tests pass in CI/CD

## Implementation Notes

This test implements all acceptance criteria from Issue #36:

- ✅ ChromaDB container with test data
- ✅ Unfiltered search validation
- ✅ Filtered search with metadata
- ✅ Fallback logic testing
- ✅ Response format validation
- ✅ Performance benchmarking
- ✅ Error handling
- ✅ CI/CD integration
