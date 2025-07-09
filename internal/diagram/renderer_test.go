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

package diagram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

const testDiagram = "graph TD\n    A[Start] --> B[End]"

func TestNewRenderer(t *testing.T) {
	config := DefaultRendererConfig()
	logger := zap.NewNop()

	renderer := NewRenderer(config, logger)

	if renderer == nil {
		t.Fatal("Expected renderer to be created")
	}

	if renderer.config.MermaidInkURL != DefaultMermaidInkURL {
		t.Errorf("Expected MermaidInkURL to be %s, got %s", DefaultMermaidInkURL, renderer.config.MermaidInkURL)
	}

	if renderer.httpClient.Timeout != DefaultTimeout {
		t.Errorf("Expected timeout to be %v, got %v", DefaultTimeout, renderer.httpClient.Timeout)
	}
}

func TestValidateDiagramCode(t *testing.T) {
	config := DefaultRendererConfig()
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "Valid graph TD diagram",
			code:    "graph TD\n    A[Start] --> B[End]",
			wantErr: false,
		},
		{
			name:    "Valid graph LR diagram",
			code:    "graph LR\n    A[Start] --> B[End]",
			wantErr: false,
		},
		{
			name:    "Empty code",
			code:    "",
			wantErr: true,
		},
		{
			name:    "Whitespace only",
			code:    "   \n\t  ",
			wantErr: true,
		},
		{
			name:    "Invalid Mermaid syntax",
			code:    "This is not a valid diagram",
			wantErr: true,
		},
		{
			name:    "Code too large",
			code:    "graph TD\n" + strings.Repeat("A --> B\n", 10000),
			wantErr: true,
		},
		{
			name:    "Contains script tag",
			code:    "graph TD\n    A[<script>alert('xss')</script>] --> B[End]",
			wantErr: true,
		},
		{
			name:    "Contains javascript",
			code:    "graph TD\n    A[javascript:alert('xss')] --> B[End]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renderer.validateDiagramCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDiagramCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContainsMaliciousContent(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name:     "Safe diagram",
			code:     "graph TD\n    A[Start] --> B[End]",
			expected: false,
		},
		{
			name:     "Script tag",
			code:     "graph TD\n    A[<script>alert('xss')</script>] --> B[End]",
			expected: true,
		},
		{
			name:     "JavaScript URL",
			code:     "graph TD\n    A[javascript:alert('xss')] --> B[End]",
			expected: true,
		},
		{
			name:     "onclick handler",
			code:     "graph TD\n    A[onclick=alert('xss')] --> B[End]",
			expected: true,
		},
		{
			name:     "eval function",
			code:     "graph TD\n    A[eval('malicious')] --> B[End]",
			expected: true,
		},
		{
			name:     "Case insensitive detection",
			code:     "graph TD\n    A[<SCRIPT>alert('xss')</SCRIPT>] --> B[End]",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsMaliciousContent(tt.code)
			if result != tt.expected {
				t.Errorf("containsMaliciousContent() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestValidateRenderedURL(t *testing.T) {
	config := DefaultRendererConfig()
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "Valid mermaid.ink URL",
			url:     "https://mermaid.ink/img/base64code",
			wantErr: false,
		},
		{
			name:    "Valid www.mermaid.ink URL",
			url:     "https://www.mermaid.ink/img/base64code",
			wantErr: false,
		},
		{
			name:    "HTTP URL (allowed)",
			url:     "http://mermaid.ink/img/base64code",
			wantErr: false,
		},
		{
			name:    "Empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "Invalid scheme",
			url:     "ftp://mermaid.ink/img/base64code",
			wantErr: true,
		},
		{
			name:    "Invalid host",
			url:     "https://malicious.com/img/base64code",
			wantErr: true,
		},
		{
			name:    "Invalid URL format",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renderer.validateRenderedURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRenderedURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCaching(t *testing.T) {
	config := DefaultRendererConfig()
	config.EnableCaching = true
	config.CacheExpiry = 100 * time.Millisecond
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Test cache miss
	diagram := testDiagram
	url, found := renderer.getCachedDiagram(diagram)
	if found {
		t.Error("Expected cache miss for new diagram")
	}
	if url != "" {
		t.Error("Expected empty URL for cache miss")
	}

	// Test cache hit
	testURL := "https://mermaid.ink/img/testcode"
	renderer.cacheDiagram(diagram, testURL)

	url, found = renderer.getCachedDiagram(diagram)
	if !found {
		t.Error("Expected cache hit for cached diagram")
	}
	if url != testURL {
		t.Errorf("Expected URL %s, got %s", testURL, url)
	}

	// Test cache expiry
	time.Sleep(150 * time.Millisecond)
	_, found = renderer.getCachedDiagram(diagram)
	if found {
		t.Error("Expected cache miss for expired diagram")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	config := DefaultRendererConfig()
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	diagram1 := testDiagram
	diagram2 := "graph TD\n    C[Begin] --> D[Finish]"

	key1 := renderer.generateCacheKey(diagram1)
	key2 := renderer.generateCacheKey(diagram2)
	key3 := renderer.generateCacheKey(diagram1) // Same as diagram1

	if key1 == key2 {
		t.Error("Different diagrams should have different cache keys")
	}

	if key1 != key3 {
		t.Error("Same diagrams should have same cache keys")
	}

	if len(key1) != 32 { // MD5 hash length
		t.Errorf("Expected cache key length 32, got %d", len(key1))
	}
}

func TestRenderDiagramViaAPI(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("User-Agent") != "AI-SA-Assistant/1.0" {
			t.Error("Expected proper User-Agent header")
		}

		// Check URL path
		if !strings.HasPrefix(r.URL.Path, "/img/") {
			t.Error("Expected URL path to start with /img/")
		}

		// Return a mock SVG
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<svg>test</svg>`))
	}))
	defer server.Close()

	// Configure renderer to use mock server
	config := DefaultRendererConfig()
	config.MermaidInkURL = server.URL + "/img"
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Test successful rendering
	diagram := testDiagram
	ctx := context.Background()

	url, err := renderer.renderDiagramViaAPI(ctx, diagram)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !strings.HasPrefix(url, server.URL+"/img/") {
		t.Errorf("Expected URL to start with %s/img/, got %s", server.URL, url)
	}
}

func TestRenderDiagramViaAPI_ErrorHandling(t *testing.T) {
	// Create a mock server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	// Configure renderer to use mock server
	config := DefaultRendererConfig()
	config.MermaidInkURL = server.URL + "/img"
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Test error handling
	diagram := testDiagram
	ctx := context.Background()

	_, err := renderer.renderDiagramViaAPI(ctx, diagram)
	if err == nil {
		t.Error("Expected error for server error response")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain status code 500, got %v", err)
	}
}

func TestRenderDiagramWithFallback(t *testing.T) {
	// Create a mock server that fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	// Configure renderer to use mock server
	config := DefaultRendererConfig()
	config.MermaidInkURL = server.URL + "/img"
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Test fallback behavior
	diagram := testDiagram
	ctx := context.Background()

	imageURL, fallbackText, err := renderer.RenderDiagramWithFallback(ctx, diagram)
	if err != nil {
		t.Errorf("Expected no error from fallback, got %v", err)
	}

	if imageURL != "" {
		t.Error("Expected empty image URL when rendering fails")
	}

	if fallbackText == "" {
		t.Error("Expected fallback text when rendering fails")
	}

	if !strings.Contains(fallbackText, "Architecture Diagram") {
		t.Error("Expected fallback text to contain diagram title")
	}

	if !strings.Contains(fallbackText, diagram) {
		t.Error("Expected fallback text to contain original diagram code")
	}
}

func TestGetCacheStats(t *testing.T) {
	config := DefaultRendererConfig()
	config.EnableCaching = true
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Add some cache entries
	renderer.cacheDiagram("diagram1", "url1")
	renderer.cacheDiagram("diagram2", "url2")

	stats := renderer.GetCacheStats()

	if stats["total_entries"] != 2 {
		t.Errorf("Expected total_entries to be 2, got %v", stats["total_entries"])
	}

	if stats["max_size"] != MaxCacheSize {
		t.Errorf("Expected max_size to be %d, got %v", MaxCacheSize, stats["max_size"])
	}

	if stats["expired_entries"] != 0 {
		t.Errorf("Expected expired_entries to be 0, got %v", stats["expired_entries"])
	}
}

func TestClearCache(t *testing.T) {
	config := DefaultRendererConfig()
	config.EnableCaching = true
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	// Add cache entries
	renderer.cacheDiagram("diagram1", "url1")
	renderer.cacheDiagram("diagram2", "url2")

	// Verify entries exist
	stats := renderer.GetCacheStats()
	if stats["total_entries"] != 2 {
		t.Error("Expected 2 cache entries before clearing")
	}

	// Clear cache
	renderer.ClearCache()

	// Verify cache is empty
	stats = renderer.GetCacheStats()
	if stats["total_entries"] != 0 {
		t.Error("Expected 0 cache entries after clearing")
	}
}

func TestCreateFallbackText(t *testing.T) {
	config := DefaultRendererConfig()
	logger := zap.NewNop()
	renderer := NewRenderer(config, logger)

	diagram := testDiagram
	fallbackText := renderer.createFallbackText(diagram)

	if !strings.Contains(fallbackText, "Architecture Diagram") {
		t.Error("Expected fallback text to contain diagram title")
	}

	if !strings.Contains(fallbackText, diagram) {
		t.Error("Expected fallback text to contain original diagram code")
	}

	if !strings.Contains(fallbackText, "temporarily unavailable") {
		t.Error("Expected fallback text to explain unavailability")
	}

	if !strings.Contains(fallbackText, "```") {
		t.Error("Expected fallback text to contain code block formatting")
	}
}
