package synth

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ContextItem represents a piece of context with its source
type ContextItem struct {
	Content  string  `json:"content"`
	SourceID string  `json:"source_id"`
	Score    float64 `json:"score,omitempty"`
	Priority int     `json:"priority,omitempty"`
}

// QueryType represents the type of query for optimization
type QueryType int

const (
	TechnicalQuery QueryType = iota
	BusinessQuery
	GeneralQuery
)

// PromptConfig holds configuration for prompt generation
type PromptConfig struct {
	MaxTokens       int
	MaxContextItems int
	MaxWebResults   int
	QueryType       QueryType
}

// DefaultPromptConfig returns default configuration
func DefaultPromptConfig() PromptConfig {
	return PromptConfig{
		MaxTokens:       4000,
		MaxContextItems: 10,
		MaxWebResults:   5,
		QueryType:       GeneralQuery,
	}
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
	config := DefaultPromptConfig()
	config.QueryType = DetectQueryType(query)
	return BuildPromptWithConfig(query, contextItems, webResults, config)
}

// BuildPromptWithConfig combines context into a comprehensive prompt with configuration
func BuildPromptWithConfig(query string, contextItems []ContextItem, webResults []string, config PromptConfig) string {
	// Prioritize and limit context based on token constraints
	optimizedContext := PrioritizeContext(contextItems, config.MaxContextItems)
	limitedWebResults := LimitWebResults(webResults, config.MaxWebResults)

	var prompt strings.Builder

	// System instructions based on query type
	systemPrompt := buildSystemPrompt(config.QueryType)
	prompt.WriteString(systemPrompt)

	// User query
	prompt.WriteString(fmt.Sprintf("User Query: %s\n\n", query))

	// Internal document context
	if len(optimizedContext) > 0 {
		prompt.WriteString("--- Internal Document Context ---\n")
		for i, item := range optimizedContext {
			prompt.WriteString(fmt.Sprintf("Context %d [%s]: %s\n\n", i+1, item.SourceID, item.Content))
		}
	}

	// Web search results
	if len(limitedWebResults) > 0 {
		prompt.WriteString("--- Live Web Search Results ---\n")
		for i, result := range limitedWebResults {
			prompt.WriteString(fmt.Sprintf("Web Result %d: %s\n\n", i+1, result))
		}
	}

	prompt.WriteString("\nPlease provide your comprehensive response now:")

	// Ensure token limits are respected
	finalPrompt := prompt.String()
	if EstimateTokens(finalPrompt) > config.MaxTokens {
		finalPrompt = TruncateToTokenLimit(finalPrompt, config.MaxTokens)
	}

	return finalPrompt

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

// DetectQueryType analyzes the query to determine its type
func DetectQueryType(query string) QueryType {
	queryLower := strings.ToLower(query)

	// Technical query indicators
	technicalKeywords := []string{
		"architecture", "deploy", "configure", "implementation", "terraform",
		"aws", "azure", "gcp", "kubernetes", "docker", "microservices",
		"api", "database", "network", "security", "vpc", "subnet",
		"ec2", "s3", "lambda", "rds", "cloudformation", "ansible",
	}

	// Business query indicators
	businessKeywords := []string{
		"cost", "pricing", "roi", "budget", "savings", "business case",
		"timeline", "roadmap", "strategy", "compliance", "governance",
		"risk", "sla", "kpi", "metrics", "performance", "scalability",
	}

	technicalScore := 0
	businessScore := 0

	for _, keyword := range technicalKeywords {
		if strings.Contains(queryLower, keyword) {
			technicalScore++
		}
	}

	for _, keyword := range businessKeywords {
		if strings.Contains(queryLower, keyword) {
			businessScore++
		}
	}

	if technicalScore > businessScore {
		return TechnicalQuery
	} else if businessScore > technicalScore {
		return BusinessQuery
	}

	return GeneralQuery
}

// PrioritizeContext prioritizes and limits context items based on score and priority
func PrioritizeContext(contextItems []ContextItem, maxItems int) []ContextItem {
	// Sort by priority (higher first), then by score (higher first)
	sortedItems := make([]ContextItem, len(contextItems))
	copy(sortedItems, contextItems)

	// Simple bubble sort for priority and score
	for i := 0; i < len(sortedItems); i++ {
		for j := i + 1; j < len(sortedItems); j++ {
			if sortedItems[i].Priority < sortedItems[j].Priority ||
				(sortedItems[i].Priority == sortedItems[j].Priority && sortedItems[i].Score < sortedItems[j].Score) {
				sortedItems[i], sortedItems[j] = sortedItems[j], sortedItems[i]
			}
		}
	}

	// Return up to maxItems
	if len(sortedItems) > maxItems {
		return sortedItems[:maxItems]
	}
	return sortedItems
}

// LimitWebResults limits the number of web results
func LimitWebResults(webResults []string, maxResults int) []string {
	if len(webResults) <= maxResults {
		return webResults
	}
	return webResults[:maxResults]
}

// buildSystemPrompt creates system prompt based on query type
func buildSystemPrompt(queryType QueryType) string {
	basePrompt := `You are an expert Cloud Solutions Architect assistant. Your role is to help Solutions Architects with pre-sales research and planning.

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

`

	diagramInstructions := buildDiagramInstructions(queryType)

	switch queryType {
	case TechnicalQuery:
		return basePrompt + diagramInstructions + `TECHNICAL FOCUS: Emphasize technical implementation details, code examples, architectural patterns, and best practices. Provide specific configuration examples and troubleshooting guidance.

`
	case BusinessQuery:
		return basePrompt + diagramInstructions + `BUSINESS FOCUS: Emphasize business value, cost considerations, ROI analysis, timeline estimates, and strategic implications. Include risk assessments and compliance considerations.

`
	default:
		return basePrompt + diagramInstructions
	}
}

// buildDiagramInstructions creates comprehensive Mermaid.js diagram generation instructions
func buildDiagramInstructions(queryType QueryType) string {
	return `
## MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS

### When to Generate Diagrams
Generate architecture diagrams for queries involving:
- Cloud architecture design (AWS, Azure, GCP, hybrid)
- Migration planning and lift-and-shift scenarios
- Disaster recovery and backup strategies
- Network topology and security configurations
- Microservices and containerization architectures
- CI/CD pipeline designs
- Data flow and integration patterns

### Mermaid Syntax Requirements
- ALWAYS use "graph TD" (Top-Down) syntax for cloud architecture diagrams
- Enclose ALL diagram code in triple backticks with "mermaid" language identifier: ` + "```mermaid" + `
- Use descriptive node names with proper formatting
- Include subgraphs for logical groupings (environments, regions, services)
- Use appropriate arrow styles for different connection types

### Cloud Architecture Diagram Conventions

#### AWS Architecture Diagrams
- Use subgraphs for VPCs, Availability Zones, and service groupings
- Node naming: Use AWS service names (EC2, RDS, S3, Lambda, etc.)
- Include security groups, subnets, and load balancers
- Show data flow with labeled arrows

Example AWS Pattern:
` + "```" + `
graph TD
    subgraph "AWS Cloud"
        subgraph "VPC: 10.0.0.0/16"
            subgraph "Public Subnet"
                ALB[Application Load Balancer]
                NAT[NAT Gateway]
            end
            subgraph "Private Subnet"
                EC2[EC2 Instances]
                RDS[RDS Database]
            end
        end
        S3[S3 Buckets]
    end
    Users[Users] --> ALB
    ALB --> EC2
    EC2 --> RDS
    EC2 --> S3
` + "```" + `

#### Azure Architecture Diagrams
- Use subgraphs for Resource Groups, Virtual Networks, and subscriptions
- Node naming: Use Azure service names (VM, SQL Database, Storage Account, etc.)
- Include Azure-specific components (Application Gateway, Traffic Manager)
- Show resource relationships and dependencies

Example Azure Pattern:
` + "```" + `
graph TD
    subgraph "Azure Subscription"
        subgraph "Resource Group: Production"
            subgraph "Virtual Network"
                AG[Application Gateway]
                VM[Virtual Machines]
                SQL[SQL Database]
            end
            SA[Storage Account]
        end
    end
    Internet[Internet] --> AG
    AG --> VM
    VM --> SQL
    VM --> SA
` + "```" + `

#### Hybrid Cloud Architecture Diagrams
- Clearly separate on-premises and cloud environments
- Show connection methods (VPN, ExpressRoute, Direct Connect)
- Include hybrid services and replication patterns
- Demonstrate data synchronization flows

Example Hybrid Pattern:
` + "```" + `
graph TD
    subgraph "On-Premises"
        DC[Data Center]
        AD[Active Directory]
        APP[Applications]
    end
    subgraph "AWS Cloud"
        VPC[VPC]
        EC2[EC2 Instances]
        RDS[RDS Database]
    end
    DC -.->|VPN Connection| VPC
    AD -.->|AD Connector| VPC
    APP --> EC2
    EC2 --> RDS
` + "```" + `

### Diagram Quality Requirements
- Include 5-15 nodes for optimal clarity
- Use meaningful node labels (not generic terms)
- Group related components in subgraphs
- Show clear data flow direction with arrows
- Include security boundaries and access controls
- Use consistent naming conventions throughout

### Node Formatting Guidelines
- Use PascalCase for service names: EC2, RDS, S3
- Use descriptive labels: "Web Servers" instead of "Servers"
- Include capacity or scale indicators when relevant
- Use square brackets for services: [EC2 Instances]
- Use parentheses for external entities: (Users)
- Use curly braces for databases: {RDS Database}

### Arrow Types and Meanings
- Solid arrows (-->) for primary data flow
- Dashed arrows (-.->)  for secondary or backup connections
- Thick arrows (==>) for high-bandwidth connections
- Dotted arrows (...>) for occasional or batch data transfer

### Fallback Instructions
If the query is NOT about architecture, infrastructure, or technical implementation:
- Do NOT generate a diagram
- Focus on textual response with bullet points and structured information
- Only include diagrams if they genuinely add value to the architectural understanding

### Common Diagram Mistakes to Avoid
- Do NOT use "graph LR" (Left-Right) - always use "graph TD" (Top-Down)
- Do NOT create overcomplicated diagrams with too many nodes
- Do NOT use generic node names like "Server1", "Database1"
- Do NOT forget to enclose diagram code in proper markdown code blocks
- Do NOT include diagrams for non-architectural queries

### Code Block Format
Always format Mermaid diagrams exactly like this:

` + "```mermaid" + `
graph TD
    [Your diagram content here]
` + "```" + `

`
}

// EstimateTokens provides a rough estimate of token count (4 characters ≈ 1 token)
func EstimateTokens(text string) int {
	return utf8.RuneCountInString(text) / 4
}

// TruncateToTokenLimit truncates text to fit within token limit
func TruncateToTokenLimit(text string, maxTokens int) string {
	estimatedTokens := EstimateTokens(text)
	if estimatedTokens <= maxTokens {
		return text
	}

	// Calculate target character count (rough approximation)
	// Use 90% of the target to account for truncation notice
	targetChars := int(float64(maxTokens) * 4 * 0.9)
	runes := []rune(text)

	if len(runes) > targetChars {
		return string(runes[:targetChars]) + "...\n\n[Context truncated due to length limits]"
	}

	return text
}

// ValidatePrompt validates the completeness and structure of a prompt
func ValidatePrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	if len(prompt) < 100 {
		return fmt.Errorf("prompt appears to be too short (< 100 characters)")
	}

	if !strings.Contains(prompt, "User Query:") {
		return fmt.Errorf("prompt must contain user query section")
	}

	if !strings.Contains(prompt, "Solutions Architect") {
		return fmt.Errorf("prompt must contain Solutions Architect persona")
	}

	if !strings.Contains(prompt, "[source_id]") {
		return fmt.Errorf("prompt must contain citation instructions")
	}

	if !strings.Contains(prompt, "MERMAID.JS DIAGRAM GENERATION INSTRUCTIONS") {
		return fmt.Errorf("prompt must contain Mermaid.js diagram generation instructions")
	}

	if !strings.Contains(prompt, "graph TD") {
		return fmt.Errorf("prompt must contain graph TD syntax instructions")
	}

	if !strings.Contains(prompt, "```mermaid") {
		return fmt.Errorf("prompt must contain mermaid code block formatting instructions")
	}

	return nil
}

// DetectArchitectureQuery determines if a query is about architecture and warrants a diagram
func DetectArchitectureQuery(query string) bool {
	queryLower := strings.ToLower(query)

	architectureKeywords := []string{
		// Core architecture terms
		"architecture", "design", "topology", "infrastructure", "deployment",
		"migration", "lift-and-shift", "lift and shift", "disaster recovery",
		"backup", "replication", "failover", "high availability", "scalability",

		// Cloud platforms
		"aws", "azure", "gcp", "google cloud", "cloud", "hybrid", "multi-cloud",
		"on-premises", "on-prem", "datacenter", "data center",

		// Networking
		"network", "vpc", "subnet", "security group", "firewall", "load balancer",
		"vpn", "expressroute", "direct connect", "peering", "gateway",

		// Services and components
		"microservices", "containers", "kubernetes", "docker", "serverless",
		"lambda", "functions", "api", "database", "storage", "cdn",

		// Integration patterns
		"integration", "data flow", "pipeline", "workflow", "orchestration",
		"messaging", "queue", "event", "streaming", "batch processing",

		// Specific implementations
		"terraform", "cloudformation", "ansible", "ci/cd", "devops",
		"monitoring", "logging", "observability", "security", "compliance",
	}

	for _, keyword := range architectureKeywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}

	return false
}

// IsBusinessOnlyQuery determines if a query is purely business-focused without technical architecture
func IsBusinessOnlyQuery(query string) bool {
	queryLower := strings.ToLower(query)

	businessOnlyKeywords := []string{
		"cost", "pricing", "budget", "savings", "roi", "return on investment",
		"business case", "financial", "billing", "invoice", "payment",
		"timeline", "schedule", "project plan", "roadmap", "strategy",
		"compliance", "governance", "policy", "regulation", "audit",
		"risk", "security assessment", "vulnerability", "threat",
		"sla", "service level", "kpi", "metrics", "performance",
		"training", "certification", "documentation", "process",
		"team", "resources", "staffing", "skills", "expertise",
	}

	// Check if it's primarily business-focused
	businessScore := 0
	for _, keyword := range businessOnlyKeywords {
		if strings.Contains(queryLower, keyword) {
			businessScore++
		}
	}

	// If it has business keywords but no architecture keywords, it's business-only
	return businessScore > 0 && !DetectArchitectureQuery(query)
}
