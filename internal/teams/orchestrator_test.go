// Copyright 2024 AI SA Assistant Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"github.com/your-org/ai-sa-assistant/internal/diagram"
	"github.com/your-org/ai-sa-assistant/internal/health"
	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/synth"
	"go.uber.org/zap/zaptest"
)

const (
	// HealthEndpoint is the health check endpoint path
	HealthEndpoint = "/health"
)

func TestOrchestrator_ProcessQuery_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Test chunk content",
					Score:    0.9,
					DocID:    "test-doc",
					SourceID: "test-source",
					Metadata: map[string]interface{}{"test": true},
				},
			},
			Count: 1,
			Query: "test query",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer retrieveServer.Close()

	websearchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := struct {
			Results []string `json:"results"`
		}{
			Results: []string{"Title: Test\nSnippet: Test snippet\nURL: http://test.com"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer websearchServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText:     "Test synthesis response",
			DiagramCode:  "",
			CodeSnippets: []synth.CodeSnippet{},
			Sources:      []string{"test-source"},
			DiagramURL:   "",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  websearchServer.URL,
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test successful query processing
	ctx := context.Background()
	result := orchestrator.ProcessQuery(ctx, "test query", "test_user")

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}

	if result.Response == nil {
		t.Error("Expected response to be non-nil")
	}

	if len(result.ServicesUsed) == 0 {
		t.Error("Expected services to be used")
	}

	if result.ExecutionTimeMs <= 0 {
		t.Error("Expected execution time to be positive")
	}

	if !result.HealthChecksPassed {
		t.Error("Expected health checks to pass")
	}
}

func TestOrchestrator_ProcessQuery_WithFreshness(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Test chunk content",
					Score:    0.9,
					DocID:    "test-doc",
					SourceID: "test-source",
				},
			},
			Count: 1,
			Query: "latest AWS updates",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer retrieveServer.Close()

	websearchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := struct {
			Results []string `json:"results"`
		}{
			Results: []string{"Title: Latest AWS Updates\nSnippet: Recent AWS announcements\nURL: http://aws.com"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer websearchServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText:     "Latest AWS updates synthesis",
			DiagramCode:  "",
			CodeSnippets: []synth.CodeSnippet{},
			Sources:      []string{"test-source"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  websearchServer.URL,
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test query with freshness keyword
	ctx := context.Background()
	result := orchestrator.ProcessQuery(ctx, "latest AWS updates", "test_user")

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}

	// Should use both retrieve and websearch services
	expectedServices := []string{"retrieve", "websearch", "synthesize"}
	if len(result.ServicesUsed) != len(expectedServices) {
		t.Errorf("Expected %d services used, got %d", len(expectedServices), len(result.ServicesUsed))
	}

	// Check that websearch service was used
	websearchUsed := false
	for _, service := range result.ServicesUsed {
		if service == "websearch" {
			websearchUsed = true
			break
		}
	}
	if !websearchUsed {
		t.Error("Expected websearch service to be used for freshness query")
	}
}

func TestOrchestrator_ProcessQuery_RetrieveServiceFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers - retrieve server returns error
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
	}))
	defer retrieveServer.Close()

	websearchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := struct {
			Results []string `json:"results"`
		}{
			Results: []string{"Title: Test\nSnippet: Test snippet\nURL: http://test.com"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer websearchServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText:     "Fallback synthesis response",
			DiagramCode:  "",
			CodeSnippets: []synth.CodeSnippet{},
			Sources:      []string{"fallback"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  websearchServer.URL,
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test query with retrieve service failure
	ctx := context.Background()
	result := orchestrator.ProcessQuery(ctx, "test query", "test_user")

	if result.Error != nil {
		t.Errorf("Expected no error due to fallback, got %v", result.Error)
	}

	if !result.FallbackUsed {
		t.Error("Expected fallback to be used")
	}

	if result.Response == nil {
		t.Error("Expected response to be non-nil due to fallback")
	}
}

func TestOrchestrator_ProcessQuery_SynthesizeServiceFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers - synthesize server returns error
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Test chunk content",
					Score:    0.9,
					DocID:    "test-doc",
					SourceID: "test-source",
				},
			},
			Count: 1,
			Query: "test query",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer retrieveServer.Close()

	websearchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := struct {
			Results []string `json:"results"`
		}{
			Results: []string{"Title: Test\nSnippet: Test snippet\nURL: http://test.com"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer websearchServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Synthesis service error"))
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  websearchServer.URL,
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test query with synthesize service failure
	ctx := context.Background()
	result := orchestrator.ProcessQuery(ctx, "test query", "test_user")

	if result.Error != nil {
		t.Errorf("Expected no error due to fallback, got %v", result.Error)
	}

	if !result.FallbackUsed {
		t.Error("Expected fallback to be used")
	}

	if result.Response == nil {
		t.Error("Expected response to be non-nil due to fallback")
	}
}

func TestOrchestrator_ProcessQuery_HealthCheckFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers - retrieve server health check fails
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Test chunk content",
					Score:    0.9,
					DocID:    "test-doc",
					SourceID: "test-source",
				},
			},
			Count: 1,
			Query: "test query",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer retrieveServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText:     "Test synthesis response",
			DiagramCode:  "",
			CodeSnippets: []synth.CodeSnippet{},
			Sources:      []string{"test-source"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  "http://invalid-url",
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test query with health check failure
	ctx := context.Background()
	result := orchestrator.ProcessQuery(ctx, "test query", "test_user")

	if result.Error == nil {
		t.Error("Expected error due to health check failure")
	}

	if result.HealthChecksPassed {
		t.Error("Expected health checks to fail")
	}

	if len(result.ServicesTested) == 0 {
		t.Error("Expected services to be tested")
	}
}

func TestOrchestrator_ProcessQuery_Timeout(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create mock servers - retrieve server with delay
	retrieveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Simulate long processing time
		time.Sleep(2 * time.Second)

		response := RetrieveResponse{
			Chunks: []RetrieveChunk{
				{
					Text:     "Test chunk content",
					Score:    0.9,
					DocID:    "test-doc",
					SourceID: "test-source",
				},
			},
			Count: 1,
			Query: "test query",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer retrieveServer.Close()

	synthesizeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthEndpoint {
			w.WriteHeader(http.StatusOK)
			return
		}

		response := synth.SynthesisResponse{
			MainText:     "Test synthesis response",
			DiagramCode:  "",
			CodeSnippets: []synth.CodeSnippet{},
			Sources:      []string{"test-source"},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer synthesizeServer.Close()

	// Create config
	cfg := &config.Config{
		Services: config.ServicesConfig{
			RetrieveURL:   retrieveServer.URL,
			WebSearchURL:  "http://invalid-url",
			SynthesizeURL: synthesizeServer.URL,
		},
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent"},
		},
	}

	// Create health manager
	healthManager := health.NewManager("test", "1.0.0", logger)

	// Create diagram renderer
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	// Create orchestrator
	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	// Test query with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result := orchestrator.ProcessQuery(ctx, "test query", "test_user")

	// Should complete with fallback due to timeout
	if result.Error != nil {
		t.Errorf("Expected no error due to fallback, got %v", result.Error)
	}

	if !result.FallbackUsed {
		t.Error("Expected fallback to be used due to timeout")
	}
}

func TestOrchestrator_needsFreshness(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := &config.Config{
		WebSearch: config.WebSearchConfig{
			FreshnessKeywords: []string{"latest", "recent", "update", "new"},
		},
	}

	healthManager := health.NewManager("test", "1.0.0", logger)
	diagramConfig := diagram.RendererConfig{
		MermaidInkURL:  "https://mermaid.ink/img",
		Timeout:        30,
		EnableCaching:  false,
		MaxDiagramSize: 10240,
	}
	diagramRenderer := diagram.NewRenderer(diagramConfig, logger)

	// Create session manager
	sessionConfig := session.Config{
		StorageType:     session.MemoryStorageType,
		DefaultTTL:      30 * time.Minute,
		MaxSessions:     1000,
		CleanupInterval: 0,
	}
	sessionManager, err := session.NewManager(sessionConfig, logger)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer func() { _ = sessionManager.Close() }()

	orchestrator := NewOrchestrator(cfg, healthManager, diagramRenderer, sessionManager, logger)

	tests := []struct {
		query    string
		expected bool
	}{
		{"latest AWS updates", true},
		{"recent Azure announcements", true},
		{"new features in Google Cloud", true},
		{"update on container security", true},
		{"general cloud architecture", false},
		{"basic networking concepts", false},
		{"LATEST trends in cloud", true}, // Case insensitive
	}

	for _, tt := range tests {
		result := orchestrator.needsFreshness(tt.query)
		if result != tt.expected {
			t.Errorf("needsFreshness(%q) = %v, expected %v", tt.query, result, tt.expected)
		}
	}
}
