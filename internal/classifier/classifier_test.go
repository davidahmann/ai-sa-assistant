package classifier

import (
	"testing"
)

func TestNewQueryClassifier(t *testing.T) {
	classifier := NewQueryClassifier()

	if classifier == nil {
		t.Fatal("NewQueryClassifier returned nil")
	}

	if len(classifier.cloudKeywords) == 0 {
		t.Error("Expected cloud keywords to be populated")
	}

	if len(classifier.cloudProviders) == 0 {
		t.Error("Expected cloud providers to be populated")
	}

	if len(classifier.cloudServices) == 0 {
		t.Error("Expected cloud services to be populated")
	}

	if len(classifier.rejectedTopics) == 0 {
		t.Error("Expected rejected topics to be populated")
	}
}

func TestClassifyQuery_CloudRelatedQueries(t *testing.T) {
	classifier := NewQueryClassifier()

	testCases := []struct {
		name             string
		query            string
		expectedCloud    bool
		expectedCategory string
	}{
		{
			name:             "AWS migration query",
			query:            "AWS migration plan for 100 VMs",
			expectedCloud:    true,
			expectedCategory: "aws",
		},
		{
			name:             "Azure security query",
			query:            "Azure security compliance requirements",
			expectedCloud:    true,
			expectedCategory: "azure",
		},
		{
			name:             "Disaster recovery query",
			query:            "Disaster recovery architecture",
			expectedCloud:    true,
			expectedCategory: "disaster-recovery",
		},
		{
			name:             "GCP networking query",
			query:            "GCP VPC subnet configuration",
			expectedCloud:    true,
			expectedCategory: "gcp",
		},
		{
			name:             "Hybrid architecture query",
			query:            "Hybrid cloud architecture with on-premises",
			expectedCloud:    true,
			expectedCategory: "hybrid",
		},
		{
			name:             "Cloud storage query",
			query:            "S3 bucket configuration for backup",
			expectedCloud:    true,
			expectedCategory: "aws",
		},
		{
			name:             "Kubernetes query",
			query:            "EKS cluster setup with load balancer",
			expectedCloud:    true,
			expectedCategory: "aws",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifier.ClassifyQuery(tc.query)

			if result.IsCloudRelated != tc.expectedCloud {
				t.Errorf("Expected IsCloudRelated=%v, got %v", tc.expectedCloud, result.IsCloudRelated)
			}

			if result.Category != tc.expectedCategory {
				t.Errorf("Expected category=%s, got %s", tc.expectedCategory, result.Category)
			}

			if result.Confidence < 0.3 {
				t.Errorf("Expected confidence > 0.3 for cloud query, got %f", result.Confidence)
			}
		})
	}
}

func TestClassifyQuery_NonCloudQueries(t *testing.T) {
	classifier := NewQueryClassifier()

	testCases := []struct {
		name           string
		query          string
		expectedCloud  bool
		expectedReason string
	}{
		{
			name:           "Weather query",
			query:          "What's the weather today?",
			expectedCloud:  false,
			expectedReason: "Query contains non-cloud topic",
		},
		{
			name:           "Recipe query",
			query:          "Recipe for chocolate cake",
			expectedCloud:  false,
			expectedReason: "Query contains non-cloud topic",
		},
		{
			name:           "Political query",
			query:          "Political opinions on trade policies",
			expectedCloud:  false,
			expectedReason: "Query contains non-cloud topic",
		},
		{
			name:           "Sports query",
			query:          "Latest football scores",
			expectedCloud:  false,
			expectedReason: "Query contains non-cloud topic",
		},
		{
			name:           "General programming query",
			query:          "How to learn python programming",
			expectedCloud:  false,
			expectedReason: "Query appears to be general/educational rather than cloud-specific",
		},
		{
			name:           "Personal advice query",
			query:          "What do you think about my career?",
			expectedCloud:  false,
			expectedReason: "Query appears to be general/educational rather than cloud-specific",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifier.ClassifyQuery(tc.query)

			if result.IsCloudRelated != tc.expectedCloud {
				t.Errorf("Expected IsCloudRelated=%v, got %v", tc.expectedCloud, result.IsCloudRelated)
			}

			if result.RejectionReason == "" {
				t.Error("Expected rejection reason to be set for non-cloud query")
			}

			if result.Confidence < 0.7 {
				t.Errorf("Expected high confidence for obvious non-cloud query, got %f", result.Confidence)
			}
		})
	}
}

func TestClassifyQuery_EdgeCases(t *testing.T) {
	classifier := NewQueryClassifier()

	testCases := []struct {
		name          string
		query         string
		expectedCloud bool
	}{
		{
			name:          "Empty query",
			query:         "",
			expectedCloud: false,
		},
		{
			name:          "Whitespace only",
			query:         "   ",
			expectedCloud: false,
		},
		{
			name:          "Single word cloud term",
			query:         "AWS",
			expectedCloud: true,
		},
		{
			name:          "Mixed case query",
			query:         "AWS Migration PLAN",
			expectedCloud: true,
		},
		{
			name:          "Ambiguous query",
			query:         "network troubleshooting",
			expectedCloud: true, // Should err on side of caution
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifier.ClassifyQuery(tc.query)

			if result.IsCloudRelated != tc.expectedCloud {
				t.Errorf("Expected IsCloudRelated=%v, got %v", tc.expectedCloud, result.IsCloudRelated)
			}
		})
	}
}

func TestCalculateCloudScore(t *testing.T) {
	classifier := NewQueryClassifier()

	testCases := []struct {
		name          string
		query         string
		expectedScore float64
	}{
		{
			name:          "High cloud score",
			query:         "aws ec2 migration with vpc",
			expectedScore: 0.7,
		},
		{
			name:          "Zero cloud score",
			query:         "chocolate cake recipe",
			expectedScore: 0.0,
		},
		{
			name:          "Medium cloud score",
			query:         "cloud storage options",
			expectedScore: 0.3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			score := classifier.calculateCloudScore(tc.query)

			if tc.expectedScore == 0.0 && score != 0.0 {
				t.Errorf("Expected score=0.0, got %f", score)
			} else if tc.expectedScore > 0.0 && score < tc.expectedScore {
				t.Errorf("Expected score>=%f, got %f", tc.expectedScore, score)
			}
		})
	}
}

func TestDetermineCloudCategory(t *testing.T) {
	classifier := NewQueryClassifier()

	testCases := []struct {
		name             string
		query            string
		expectedCategory string
	}{
		{
			name:             "AWS provider",
			query:            "aws ec2 instances",
			expectedCategory: "aws",
		},
		{
			name:             "Azure provider",
			query:            "azure virtual machines",
			expectedCategory: "azure",
		},
		{
			name:             "GCP provider",
			query:            "gcp compute engine",
			expectedCategory: "gcp",
		},
		{
			name:             "Migration category",
			query:            "lift and shift migration",
			expectedCategory: "migration",
		},
		{
			name:             "Security category",
			query:            "security compliance requirements",
			expectedCategory: "security",
		},
		{
			name:             "Networking category",
			query:            "vpc subnet configuration",
			expectedCategory: "networking",
		},
		{
			name:             "General cloud",
			query:            "cloud architecture best practices",
			expectedCategory: "general-cloud",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			category := classifier.determineCloudCategory(tc.query)

			if category != tc.expectedCategory {
				t.Errorf("Expected category=%s, got %s", tc.expectedCategory, category)
			}
		})
	}
}

func TestGetRejectionMessage(t *testing.T) {
	classifier := NewQueryClassifier()

	result := ClassificationResult{
		IsCloudRelated:  false,
		Category:        "rejected",
		Confidence:      0.95,
		RejectionReason: "Query contains non-cloud topic",
	}

	message := classifier.GetRejectionMessage(result)

	if message == "" {
		t.Error("Expected rejection message to be non-empty")
	}

	expectedMessage := "I'm specialized in cloud architecture and solutions. " +
		"Please ask about AWS, Azure, GCP, migrations, security, compliance, or infrastructure topics."
	if message != expectedMessage {
		t.Errorf("Expected message=%s, got %s", expectedMessage, message)
	}
}

func TestClassificationAccuracy(t *testing.T) {
	classifier := NewQueryClassifier()

	// Test classification accuracy for obvious cases
	cloudQueries := []string{
		"AWS migration plan for 100 VMs",
		"Azure security compliance requirements",
		"GCP kubernetes cluster setup",
		"Disaster recovery architecture",
		"Hybrid cloud with on-premises",
		"S3 bucket configuration",
		"VPC subnet design",
		"EC2 instance sizing",
		"Azure AD integration",
		"Cloud storage backup strategy",
	}

	nonCloudQueries := []string{
		"What's the weather today?",
		"Recipe for chocolate cake",
		"Political opinions on trade policies",
		"Latest football scores",
		"How to learn python programming",
		"What do you think about my career?",
		"Movie recommendations",
		"Travel destinations",
		"Health advice",
		"Stock market predictions",
	}

	// Test cloud queries
	cloudCorrect := 0
	for _, query := range cloudQueries {
		result := classifier.ClassifyQuery(query)
		if result.IsCloudRelated {
			cloudCorrect++
		}
	}

	cloudAccuracy := float64(cloudCorrect) / float64(len(cloudQueries))
	if cloudAccuracy < 0.95 {
		t.Errorf("Cloud query accuracy below 95%%, got %f", cloudAccuracy)
	}

	// Test non-cloud queries
	nonCloudCorrect := 0
	for _, query := range nonCloudQueries {
		result := classifier.ClassifyQuery(query)
		if !result.IsCloudRelated {
			nonCloudCorrect++
		}
	}

	nonCloudAccuracy := float64(nonCloudCorrect) / float64(len(nonCloudQueries))
	if nonCloudAccuracy < 0.95 {
		t.Errorf("Non-cloud query accuracy below 95%%, got %f", nonCloudAccuracy)
	}
}

func BenchmarkClassifyQuery(b *testing.B) {
	classifier := NewQueryClassifier()
	query := "AWS migration plan for 100 VMs with EC2 and RDS"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.ClassifyQuery(query)
	}
}
