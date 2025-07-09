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

// Package chunker provides functionality for splitting documents into smaller chunks
// suitable for embedding and retrieval. It supports various chunking strategies
// including section-based chunking and metadata extraction.
package chunker

import (
	"strings"
)

// Splitter splits text into chunks based on the specified chunk size
// It attempts to split on sentence boundaries to maintain semantic meaning
func Splitter(text string, chunkSize int) []string {
	if text == "" {
		return []string{}
	}

	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	words := strings.Fields(text)

	var currentChunk strings.Builder
	wordCount := 0

	for _, word := range words {
		// Check if adding this word would exceed the chunk size
		if wordCount > 0 && currentChunk.Len()+len(word)+1 > chunkSize {
			// Try to find a good breaking point (sentence end)
			chunk := currentChunk.String()
			if lastSentence := findSentenceBreak(chunk); lastSentence != "" {
				chunks = append(chunks, strings.TrimSpace(lastSentence))
				// Start new chunk with remaining text
				remaining := strings.TrimSpace(chunk[len(lastSentence):])
				currentChunk.Reset()
				if remaining != "" {
					currentChunk.WriteString(remaining)
					currentChunk.WriteString(" ")
				}
				wordCount = len(strings.Fields(remaining))
			} else {
				// No sentence break found, use the whole chunk
				chunks = append(chunks, strings.TrimSpace(chunk))
				currentChunk.Reset()
				wordCount = 0
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(word)
		wordCount++
	}

	// Add the last chunk if it's not empty
	if currentChunk.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(currentChunk.String()))
	}

	return chunks
}

// findSentenceBreak finds the last sentence boundary in the text
func findSentenceBreak(text string) string {
	sentenceEnders := []string{". ", "! ", "? ", ".\n", "!\n", "?\n"}

	lastIndex := -1
	for _, ender := range sentenceEnders {
		if idx := strings.LastIndex(text, ender); idx > lastIndex {
			lastIndex = idx + len(ender)
		}
	}

	if lastIndex > 0 {
		return text[:lastIndex]
	}

	return ""
}

// ParseMarkdown extracts and cleans text content from a markdown file
func ParseMarkdown(content string) string {
	// Remove excessive newlines (keep doing until no more triple newlines)
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	// Remove markdown headers (###, ##, #)
	lines := strings.Split(content, "\n")
	var cleanLines []string

	for _, line := range lines {
		// Convert headers to plain text
		switch {
		case strings.HasPrefix(line, "# "):
			cleanLines = append(cleanLines, strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			cleanLines = append(cleanLines, strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "### "):
			cleanLines = append(cleanLines, strings.TrimPrefix(line, "### "))
		default:
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}
