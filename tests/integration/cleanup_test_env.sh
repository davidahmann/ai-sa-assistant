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

# cleanup_test_env.sh - Cleans up the test environment for retrieval API integration tests

set -e

echo "Cleaning up test environment for retrieval API integration tests..."

# Stop and remove containers
echo "Stopping test containers..."
docker-compose -f docker-compose.test.yml down -v

# Remove test volumes
echo "Removing test volumes..."
docker volume rm $(docker volume ls -q | grep -E "(chromadb-test-data|test-metadata)" || true) 2>/dev/null || true

# Remove test configuration
echo "Removing test configuration..."
rm -rf "$(dirname "$0")/config"

# Remove test database files
echo "Removing test database files..."
rm -f test_metadata.db
rm -f test_metadata.db-journal

# Remove any temporary test files
echo "Removing temporary test files..."
find "$(dirname "$0")" -name "*.tmp" -delete 2>/dev/null || true
find "$(dirname "$0")" -name "*.log" -delete 2>/dev/null || true

echo "Test environment cleanup complete!"
