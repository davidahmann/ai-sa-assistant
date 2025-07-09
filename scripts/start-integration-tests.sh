#!/bin/bash

# start-integration-tests.sh - Script to start integration test environment

set -e

echo "🚀 Starting Integration Test Environment"
echo "======================================="

# Check if required tools are available
command -v docker >/dev/null 2>&1 || { echo "❌ Docker is required but not installed"; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "❌ Docker Compose is required but not installed"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "❌ Go is required but not installed"; exit 1; }

# Check if OPENAI_API_KEY is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "⚠️  OPENAI_API_KEY not set - some integration tests may be skipped"
fi

echo "🔧 Starting test infrastructure..."

# Stop any existing containers
echo "  Stopping existing containers..."
docker-compose -f docker-compose.test.yml down -v 2>/dev/null || true
docker-compose down 2>/dev/null || true

# Start test environment
echo "  Starting ChromaDB test container..."
docker-compose -f docker-compose.test.yml up -d chromadb-test

# Wait for ChromaDB to be ready
echo "  Waiting for ChromaDB to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8001/api/v1/heartbeat > /dev/null 2>&1; then
        echo "  ✅ ChromaDB test instance ready on port 8001"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "  ❌ ChromaDB did not start within timeout"
        exit 1
    fi
    echo "  Attempt $i/30 - ChromaDB not ready yet"
    sleep 2
done

# Check if we should also start the full stack
if [ "$1" = "--full-stack" ]; then
    echo "  Starting full service stack..."
    docker-compose up -d

    echo "  Waiting for services to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:8081/health > /dev/null 2>&1 && \
           curl -s http://localhost:8082/health > /dev/null 2>&1 && \
           curl -s http://localhost:8083/health > /dev/null 2>&1 && \
           curl -s http://localhost:8080/health > /dev/null 2>&1; then
            echo "  ✅ All services ready"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "  ⚠️  Some services may not be ready"
            break
        fi
        echo "  Attempt $i/30 - Services not ready yet"
        sleep 3
    done
fi

echo ""
echo "🧪 Running integration tests..."
echo "==============================="

# Run integration tests with proper tags
if go test -v -tags=integration ./tests/integration/...; then
    echo "✅ Integration tests completed successfully"
else
    echo "❌ Integration tests failed"
    exit 1
fi

echo ""
echo "🎯 Integration Test Environment Status:"
echo "======================================"
echo "ChromaDB (test): $(curl -s http://localhost:8001/api/v1/heartbeat > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "ChromaDB (prod): $(curl -s http://localhost:8000/api/v1/heartbeat > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "Retrieve: $(curl -s http://localhost:8081/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "Synthesize: $(curl -s http://localhost:8082/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "WebSearch: $(curl -s http://localhost:8083/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "TeamsBot: $(curl -s http://localhost:8080/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"

echo ""
echo "🔧 Integration Test Infrastructure:"
echo "=================================="
echo "✅ Service availability checks implemented"
echo "✅ Dynamic ChromaDB port detection (8000/8001)"
echo "✅ Graceful test skipping when services unavailable"
echo "✅ Test isolation with separate containers"
echo "✅ Proper cleanup and teardown"

echo ""
echo "🎯 To run integration tests manually:"
echo "  1. Start test environment: ./scripts/start-integration-tests.sh"
echo "  2. Run specific tests: go test -v -tags=integration ./tests/integration/..."
echo "  3. Run with full stack: ./scripts/start-integration-tests.sh --full-stack"

echo ""
echo "🧹 Cleanup:"
echo "  To stop test environment: docker-compose -f docker-compose.test.yml down -v"
echo "  To stop all services: docker-compose down"
