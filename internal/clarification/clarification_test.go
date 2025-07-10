package clarification

import (
	"context"
	"strings"
	"testing"

	"github.com/your-org/ai-sa-assistant/internal/session"
	"github.com/your-org/ai-sa-assistant/internal/synth"
)

func TestAnalyzer_AnalyzeQuery(t *testing.T) {
	analyzer := NewAnalyzer()

	tests := []struct {
		name     string
		query    string
		expected bool // whether clarification is required
	}{
		{
			name:     "Clear specific query",
			query:    "Help me migrate 50 Windows VMs from VMware to AWS with 6-month timeline for production workloads",
			expected: false,
		},
		{
			name:     "Ambiguous generic query",
			query:    "Help me migrate to the cloud",
			expected: true,
		},
		{
			name:     "Incomplete security query",
			query:    "I need a security plan",
			expected: true,
		},
		{
			name:     "Vague architecture request",
			query:    "Design a good architecture",
			expected: true,
		},
		{
			name:     "Specific architecture request",
			query:    "Design a scalable web application architecture for 100,000 users on AWS with high availability and HIPAA compliance",
			expected: false,
		},
		{
			name:     "Too short query",
			query:    "AWS help",
			expected: true,
		},
		{
			name:     "General help request",
			query:    "Can you help me?",
			expected: true,
		},
		{
			name:     "Very short query",
			query:    "Help",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := analyzer.AnalyzeQuery(context.Background(), tt.query, []session.Message{})
			if err != nil {
				t.Fatalf("AnalyzeQuery() error = %v", err)
			}

			if analysis.RequiresClarification != tt.expected {
				t.Errorf("AnalyzeQuery() RequiresClarification = %v, expected %v",
					analysis.RequiresClarification, tt.expected)
			}

			// Additional checks for queries that should require clarification
			if tt.expected {
				if !analysis.IsAmbiguous && !analysis.IsIncomplete {
					t.Errorf("Query requiring clarification should be either ambiguous or incomplete")
				}
				if len(analysis.ClarificationNeeded) == 0 {
					t.Errorf("Query requiring clarification should have clarification areas")
				}
			}
		})
	}
}

func TestAnalyzer_DetectIntents(t *testing.T) {
	analyzer := NewAnalyzer()

	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "Migration query",
			query:    "migrate VMs from VMware to AWS",
			expected: []string{"migration"},
		},
		{
			name:     "Security query",
			query:    "security assessment for HIPAA compliance",
			expected: []string{"security"},
		},
		{
			name:     "Architecture query",
			query:    "design microservices architecture",
			expected: []string{"architecture"},
		},
		{
			name:     "Multiple intents",
			query:    "migrate and secure our applications to AWS",
			expected: []string{"migration", "security"},
		},
		{
			name:     "Disaster recovery query",
			query:    "backup and disaster recovery plan",
			expected: []string{"disaster_recovery", "storage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intents := analyzer.detectIntents(tt.query)

			for _, expectedIntent := range tt.expected {
				found := false
				for _, intent := range intents {
					if intent == expectedIntent {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected intent %s not found in %v", expectedIntent, intents)
				}
			}
		})
	}
}

func TestAnalyzer_AmbiguityScore(t *testing.T) {
	analyzer := NewAnalyzer()

	tests := []struct {
		name     string
		query    string
		minScore float64
		maxScore float64
	}{
		{
			name:     "Very specific query",
			query:    "Migrate 50 Windows Server 2019 VMs from VMware vSphere 7.0 to AWS EC2 with t3.large instances",
			minScore: 0.0,
			maxScore: 0.3,
		},
		{
			name:     "Very ambiguous query",
			query:    "Help me with the best cloud solution",
			minScore: 0.7,
			maxScore: 1.0,
		},
		{
			name:     "Moderately ambiguous",
			query:    "Design a good architecture for our application",
			minScore: 0.3,
			maxScore: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := analyzer.calculateAmbiguityScore(tt.query)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("AmbiguityScore = %f, expected between %f and %f",
					score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAnalyzer_CompletenessScore(t *testing.T) {
	analyzer := NewAnalyzer()

	tests := []struct {
		name     string
		query    string
		minScore float64
		maxScore float64
	}{
		{
			name:     "Very complete query",
			query:    "Design a HIPAA-compliant web application architecture for 100,000 concurrent users on AWS with high availability and disaster recovery",
			minScore: 0.7,
			maxScore: 1.0,
		},
		{
			name:     "Very incomplete query",
			query:    "Help",
			minScore: 0.0,
			maxScore: 0.3,
		},
		{
			name:     "Moderately complete",
			query:    "Migrate applications to AWS cloud",
			minScore: 0.3,
			maxScore: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := analyzer.calculateCompletenessScore(tt.query)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CompletenessScore = %f, expected between %f and %f",
					score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAnalyzer_GenerateClarificationRequest(t *testing.T) {
	analyzer := NewAnalyzer()

	// Test with an ambiguous query
	query := "Help me migrate to the cloud"
	analysis, err := analyzer.AnalyzeQuery(context.Background(), query, []session.Message{})
	if err != nil {
		t.Fatalf("AnalyzeQuery() error = %v", err)
	}

	clarificationReq, err := analyzer.GenerateClarificationRequest(context.Background(), analysis)
	if err != nil {
		t.Fatalf("GenerateClarificationRequest() error = %v", err)
	}

	// Verify clarification request structure
	if clarificationReq.OriginalQuery != query {
		t.Errorf("OriginalQuery = %s, expected %s", clarificationReq.OriginalQuery, query)
	}

	if len(clarificationReq.Questions) == 0 {
		t.Error("Expected at least one clarification question")
	}

	if len(clarificationReq.Suggestions) == 0 {
		t.Error("Expected at least one suggestion")
	}

	// Verify question structure
	for i, question := range clarificationReq.Questions {
		if question.ID == "" {
			t.Errorf("Question %d missing ID", i)
		}
		if question.Question == "" {
			t.Errorf("Question %d missing question text", i)
		}
		if question.Category == "" {
			t.Errorf("Question %d missing category", i)
		}
	}
}

func TestFollowupDetection(t *testing.T) {
	analyzer := NewAnalyzer()

	tests := []struct {
		name        string
		query       string
		expectType  string
		hasFollowup bool
	}{
		{
			name:        "Reference to previous content",
			query:       "Can you make that diagram more detailed?",
			expectType:  "reference",
			hasFollowup: true,
		},
		{
			name:        "Request for more information",
			query:       "Can you provide more details about the security aspects?",
			expectType:  "expansion",
			hasFollowup: true,
		},
		{
			name:        "Alternative request",
			query:       "What about a different approach using containers?",
			expectType:  "alternative",
			hasFollowup: true,
		},
		{
			name:        "Cost inquiry",
			query:       "What would be the cost implications?",
			expectType:  "cost_inquiry",
			hasFollowup: true,
		},
		{
			name:        "Regular new query",
			query:       "Help me design a new architecture",
			expectType:  "general_followup",
			hasFollowup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock conversation history
			history := []session.Message{
				{
					Role:    session.AssistantRole,
					Content: "Here's a VPC architecture diagram with security groups...",
				},
			}

			analysis, err := analyzer.AnalyzeQuery(context.Background(), tt.query, history)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error = %v", err)
			}

			hasFollowup := analysis.FollowupContext != nil
			if hasFollowup != tt.hasFollowup {
				t.Errorf("Expected hasFollowup = %v, got %v", tt.hasFollowup, hasFollowup)
			}

			if hasFollowup {
				followupType := synth.DetectFollowupType(tt.query)
				if followupType != tt.expectType {
					t.Errorf("Expected followup type %s, got %s", tt.expectType, followupType)
				}
			}
		})
	}
}

func TestHasSpecificContext(t *testing.T) {
	// Test helper function to check for specific context
	hasSpecificContext := func(query string) bool {
		queryLower := strings.ToLower(query)
		specificTerms := []string{
			"aws", "azure", "gcp", "google cloud",
			"vmware", "hyper-v", "kubernetes", "docker", "terraform",
			"hipaa", "gdpr", "sox", "pci", "iso 27001",
			"production", "staging", "development", "gb", "tb", "users",
			"million", "thousand", "hours", "minutes", "days", "months",
		}

		for _, term := range specificTerms {
			if strings.Contains(queryLower, term) {
				return true
			}
		}
		return false
	}

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "Has cloud provider context",
			query:    "migrate to aws cloud",
			expected: true,
		},
		{
			name:     "Has compliance context",
			query:    "hipaa compliant solution",
			expected: true,
		},
		{
			name:     "Has scale context",
			query:    "support 100000 users",
			expected: true,
		},
		{
			name:     "No specific context",
			query:    "good cloud solution",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSpecificContext(tt.query)
			if result != tt.expected {
				t.Errorf("hasSpecificContext() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkAnalyzeQuery(b *testing.B) {
	analyzer := NewAnalyzer()
	query := "Help me migrate my applications to AWS with security compliance"

	for i := 0; i < b.N; i++ {
		_, err := analyzer.AnalyzeQuery(context.Background(), query, []session.Message{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDetectIntents(b *testing.B) {
	analyzer := NewAnalyzer()
	query := "migrate and secure our web applications to AWS cloud with disaster recovery"

	for i := 0; i < b.N; i++ {
		analyzer.detectIntents(query)
	}
}

// Helper function to create test analyzer (used in integration tests)
func CreateTestAnalyzer() *Analyzer {
	return NewAnalyzer()
}
