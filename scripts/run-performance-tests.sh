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

# Performance Testing Script for AI SA Assistant
# This script provides an easy way to run various performance tests locally

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
CONFIG_PATH="$PROJECT_ROOT/configs/config.yaml"
RESULTS_DIR="$PROJECT_ROOT/performance-results"

# Default values
TEST_TYPE="basic"
TIMEOUT="20m"
VERBOSE=false
CLEANUP=true

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Performance testing script for AI SA Assistant

OPTIONS:
    -t, --type TYPE         Type of performance test to run (default: basic)
                           Options: basic, ingestion, concurrent, memory, rate_limit, e2e, all
    -c, --config PATH      Path to configuration file (default: configs/config.yaml)
    -o, --output DIR       Output directory for results (default: performance-results)
    -T, --timeout DURATION Test timeout (default: 20m)
    -v, --verbose          Enable verbose output
    -k, --keep-services    Keep services running after tests (don't cleanup)
    -h, --help             Show this help message

EXAMPLES:
    # Run basic performance tests
    $0 --type basic

    # Run memory tests with verbose output
    $0 --type memory --verbose

    # Run all performance tests with custom timeout
    $0 --type all --timeout 30m

    # Run concurrent tests and keep services running
    $0 --type concurrent --keep-services

ENVIRONMENT VARIABLES:
    OPENAI_API_KEY         OpenAI API key (required for full tests)
    TEST_MODE              Set to 'true' to run in test mode
    CONFIG_PATH            Path to configuration file

PREREQUISITES:
    - Go 1.23.5+
    - Docker and Docker Compose
    - ChromaDB running on port 8000
    - Valid OpenAI API key for comprehensive tests
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--type)
            TEST_TYPE="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_PATH="$2"
            shift 2
            ;;
        -o|--output)
            RESULTS_DIR="$2"
            shift 2
            ;;
        -T|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -k|--keep-services)
            CLEANUP=false
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Validate test type
case $TEST_TYPE in
    basic|ingestion|concurrent|memory|rate_limit|e2e|all)
        ;;
    *)
        log_error "Invalid test type: $TEST_TYPE"
        log_error "Valid types: basic, ingestion, concurrent, memory, rate_limit, e2e, all"
        exit 1
        ;;
esac

# Create results directory
mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULT_FILE="$RESULTS_DIR/performance_${TEST_TYPE}_${TIMESTAMP}.txt"

log_info "Starting performance tests..."
log_info "Test type: $TEST_TYPE"
log_info "Config path: $CONFIG_PATH"
log_info "Results will be saved to: $RESULT_FILE"
log_info "Timeout: $TIMEOUT"

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check Go version
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        exit 1
    fi

    local go_version=$(go version | grep -o 'go[0-9.]*' | head -1)
    log_info "Go version: $go_version"

    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi

    # Check if ChromaDB is running
    if ! curl -sf http://localhost:8000/api/v1/heartbeat &> /dev/null; then
        log_warning "ChromaDB is not running on localhost:8000"
        log_info "Starting ChromaDB with Docker Compose..."
        cd "$PROJECT_ROOT"
        docker-compose up -d chromadb

        # Wait for ChromaDB
        log_info "Waiting for ChromaDB to be ready..."
        for i in {1..30}; do
            if curl -sf http://localhost:8000/api/v1/heartbeat &> /dev/null; then
                log_success "ChromaDB is ready"
                break
            fi
            echo -n "."
            sleep 2
        done
        echo

        if ! curl -sf http://localhost:8000/api/v1/heartbeat &> /dev/null; then
            log_error "ChromaDB failed to start"
            exit 1
        fi
    else
        log_success "ChromaDB is already running"
    fi

    # Check configuration file
    if [[ ! -f "$CONFIG_PATH" ]]; then
        log_error "Configuration file not found: $CONFIG_PATH"
        exit 1
    fi

    # Check OpenAI API key for comprehensive tests
    if [[ "$TEST_TYPE" != "basic" && -z "$OPENAI_API_KEY" ]]; then
        log_warning "OPENAI_API_KEY not set - some tests may be skipped"
    fi

    log_success "Prerequisites check completed"
}

# Start required services
start_services() {
    log_info "Starting required services..."

    cd "$PROJECT_ROOT"

    # Build services if needed
    if [[ ! -f "bin/retrieve" || ! -f "bin/synthesize" || ! -f "bin/websearch" || ! -f "bin/teamsbot" ]]; then
        log_info "Building services..."
        mkdir -p bin
        CGO_ENABLED=1 go build -o bin/retrieve ./cmd/retrieve
        CGO_ENABLED=0 go build -o bin/websearch ./cmd/websearch
        CGO_ENABLED=0 go build -o bin/synthesize ./cmd/synthesize
        CGO_ENABLED=0 go build -o bin/teamsbot ./cmd/teamsbot
        chmod +x bin/*
        log_success "Services built successfully"
    fi

    # Start services in background
    mkdir -p logs

    log_info "Starting retrieve service..."
    ./bin/retrieve --config="$CONFIG_PATH" > logs/retrieve.log 2>&1 &
    RETRIEVE_PID=$!

    log_info "Starting websearch service..."
    ./bin/websearch --config="$CONFIG_PATH" > logs/websearch.log 2>&1 &
    WEBSEARCH_PID=$!

    log_info "Starting synthesize service..."
    ./bin/synthesize --config="$CONFIG_PATH" > logs/synthesize.log 2>&1 &
    SYNTHESIZE_PID=$!

    log_info "Starting teamsbot service..."
    ./bin/teamsbot --config="$CONFIG_PATH" > logs/teamsbot.log 2>&1 &
    TEAMSBOT_PID=$!

    # Wait for services to be ready
    log_info "Waiting for services to be ready..."
    sleep 15

    # Check each service
    services_ready=true

    for service in "retrieve:8081" "synthesize:8082" "websearch:8083" "teamsbot:8080"; do
        name=$(echo $service | cut -d: -f1)
        port=$(echo $service | cut -d: -f2)

        if timeout 30 bash -c "until curl -sf http://localhost:$port/health; do sleep 1; done" 2>/dev/null; then
            log_success "$name service is ready"
        else
            log_error "$name service failed to start"
            log_error "Check logs/$(echo $name).log for details"
            services_ready=false
        fi
    done

    if [[ "$services_ready" != "true" ]]; then
        log_error "Some services failed to start"
        exit 1
    fi

    log_success "All services are ready"
}

# Stop services
stop_services() {
    if [[ "$CLEANUP" == "true" ]]; then
        log_info "Stopping services..."

        pkill -f "./bin/retrieve" 2>/dev/null || true
        pkill -f "./bin/websearch" 2>/dev/null || true
        pkill -f "./bin/synthesize" 2>/dev/null || true
        pkill -f "./bin/teamsbot" 2>/dev/null || true

        log_success "Services stopped"
    else
        log_info "Keeping services running (--keep-services flag set)"
        log_info "Service PIDs: retrieve=$RETRIEVE_PID, websearch=$WEBSEARCH_PID, synthesize=$SYNTHESIZE_PID, teamsbot=$TEAMSBOT_PID"
    fi
}

# Cleanup on exit
cleanup() {
    if [[ "$CLEANUP" == "true" ]]; then
        stop_services
    fi
}
trap cleanup EXIT

# Run performance tests
run_tests() {
    log_info "Running $TEST_TYPE performance tests..."

    cd "$PROJECT_ROOT"

    local test_pattern=""
    local test_files=""

    case $TEST_TYPE in
        basic)
            test_pattern="Benchmark"
            test_files="./tests/performance/performance_test.go"
            ;;
        ingestion)
            test_pattern="TestLargeDocumentProcessing|TestBatchEmbeddingGeneration|TestGracefulLargeDocumentHandling"
            test_files="./tests/performance/ingestion_performance_test.go"
            ;;
        concurrent)
            test_pattern="TestConcurrentTeamsWebhookProcessing|TestConcurrentRetrievalRequests|TestDatabaseConnectionPooling"
            test_files="./tests/performance/concurrent_load_test.go"
            ;;
        memory)
            test_pattern="TestVectorSearchMemoryUsage|TestGarbageCollectionBehavior|TestResourceCleanupAfterRequests|TestMemoryLeakDetection"
            test_files="./tests/performance/resource_usage_test.go"
            ;;
        rate_limit)
            test_pattern="TestOpenAIRateLimitHandling|TestBackoffBehavior|TestBatchRequestOptimization"
            test_files="./tests/performance/rate_limit_test.go"
            ;;
        e2e)
            test_pattern="TestCompleteDemoScenariosUnderLoad|TestThirtySecondTargetComplexQueries|TestMultipleConcurrentDemos"
            test_files="./tests/performance/e2e_performance_test.go"
            ;;
        all)
            test_pattern=""
            test_files="./tests/performance/..."
            ;;
    esac

    local go_test_cmd="go test"

    if [[ "$VERBOSE" == "true" ]]; then
        go_test_cmd="$go_test_cmd -v"
    fi

    if [[ "$TEST_TYPE" == "basic" ]]; then
        go_test_cmd="$go_test_cmd -bench=. -benchmem"
    elif [[ -n "$test_pattern" ]]; then
        go_test_cmd="$go_test_cmd -run=\"$test_pattern\""
    fi

    go_test_cmd="$go_test_cmd -timeout=$TIMEOUT $test_files"

    log_info "Running: $go_test_cmd"

    # Run tests and capture output
    export TEST_MODE="true"
    export CONFIG_PATH="$CONFIG_PATH"

    if eval "$go_test_cmd" 2>&1 | tee "$RESULT_FILE"; then
        log_success "Performance tests completed successfully"
    else
        log_warning "Some performance tests failed or were skipped"
    fi

    # Analyze results
    analyze_results
}

# Analyze test results
analyze_results() {
    log_info "Analyzing test results..."

    if [[ ! -f "$RESULT_FILE" ]]; then
        log_error "Results file not found: $RESULT_FILE"
        return 1
    fi

    local total_tests=$(grep -c "^=== RUN\|^Benchmark" "$RESULT_FILE" 2>/dev/null || echo "0")
    local passed_tests=$(grep -c "^--- PASS\|^PASS" "$RESULT_FILE" 2>/dev/null || echo "0")
    local failed_tests=$(grep -c "^--- FAIL\|^FAIL" "$RESULT_FILE" 2>/dev/null || echo "0")
    local skipped_tests=$(grep -c "^--- SKIP" "$RESULT_FILE" 2>/dev/null || echo "0")

    log_info "Test Results Summary:"
    log_info "  Total tests: $total_tests"
    log_info "  Passed: $passed_tests"
    log_info "  Failed: $failed_tests"
    log_info "  Skipped: $skipped_tests"

    # Extract key performance metrics
    if grep -q "success rate\|response time\|throughput\|requests per second" "$RESULT_FILE"; then
        log_info "Key Performance Metrics:"
        grep -i "success rate\|average response time\|throughput\|requests per second\|memory used" "$RESULT_FILE" | head -10 | sed 's/^/  /'
    fi

    # Check for performance alerts
    if grep -q "exceeded\|timeout\|failed\|error" "$RESULT_FILE"; then
        log_warning "Performance alerts detected - check the full results"
    fi

    log_info "Full results saved to: $RESULT_FILE"
}

# Main execution
main() {
    log_info "AI SA Assistant Performance Testing Script"
    log_info "=========================================="

    check_prerequisites
    start_services
    run_tests

    log_success "Performance testing completed!"
    log_info "Results location: $RESULT_FILE"

    if [[ "$CLEANUP" == "false" ]]; then
        log_info ""
        log_info "Services are still running. To stop them manually, run:"
        log_info "  pkill -f './bin/retrieve'"
        log_info "  pkill -f './bin/websearch'"
        log_info "  pkill -f './bin/synthesize'"
        log_info "  pkill -f './bin/teamsbot'"
    fi
}

# Run main function
main "$@"
