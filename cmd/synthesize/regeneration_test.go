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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetParameterPresets(t *testing.T) {
	presets := getParameterPresets()

	// Test that all expected presets exist
	expectedPresets := []string{"creative", "balanced", "focused", "detailed", "concise"}
	assert.Len(t, presets, len(expectedPresets))

	for _, preset := range expectedPresets {
		assert.Contains(t, presets, preset)

		p := presets[preset]
		assert.NotEmpty(t, p.Name)
		assert.NotEmpty(t, p.Description)
		assert.NotEmpty(t, p.Model)
		assert.Greater(t, p.Temperature, float32(0.0))
		assert.LessOrEqual(t, p.Temperature, float32(2.0))
		assert.Greater(t, p.MaxTokens, 0)
		assert.LessOrEqual(t, p.MaxTokens, 8000)
	}
}

func TestValidateGenerationParams(t *testing.T) {
	tests := []struct {
		name    string
		params  GenerationParams
		wantErr bool
	}{
		{
			name: "valid creative preset",
			params: GenerationParams{
				Preset:      "creative",
				Temperature: 0.8,
				MaxTokens:   3000,
				Model:       "gpt-4o",
			},
			wantErr: false,
		},
		{
			name: "invalid preset",
			params: GenerationParams{
				Preset:      "invalid",
				Temperature: 0.5,
				MaxTokens:   2000,
				Model:       "gpt-4o",
			},
			wantErr: true,
		},
		{
			name: "temperature too high",
			params: GenerationParams{
				Preset:      "balanced",
				Temperature: 3.0,
				MaxTokens:   2000,
				Model:       "gpt-4o",
			},
			wantErr: true,
		},
		{
			name: "temperature too low",
			params: GenerationParams{
				Preset:      "balanced",
				Temperature: -0.1,
				MaxTokens:   2000,
				Model:       "gpt-4o",
			},
			wantErr: true,
		},
		{
			name: "max tokens too high",
			params: GenerationParams{
				Preset:      "balanced",
				Temperature: 0.5,
				MaxTokens:   10000,
				Model:       "gpt-4o",
			},
			wantErr: true,
		},
		{
			name: "max tokens too low",
			params: GenerationParams{
				Preset:      "balanced",
				Temperature: 0.5,
				MaxTokens:   50,
				Model:       "gpt-4o",
			},
			wantErr: true,
		},
		{
			name: "invalid model",
			params: GenerationParams{
				Preset:      "balanced",
				Temperature: 0.5,
				MaxTokens:   2000,
				Model:       "invalid-model",
			},
			wantErr: true,
		},
		{
			name: "valid parameters without preset",
			params: GenerationParams{
				Temperature: 0.3,
				MaxTokens:   1500,
				Model:       "gpt-4o-mini",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerationParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyParameterPreset(t *testing.T) {
	tests := []struct {
		name     string
		params   GenerationParams
		expected GenerationParams
	}{
		{
			name: "apply creative preset",
			params: GenerationParams{
				Preset: "creative",
			},
			expected: GenerationParams{
				Preset:      "creative",
				Temperature: 0.8,
				MaxTokens:   3000,
				Model:       "gpt-4o",
			},
		},
		{
			name: "apply focused preset",
			params: GenerationParams{
				Preset: "focused",
			},
			expected: GenerationParams{
				Preset:      "focused",
				Temperature: 0.1,
				MaxTokens:   2000,
				Model:       "gpt-4o",
			},
		},
		{
			name: "preserve explicit values",
			params: GenerationParams{
				Preset:      "creative",
				Temperature: 0.5,  // Explicit value should be preserved
				MaxTokens:   1500, // Explicit value should be preserved
			},
			expected: GenerationParams{
				Preset:      "creative",
				Temperature: 0.5,      // Preserved
				MaxTokens:   1500,     // Preserved
				Model:       "gpt-4o", // Applied from preset
			},
		},
		{
			name: "no preset specified",
			params: GenerationParams{
				Temperature: 0.6,
				MaxTokens:   2500,
				Model:       "gpt-4o-mini",
			},
			expected: GenerationParams{
				Temperature: 0.6,
				MaxTokens:   2500,
				Model:       "gpt-4o-mini",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := tt.params
			applyParameterPreset(&params)
			assert.Equal(t, tt.expected, params)
		})
	}
}

func TestValidateRegenerationRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RegenerationRequest
		wantErr bool
	}{
		{
			name: "valid regeneration request",
			req: RegenerationRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{Text: "test chunk", DocID: "doc1"},
				},
				Parameters: GenerationParams{
					Preset:      "balanced",
					Temperature: 0.4,
					MaxTokens:   2000,
					Model:       "gpt-4o",
				},
			},
			wantErr: false,
		},
		{
			name: "missing query",
			req: RegenerationRequest{
				Query: "",
				Chunks: []ChunkItem{
					{Text: "test chunk", DocID: "doc1"},
				},
				Parameters: GenerationParams{
					Preset: "balanced",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid parameters",
			req: RegenerationRequest{
				Query: "Test query",
				Chunks: []ChunkItem{
					{Text: "test chunk", DocID: "doc1"},
				},
				Parameters: GenerationParams{
					Preset:      "invalid",
					Temperature: 0.4,
					MaxTokens:   2000,
					Model:       "gpt-4o",
				},
			},
			wantErr: true,
		},
		{
			name: "no chunks or web results",
			req: RegenerationRequest{
				Query:      "Test query",
				Chunks:     []ChunkItem{},
				WebResults: []WebResult{},
				Parameters: GenerationParams{
					Preset: "balanced",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegenerationRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildRegenerationPrompt(t *testing.T) {
	query := "How do I migrate to AWS?"
	webResults := []string{"Web result about AWS migration"}
	previousResponse := "Previous response about AWS migration"

	// Test with previous response
	prompt := buildRegenerationPrompt(query, nil, webResults, nil, &previousResponse)
	assert.Contains(t, prompt, query)
	assert.Contains(t, prompt, "Previous Response")
	assert.Contains(t, prompt, previousResponse)
	assert.Contains(t, prompt, "Regeneration Instructions")
	assert.Contains(t, prompt, "alternative response")

	// Test without previous response
	prompt2 := buildRegenerationPrompt(query, nil, webResults, nil, nil)
	assert.Contains(t, prompt2, query)
	assert.NotContains(t, prompt2, "Previous Response")
	assert.NotContains(t, prompt2, "Regeneration Instructions")
}
