package synth

import (
	"fmt"
	"regexp"
	"strings"
)

// ContextItem represents a piece of context with its source
type ContextItem struct {
	Content  string `json:"content"`
	SourceID string `json:"source_id"`
}

// SynthesisRequest represents a request for synthesis
type SynthesisRequest struct {
	Query      string        `json:"query"`
	Context    []ContextItem `json:"context"`
	WebResults []string      `json:"web_results"`
}

// SynthesisResponse represents the structured response from synthesis
type SynthesisResponse struct {
	MainText     string        `json:"main_text"`
	DiagramCode  string        `json:"diagram_code"`
	CodeSnippets []CodeSnippet `json:"code_snippets"`
	Sources      []string      `json:"sources"`
}

// CodeSnippet represents a code snippet with its language
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// BuildPrompt combines context into a comprehensive prompt for the LLM
func BuildPrompt(query string, contextItems []ContextItem, webResults []string) string {
	var prompt strings.Builder

	// System instructions
	prompt.WriteString(`You are an expert Cloud Solutions Architect assistant. Your role is to help Solutions Architects with pre-sales research and planning.

Your response must be structured and comprehensive. Please provide:

1. A detailed, actionable answer to the user's query
2. If applicable, generate a high-level architecture diagram using Mermaid.js graph TD syntax
3. If applicable, provide relevant code snippets for implementation
4. Always cite your sources using [source_id] format when referencing internal documents

Guidelines:
- Be specific and actionable in your recommendations
- Include technical details and best practices
- For diagrams: Use Mermaid.js graph TD syntax enclosed in a "mermaid" code block
- For code: Use appropriate language identifiers (terraform, bash, yaml, etc.)
- Citations: End sentences with [source_id] when using information from internal documents
- Focus on practical implementation guidance

`)

	// User query
	prompt.WriteString(fmt.Sprintf("User Query: %s\n\n", query))

	// Internal document context
	if len(contextItems) > 0 {
		prompt.WriteString("--- Internal Document Context ---\n")
		for i, item := range contextItems {
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	// Web search results
	if len(webResults) > 0 {
		prompt.WriteString("--- Live Web Search Results ---\n")
		for i, result := range webResults {
			prompt.WriteString(fmt.Sprintf("Web Result %d: %s\n\n", i+1, result))
		}
	}

	prompt.WriteString("\nPlease provide your comprehensive response now:")

	return prompt.String()
}

// ParseResponse parses the LLM response into structured components
func ParseResponse(response string) SynthesisResponse {
	result := SynthesisResponse{
		MainText:     response,
		CodeSnippets: []CodeSnippet{},
		Sources:      []string{},
	}

	// Extract diagram code
	if diagramCode := extractMermaidDiagram(response); diagramCode != "" {
		result.DiagramCode = diagramCode
		// Remove diagram from main text
		result.MainText = removeMermaidDiagram(response)
	}

	// Extract code snippets
	codeSnippets := extractCodeSnippets(response)
	result.CodeSnippets = codeSnippets
	// Remove code snippets from main text
	result.MainText = removeCodeSnippets(result.MainText)

	// Extract sources from citations
	sources := extractSources(response)
	result.Sources = uniqueStrings(sources)

	// Clean up main text
	result.MainText = strings.TrimSpace(result.MainText)

	return result
}

// extractMermaidDiagram extracts Mermaid diagram code from the response
func extractMermaidDiagram(response string) string {
	// Look for mermaid code blocks
	mermaidRegex := regexp.MustCompile("```mermaid\\s*\\n([\\s\\S]*?)\\n```")
	matches := mermaidRegex.FindStringSubmatch(response)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Also try without language identifier
	mermaidRegex2 := regexp.MustCompile("```\\s*\\n(graph\\s+TD[\\s\\S]*?)\\n```")
	matches2 := mermaidRegex2.FindStringSubmatch(response)

	if len(matches2) > 1 {
		return strings.TrimSpace(matches2[1])
	}

	return ""
}

// removeMermaidDiagram removes Mermaid diagram blocks from text
func removeMermaidDiagram(text string) string {
	mermaidRegex := regexp.MustCompile("```mermaid\\s*\\n[\\s\\S]*?\\n```")
	text = mermaidRegex.ReplaceAllString(text, "")

	mermaidRegex2 := regexp.MustCompile("```\\s*\\n(graph\\s+TD[\\s\\S]*?)\\n```")
	text = mermaidRegex2.ReplaceAllString(text, "")

	return text
}

// extractCodeSnippets extracts code snippets from the response
func extractCodeSnippets(response string) []CodeSnippet {
	var snippets []CodeSnippet

	// Regex to match code blocks with language identifiers
	codeRegex := regexp.MustCompile("```(\\w+)\\s*\\n([\\s\\S]*?)\\n```")
	matches := codeRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			language := match[1]
			code := strings.TrimSpace(match[2])

			// Skip mermaid blocks (handled separately)
			if language != "mermaid" && code != "" {
				snippets = append(snippets, CodeSnippet{
					Language: language,
					Code:     code,
				})
			}
		}
	}

	return snippets
}

// removeCodeSnippets removes code blocks from text
func removeCodeSnippets(text string) string {
	codeRegex := regexp.MustCompile("```\\w*\\s*\\n[\\s\\S]*?\\n```")
	return codeRegex.ReplaceAllString(text, "")
}

// extractSources extracts source citations from the response
func extractSources(response string) []string {
	var sources []string

	// Regex to match [source_id] patterns
	sourceRegex := regexp.MustCompile(`\[([^\]]+)\]`)
	matches := sourceRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			source := strings.TrimSpace(match[1])
			if source != "" {
				sources = append(sources, source)
			}
		}
	}

	return sources
}

// uniqueStrings removes duplicates from a string slice
func uniqueStrings(strings []string) []string {
	keys := make(map[string]bool)
	var unique []string

	for _, str := range strings {
		if !keys[str] {
			keys[str] = true
			unique = append(unique, str)
		}
	}

	return unique
}

// DetectFreshnessKeywords checks if the query contains keywords indicating need for fresh information
func DetectFreshnessKeywords(query string, keywords []string) bool {
	queryLower := strings.ToLower(query)

	for _, keyword := range keywords {
		if strings.Contains(queryLower, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}
