# Summary

The web UI streaming implementation experiences immediate context cancellation that prevents HTTP requests to backend services (retrieve, synthesize) from completing. This results in fallback responses being used instead of actual AI-generated content.

**Impact**: Users see generic fallback messages instead of AI-generated migration plans, architecture diagrams, and code snippets.

## Problem Details

### Working Components ✅
- All backend services are healthy and functional
- ChromaDB contains 400 properly ingested document chunks  
- Direct API calls to retrieve/synthesize services work perfectly
- OpenAI API integration is functional

### Failing Components ❌
- Web UI streaming implementation (cmd/webui/main.go)
- Context propagation through goroutines (internal/teams/orchestrator.go)
- HTTP request execution within streaming context

### Error Pattern
```
"error": "Post \"http://retrieve:8081/search\": context canceled"
"execution_time_ms": 0
```

## Affected Files
- `cmd/webui/main.go` (lines 569-577) - Background context creation
- `internal/teams/orchestrator.go` (lines 401-409, 670-682) - HTTP request execution  
- `cmd/webui/main.go` (lines 621, 667) - SSE connection management

## Investigation Plan

### Phase 1: Context Analysis
1. Trace context propagation path from HTTP request to backend services
2. Identify source of immediate cancellation signal
3. Test hypothesis about SSE connection interference

### Phase 2: Alternative Strategies  
1. Implement true context isolation independent of HTTP lifecycle
2. Create per-request HTTP clients instead of shared orchestrator client
3. Add diagnostic endpoints for internal service communication

### Phase 3: Architecture Review
1. Evaluate queue-based processing vs real-time streaming
2. Consider non-streaming endpoints as interim solution
3. Implement proper goroutine lifecycle management

## Acceptance Criteria
- [ ] Web UI streaming requests complete without context cancellation
- [ ] Backend services receive and process requests normally  
- [ ] Users receive AI-generated responses with diagrams and code
- [ ] End-to-end response time < 30 seconds for typical queries
- [ ] Integration tests pass for all 4 demo scenarios

## Priority: Critical
Core functionality (AI responses) is completely broken in web UI while backend services are healthy but inaccessible through primary interface.