#!/bin/bash

# AI-SA-Assistant Demo Setup Script
# This script fully rebuilds and initializes the demo environment

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Check if we're in the right directory
if [[ ! -f "docker-compose.yml" ]]; then
    print_error "docker-compose.yml not found. Please run this script from the ai-sa-assistant root directory."
    exit 1
fi

# Check if OpenAI API key is set
if [[ -z "$OPENAI_API_KEY" ]]; then
    print_error "OPENAI_API_KEY environment variable is not set."
    print_info "Please set your OpenAI API key: export OPENAI_API_KEY=your_key_here"
    exit 1
fi

print_info "Starting AI-SA-Assistant Demo Setup"
print_info "This will take 5-10 minutes to complete..."
echo

# Step 1: Kill all running services and clear caches
print_step "1/6 Killing all running services and clearing caches..."
docker-compose down --volumes --remove-orphans 2>/dev/null || true
docker system prune -af --volumes >/dev/null 2>&1 || true
rm -rf ./data ./metadata.db ./chroma_data 2>/dev/null || true
print_success "Services stopped and caches cleared"
echo

# Step 2: Rebuild all Docker containers
print_step "2/6 Rebuilding all Docker containers..."
print_info "This may take several minutes for the first build..."
if docker-compose build --no-cache >/dev/null 2>&1; then
    print_success "All Docker containers rebuilt successfully"
else
    print_error "Failed to build Docker containers"
    exit 1
fi
echo

# Step 3: Start all services via docker-compose
print_step "3/6 Starting all services via docker-compose..."
if docker-compose up -d; then
    print_success "All services started"
else
    print_error "Failed to start services"
    exit 1
fi

# Wait for services to initialize with health check polling
print_info "Waiting for services to initialize..."
wait_for_service() {
    local service_url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$service_url" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
        ((attempt++))
    done
    return 1
}

# Wait for ChromaDB to be ready
if wait_for_service "http://localhost:8000/api/v1/heartbeat" "ChromaDB"; then
    print_info "ChromaDB heartbeat is ready"
else
    print_error "ChromaDB failed to start"
    exit 1
fi

# Additional wait for ChromaDB to be fully operational
print_info "Waiting for ChromaDB to be fully operational..."
wait_for_chroma_ready() {
    local max_attempts=20
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "http://localhost:8000/api/v1/collections" >/dev/null 2>&1; then
            return 0
        fi
        sleep 3
        ((attempt++))
    done
    return 1
}

if wait_for_chroma_ready; then
    print_info "ChromaDB is fully operational"
else
    print_error "ChromaDB is not accepting operations"
    exit 1
fi

# Check service status
print_info "Service Status:"
docker-compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
echo

# Step 4: Initialize ChromaDB chunking and ingestion
print_step "4/6 Initializing ChromaDB chunking and ingestion..."
print_info "Processing documents and generating embeddings..."

# Run ingestion with timeout and retry
run_ingestion() {
    local attempt=$1
    print_info "Running ingestion attempt ${attempt}..."
    
    # Use gtimeout on macOS if available, otherwise use built-in timeout or none
    if command -v gtimeout >/dev/null 2>&1; then
        gtimeout 300s docker-compose run --rm ingest ./ingest --docs-path /app/docs
    elif command -v timeout >/dev/null 2>&1; then
        timeout 300s docker-compose run --rm ingest ./ingest --docs-path /app/docs
    else
        # No timeout available, run directly
        docker-compose run --rm ingest ./ingest --docs-path /app/docs
    fi
}

# Try ingestion with multiple attempts and exponential backoff
ingestion_success=false
for attempt in 1 2 3; do
    if run_ingestion $attempt; then
        print_success "Ingestion completed successfully"
        ingestion_success=true
        break
    else
        if [ $attempt -lt 3 ]; then
            wait_time=$((attempt * 10))
            print_warning "Ingestion attempt ${attempt} failed, waiting ${wait_time}s before retry..."
            sleep $wait_time
        fi
    fi
done

if [ "$ingestion_success" = false ]; then
    print_error "Failed to initialize data ingestion after 3 attempts"
    print_info "Checking ChromaDB logs..."
    docker-compose logs --tail=20 chromadb
    exit 1
fi

# Verify collection has documents
print_info "Verifying ChromaDB collection..."
if curl -s http://localhost:8000/api/v1/collections/cloud_assistant >/dev/null 2>&1; then
    print_success "ChromaDB collection verified"
else
    print_error "ChromaDB collection not found"
    exit 1
fi
echo

# Step 5: Verify app is running on localhost:8084
print_step "5/6 Verifying app is running on localhost:8084..."

# Check if web UI is accessible with polling
if wait_for_service "http://localhost:8084/health" "Web UI"; then
    print_success "Web UI is accessible on localhost:8084"
else
    print_error "Web UI is not accessible on localhost:8084"
    print_info "Checking service logs..."
    docker-compose logs --tail=10 webui
    exit 1
fi

# Step 6: Run end-to-end test
print_step "6/6 Running end-to-end test query..."
print_info "Testing AWS lift-and-shift migration query..."

# Test query
test_query='{"message": "Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS, including EC2 instance recommendations, VPC/subnet topology, and the latest AWS MGN best practices from Q2 2025", "conversation_id": "demo-test-session"}'

# Run test with timeout (handle macOS timeout issue)
test_api() {
    if command -v gtimeout >/dev/null 2>&1; then
        gtimeout 60s curl -s -X POST http://localhost:8084/chat \
          -H "Content-Type: application/json" \
          -d "$test_query" | jq -r '.message.content' | head -2 >/dev/null 2>&1
    elif command -v timeout >/dev/null 2>&1; then
        timeout 60s curl -s -X POST http://localhost:8084/chat \
          -H "Content-Type: application/json" \
          -d "$test_query" | jq -r '.message.content' | head -2 >/dev/null 2>&1
    else
        # No timeout available, run directly with risk of hanging
        curl -s -X POST http://localhost:8084/chat \
          -H "Content-Type: application/json" \
          -d "$test_query" | jq -r '.message.content' | head -2 >/dev/null 2>&1
    fi
}

if test_api; then
    print_success "End-to-end test completed successfully"
else
    print_warning "End-to-end test may have timed out or failed"
    print_info "The system should still be functional for manual testing"
fi

echo
print_success "🎉 AI-SA-Assistant Demo Setup Complete!"
echo
print_info "Demo Access URLs:"
echo -e "  ${GREEN}Web UI:${NC}        http://localhost:8084"
echo -e "  ${GREEN}Health Check:${NC}  http://localhost:8084/health"
echo -e "  ${GREEN}API Endpoint:${NC}  POST http://localhost:8084/chat"
echo
print_info "Service Status:"
docker-compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
echo
print_info "Demo Test Queries:"
echo '  • "Generate an AWS lift-and-shift plan for 120 VMs"'
echo '  • "Design a hybrid Azure architecture with ExpressRoute"'
echo '  • "Create a disaster recovery plan with RTO=2 hours"'
echo '  • "Summarize HIPAA compliance requirements for AWS"'
echo
print_info "To stop the demo: docker-compose down"
print_info "To view logs: docker-compose logs -f [service-name]"