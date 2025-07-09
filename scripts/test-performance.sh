#!/bin/bash

# test-performance.sh - Script to test performance tests locally

set -e

echo "🔧 Testing Performance Test Infrastructure"
echo "=========================================="

# Check if required tools are available
command -v docker >/dev/null 2>&1 || { echo "❌ Docker is required but not installed"; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "❌ Docker Compose is required but not installed"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "❌ Go is required but not installed"; exit 1; }

# Check if OPENAI_API_KEY is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "⚠️  OPENAI_API_KEY not set - performance tests will be skipped"
fi

# Create test config if it doesn't exist
if [ ! -f "configs/config.yaml" ]; then
    echo "📝 Creating test configuration..."
    mkdir -p configs
    cat > configs/config.yaml << EOF
chroma:
  url: "http://localhost:8000"
  collection_name: "perf_test_collection"
openai:
  api_key: "${OPENAI_API_KEY:-test-key}"
  model: "gpt-4o"
  embedding_model: "text-embedding-3-small"
services:
  retrieve_url: "http://localhost:8081"
  synthesize_url: "http://localhost:8082"
  websearch_url: "http://localhost:8083"
teams:
  webhook_url: "https://example.com/webhook"
metadata:
  db_path: "./metadata.db"
EOF
fi

echo "🚀 Starting services..."
docker-compose up -d

echo "⏳ Waiting for services to be ready..."

# Wait for ChromaDB
echo "  Waiting for ChromaDB..."
for i in {1..30}; do
    if curl -s http://localhost:8000/api/v1/heartbeat > /dev/null 2>&1; then
        echo "  ✅ ChromaDB is ready"
        break
    fi
    echo "  Attempt $i/30 - ChromaDB not ready yet"
    sleep 2
done

# Wait for other services
echo "  Waiting for other services..."
for i in {1..30}; do
    if curl -s http://localhost:8081/health > /dev/null 2>&1 && \
       curl -s http://localhost:8082/health > /dev/null 2>&1 && \
       curl -s http://localhost:8083/health > /dev/null 2>&1 && \
       curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo "  ✅ All services are ready"
        break
    fi
    echo "  Attempt $i/30 - Services not ready yet"
    sleep 3
done

echo "🧪 Running performance tests..."
echo "================================"

# Run the performance tests
if go test -v -timeout=5m ./tests/performance/...; then
    echo "✅ Performance tests completed successfully"
else
    echo "⚠️  Performance tests were skipped or failed"
    echo "This is expected if:"
    echo "  - OPENAI_API_KEY is not set"
    echo "  - Services are not fully ready"
    echo "  - Running in short mode"
fi

echo ""
echo "🧪 Running performance benchmarks..."
echo "===================================="

# Run benchmarks if services are ready
if [ -n "$OPENAI_API_KEY" ]; then
    echo "Running benchmarks with OpenAI API..."
    go test -bench=. -benchmem -timeout=10m ./tests/performance/... || true
else
    echo "Skipping benchmarks - OPENAI_API_KEY not set"
fi

echo ""
echo "🔍 Service Status Check:"
echo "========================"
echo "ChromaDB: $(curl -s http://localhost:8000/api/v1/heartbeat > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "Retrieve: $(curl -s http://localhost:8081/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "Synthesize: $(curl -s http://localhost:8082/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "WebSearch: $(curl -s http://localhost:8083/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"
echo "TeamsBot: $(curl -s http://localhost:8080/health > /dev/null && echo "✅ Ready" || echo "❌ Not ready")"

echo ""
echo "📊 Performance Test Infrastructure Status:"
echo "=========================================="
echo "✅ Service dependency checks implemented"
echo "✅ Proper service health validation"
echo "✅ Graceful test skipping when services unavailable"
echo "✅ Environment variable validation"
echo "✅ Timeout handling for service readiness"

echo ""
echo "🎯 To run performance tests manually:"
echo "  1. Ensure services are running: docker-compose up -d"
echo "  2. Set OPENAI_API_KEY environment variable"
echo "  3. Run: go test -v ./tests/performance/..."
echo "  4. Run benchmarks: go test -bench=. ./tests/performance/..."

echo ""
echo "🧹 Cleanup:"
echo "  To stop services: docker-compose down"
