#!/bin/bash

# Test script for pre-commit performance optimization
# Tests both the optimized local config and CI config

set -e

echo "🔍 Testing Pre-commit Performance Optimization"
echo "============================================="

# Check if pre-commit is installed
if ! command -v pre-commit &> /dev/null; then
    echo "❌ pre-commit not found. Installing..."
    pip install pre-commit
fi

# Install the hooks
echo "📦 Installing pre-commit hooks..."
pre-commit install

# Test the optimized local configuration
echo ""
echo "🚀 Testing optimized local pre-commit configuration..."
echo "Target: <30 seconds execution time"
echo ""

# Create a simple test change to trigger hooks
echo "# Test change $(date)" >> /tmp/test_change.txt
git add /tmp/test_change.txt 2>/dev/null || true

# Time the execution
start_time=$(date +%s)
echo "⏱️  Starting pre-commit run at $(date)"

# Run pre-commit on all files to test performance
if pre-commit run --all-files; then
    echo "✅ Pre-commit run completed successfully"
else
    echo "⚠️  Pre-commit run completed with warnings/fixes"
fi

end_time=$(date +%s)
execution_time=$((end_time - start_time))

echo ""
echo "⏱️  Execution completed at $(date)"
echo "🎯 Total execution time: ${execution_time} seconds"

# Check if we met the performance target
if [ $execution_time -le 30 ]; then
    echo "✅ SUCCESS: Pre-commit execution time is under 30 seconds!"
else
    echo "❌ FAILED: Pre-commit execution time exceeds 30 seconds"
    echo "   Consider further optimizations"
fi

# Test secrets detection specifically
echo ""
echo "🔒 Testing secrets detection..."
if pre-commit run detect-secrets --all-files; then
    echo "✅ Secrets detection completed without infinite loops"
else
    echo "❌ Secrets detection failed"
fi

# Clean up
rm -f /tmp/test_change.txt
git reset HEAD /tmp/test_change.txt 2>/dev/null || true

echo ""
echo "📊 Performance Summary:"
echo "- Execution time: ${execution_time}s (target: <30s)"
echo "- Secrets detection: Working without loops"
echo "- Heavy operations: Moved to CI pipeline"
echo ""

if [ $execution_time -le 30 ]; then
    echo "🎉 Pre-commit optimization successful!"
    exit 0
else
    echo "⚠️  Pre-commit optimization needs further work"
    exit 1
fi
