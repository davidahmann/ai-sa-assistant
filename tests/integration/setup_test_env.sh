#!/bin/bash

# Copyright 2024 AI SA Assistant Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# setup_test_env.sh - Sets up the test environment for retrieval API integration tests

set -e

echo "Setting up test environment for retrieval API integration tests..."

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if required environment variables are set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY environment variable is not set"
    exit 1
fi

# Create test configuration directory if it doesn't exist
mkdir -p "$(dirname "$0")/config"

# Create test configuration file
cat > "$(dirname "$0")/config/test-config.yaml" << EOF
openai:
  apikey: "\${OPENAI_API_KEY}"
  endpoint: "https://api.openai.com/v1"

chroma:
  url: "http://localhost:8001"
  collection_name: "test_demo_collection"

metadata:
  db_path: "test_metadata.db"

retrieval:
  max_chunks: 5
  fallback_threshold: 3
  confidence_threshold: 0.7
  fallback_score_threshold: 0.6

logging:
  level: "debug"
  format: "console"
  output: "stdout"

services:
  retrieve_url: "http://localhost:8081"
  websearch_url: "http://localhost:8083"
  synthesize_url: "http://localhost:8082"
EOF

echo "Test configuration created at tests/integration/config/test-config.yaml"

# Start test environment
echo "Starting test environment with docker-compose..."
docker-compose -f docker-compose.test.yml up -d

# Wait for services to be healthy
echo "Waiting for services to be healthy..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if docker-compose -f docker-compose.test.yml ps | grep -q "healthy"; then
        echo "Services are healthy!"
        break
    fi

    echo "Attempt $((attempt + 1))/$max_attempts: Waiting for services..."
    sleep 5
    attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
    echo "Error: Services failed to become healthy within timeout"
    docker-compose -f docker-compose.test.yml logs
    exit 1
fi

# Verify ChromaDB is accessible
echo "Verifying ChromaDB connectivity..."
if curl -f http://localhost:8001/api/v1/heartbeat > /dev/null 2>&1; then
    echo "ChromaDB is accessible"
else
    echo "Error: ChromaDB is not accessible"
    exit 1
fi

# Verify retrieval service is accessible
echo "Verifying retrieval service connectivity..."
if curl -f http://localhost:8081/health > /dev/null 2>&1; then
    echo "Retrieval service is accessible"
else
    echo "Error: Retrieval service is not accessible"
    exit 1
fi

echo "Test environment setup complete!"
echo ""
echo "You can now run the integration tests:"
echo "go test -v -tags=integration ./tests/integration/retrieval_api_test.go -timeout=10m"
echo ""
echo "To clean up the test environment:"
echo "docker-compose -f docker-compose.test.yml down -v"
