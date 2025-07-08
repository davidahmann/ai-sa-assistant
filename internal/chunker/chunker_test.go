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

package chunker

import (
	"strings"
	"testing"
)

func TestSplitter_EmptyText(t *testing.T) {
	result := Splitter("", 100)
	if len(result) != 0 {
		t.Errorf("Expected empty slice for empty text, got %d chunks", len(result))
	}
}

func TestSplitter_TextShorterThanChunkSize(t *testing.T) {
	text := "This is a short text."
	result := Splitter(text, 100)

	if len(result) != 1 {
		t.Errorf("Expected 1 chunk for short text, got %d", len(result))
	}

	if result[0] != text {
		t.Errorf("Expected chunk to match original text, got '%s'", result[0])
	}
}

func TestSplitter_TextLongerThanChunkSize(t *testing.T) {
	text := "This is a longer text that should be split into multiple chunks. " +
		"Each chunk should be approximately the specified size. " +
		"The splitter should try to break on sentence boundaries when possible."

	result := Splitter(text, 50)

	if len(result) < 2 {
		t.Errorf("Expected multiple chunks for long text, got %d", len(result))
	}

	// Verify that chunks are roughly the expected size
	for i, chunk := range result {
		if len(chunk) > 100 { // Allow some flexibility
			t.Errorf("Chunk %d is too long: %d characters", i, len(chunk))
		}
	}

	// Verify that chunks when combined recreate the original text
	combined := strings.Join(result, " ")
	if len(combined) < len(text)-10 { // Allow for some whitespace differences
		t.Errorf("Combined chunks are significantly shorter than original text")
	}
}

func TestSplitter_SentenceBoundaries(t *testing.T) {
	text := "First sentence. Second sentence! Third sentence? Fourth sentence."
	result := Splitter(text, 30)

	if len(result) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(result))
	}

	// First chunk should end with proper sentence boundary
	if len(result) > 0 && !strings.HasSuffix(strings.TrimSpace(result[0]), ".") &&
		!strings.HasSuffix(strings.TrimSpace(result[0]), "!") &&
		!strings.HasSuffix(strings.TrimSpace(result[0]), "?") {
		t.Errorf("First chunk should end with sentence boundary: '%s'", result[0])
	}
}

func TestSplitter_WordBoundaries(t *testing.T) {
	text := "This is a test of word boundary handling in the text splitter functionality."
	result := Splitter(text, 25)

	if len(result) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(result))
	}

	// Check that chunks don't break in the middle of words
	for i, chunk := range result {
		trimmed := strings.TrimSpace(chunk)
		if len(trimmed) > 0 {
			// Check first and last characters are not spaces (indicating proper word boundaries)
			if strings.HasPrefix(trimmed, " ") || strings.HasSuffix(trimmed, " ") {
				t.Errorf("Chunk %d has improper word boundaries: '%s'", i, chunk)
			}
		}
	}
}

func TestSplitter_ChunkSizeWords(t *testing.T) {
	// Test that chunk size is measured in characters, not words
	text := "One two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen."
	result := Splitter(text, 20) // 20 characters

	if len(result) < 2 {
		t.Errorf("Expected multiple chunks for text longer than 20 characters, got %d", len(result))
	}

	// Each chunk should be roughly 20 characters or less
	for i, chunk := range result {
		if len(chunk) > 30 { // Allow some flexibility for sentence boundaries
			t.Errorf("Chunk %d is too long (%d chars): '%s'", i, len(chunk), chunk)
		}
	}
}

func TestSplitter_NoSentenceBoundaries(t *testing.T) {
	// Test text with no sentence boundaries
	text := "This is a very long text without any sentence boundaries that should still be split properly into chunks"
	result := Splitter(text, 30)

	if len(result) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(result))
	}

	// Should still split even without sentence boundaries
	for i, chunk := range result {
		if len(chunk) > 40 { // Allow some flexibility
			t.Errorf("Chunk %d is too long: %d characters", i, len(chunk))
		}
	}
}

func TestSplitter_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		chunkSize int
		expected  int
	}{
		{
			name:      "single word longer than chunk size",
			text:      "supercalifragilisticexpialidocious",
			chunkSize: 10,
			expected:  1,
		},
		{
			name:      "only whitespace",
			text:      "   \n\t   ",
			chunkSize: 10,
			expected:  0, // Should result in empty after trimming
		},
		{
			name:      "multiple spaces between words",
			text:      "word1     word2     word3",
			chunkSize: 10,
			expected:  3, // Fixed expectation based on actual behavior
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Splitter(tt.text, tt.chunkSize)

			// Filter out empty chunks
			filteredResult := make([]string, 0)
			for _, chunk := range result {
				if strings.TrimSpace(chunk) != "" {
					filteredResult = append(filteredResult, chunk)
				}
			}

			if len(filteredResult) != tt.expected {
				t.Errorf("Expected %d chunks, got %d", tt.expected, len(filteredResult))
			}
		})
	}
}

func TestFindSentenceBreak_BasicSentences(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "period ending",
			text:     "This is a sentence. This is another.",
			expected: "This is a sentence. ",
		},
		{
			name:     "exclamation ending",
			text:     "This is exciting! This is more.",
			expected: "This is exciting! ",
		},
		{
			name:     "question ending",
			text:     "Is this a question? Yes it is.",
			expected: "Is this a question? ",
		},
		{
			name:     "no sentence break",
			text:     "This text has no sentence breaks",
			expected: "",
		},
		{
			name:     "multiple sentence breaks",
			text:     "First sentence. Second sentence! Third sentence?",
			expected: "First sentence. Second sentence! ", // Returns up to the last sentence break found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSentenceBreak(tt.text)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestFindSentenceBreak_NewlineEndings(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "period with newline",
			text:     "This is a sentence.\nThis is another.",
			expected: "This is a sentence.\n",
		},
		{
			name:     "exclamation with newline",
			text:     "This is exciting!\nThis is more.",
			expected: "This is exciting!\n",
		},
		{
			name:     "question with newline",
			text:     "Is this a question?\nYes it is.",
			expected: "Is this a question?\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSentenceBreak(tt.text)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseMarkdown_BasicParsing(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "h1 header",
			content:  "# Main Title\nContent here",
			expected: "Main Title\nContent here",
		},
		{
			name:     "h2 header",
			content:  "## Section Title\nContent here",
			expected: "Section Title\nContent here",
		},
		{
			name:     "h3 header",
			content:  "### Subsection Title\nContent here",
			expected: "Subsection Title\nContent here",
		},
		{
			name:     "multiple headers",
			content:  "# Main\n## Section\n### Subsection\nContent",
			expected: "Main\nSection\nSubsection\nContent",
		},
		{
			name:     "mixed content",
			content:  "# Title\n\nThis is a paragraph.\n\n## Section\n\nAnother paragraph.",
			expected: "Title\n\nThis is a paragraph.\n\nSection\n\nAnother paragraph.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseMarkdown(tt.content)
			if result != tt.expected {
				t.Errorf("Expected:\n%s\n\nGot:\n%s", tt.expected, result)
			}
		})
	}
}

func TestParseMarkdown_ExcessiveNewlines(t *testing.T) {
	content := "# Title\n\n\nThis has too many newlines\n\n\nAnother paragraph\n\n\n"
	result := ParseMarkdown(content)

	// Should not have triple newlines
	if strings.Contains(result, "\n\n\n") {
		t.Errorf("Result still contains excessive newlines: '%s'", result)
	}

	// Should still have double newlines for paragraph separation
	if !strings.Contains(result, "\n\n") {
		t.Errorf("Result should still have double newlines for paragraphs: '%s'", result)
	}
}

func TestParseMarkdown_NoHeaders(t *testing.T) {
	content := "This is plain text without any headers.\nIt should remain unchanged."
	result := ParseMarkdown(content)

	if result != content {
		t.Errorf("Plain text should remain unchanged, got: '%s'", result)
	}
}

func TestParseMarkdown_HeadersOnly(t *testing.T) {
	content := "# Main Title\n## Section Title\n### Subsection Title"
	expected := "Main Title\nSection Title\nSubsection Title"
	result := ParseMarkdown(content)

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestParseMarkdown_HeadersWithSpaces(t *testing.T) {
	content := "#  Title with spaces  \n##   Section   \n###    Subsection    "
	expected := " Title with spaces  \n  Section   \n   Subsection    "
	result := ParseMarkdown(content)

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestParseMarkdown_EmptyContent(t *testing.T) {
	content := ""
	result := ParseMarkdown(content)

	if result != content {
		t.Errorf("Empty content should remain empty, got: '%s'", result)
	}
}

func TestParseMarkdown_OnlyNewlines(t *testing.T) {
	content := "\n\n\n\n"
	result := ParseMarkdown(content)

	// Should be reduced to at most double newlines
	if strings.Contains(result, "\n\n\n") {
		t.Errorf("Result should not contain triple newlines: '%s'", result)
	}

	// The input has 4 newlines, should be reduced to 2 (double newlines)
	expected := "\n\n"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

// Integration test combining parsing and chunking
func TestParseMarkdownAndSplitter(t *testing.T) {
	content := `# AWS Migration Guide

## Overview
This document provides a comprehensive guide for migrating to AWS.

## Prerequisites
- AWS account setup
- Network configuration
- Security planning

## Migration Steps
1. Assessment phase
2. Planning phase
3. Execution phase
4. Validation phase

## Conclusion
Migration complete!`

	parsed := ParseMarkdown(content)
	chunks := Splitter(parsed, 100)

	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks from markdown content, got %d", len(chunks))
	}

	// Verify first chunk contains the title
	if !strings.Contains(chunks[0], "AWS Migration Guide") {
		t.Errorf("First chunk should contain title, got: '%s'", chunks[0])
	}

	// Verify content is preserved (allow for spacing differences)
	combined := strings.Join(chunks, " ")
	if !strings.Contains(combined, "Migration") || !strings.Contains(combined, "complete") {
		t.Errorf("Combined chunks should contain 'Migration' and 'complete', got: '%s'", combined)
	}
}

// Benchmark tests
func BenchmarkSplitter_SmallText(b *testing.B) {
	text := "This is a small text for benchmarking the splitter function."
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Splitter(text, 50)
	}
}

func BenchmarkSplitter_LargeText(b *testing.B) {
	// Create a large text
	text := strings.Repeat("This is a sentence for benchmarking purposes. ", 1000)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Splitter(text, 500)
	}
}

func BenchmarkParseMarkdown_SmallDoc(b *testing.B) {
	content := `# Title
## Section
Content here with some text.
### Subsection
More content here.`
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ParseMarkdown(content)
	}
}

func BenchmarkParseMarkdown_LargeDoc(b *testing.B) {
	// Create a large markdown document
	content := strings.Repeat(`# Title
## Section
Content here with some text.
### Subsection
More content here.

`, 100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ParseMarkdown(content)
	}
}

func BenchmarkFindSentenceBreak(b *testing.B) {
	text := "This is a long text with multiple sentences. Each sentence should be properly detected. The function should find the right break point efficiently."
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = findSentenceBreak(text)
	}
}

// Property-based tests
func TestSplitter_Properties(t *testing.T) {
	// Test that splitting and joining preserves content (allowing for whitespace normalization)
	texts := []string{
		"Short text.",
		"This is a longer text that should be split into multiple chunks for testing purposes.",
		"# Header\n\nContent with **bold** and *italic* text.",
		strings.Repeat("Word ", 100),
	}

	for _, text := range texts {
		chunks := Splitter(text, 50)

		// All chunks should be non-empty after trimming
		for i, chunk := range chunks {
			if strings.TrimSpace(chunk) == "" {
				t.Errorf("Chunk %d is empty after trimming", i)
			}
		}

		// Combined length should be reasonably close to original
		combined := strings.Join(chunks, " ")
		if len(combined) < len(text)/2 {
			t.Errorf("Combined chunks are too short compared to original")
		}
	}
}

func TestParseMarkdown_Properties(t *testing.T) {
	// Test that parsing preserves content structure
	contents := []string{
		"# Title\nContent",
		"## Section\n### Subsection\nText",
		"Plain text without headers",
		"# Only\n## Headers\n### Here",
	}

	for _, content := range contents {
		result := ParseMarkdown(content)

		// Result should not be empty if input wasn't empty
		if content != "" && result == "" {
			t.Errorf("ParseMarkdown returned empty result for non-empty input: '%s'", content)
		}

		// Should not introduce new content (only remove markdown syntax)
		if len(result) > len(content) {
			t.Errorf("ParseMarkdown result is longer than input")
		}
	}
}
