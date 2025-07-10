// Package classifier provides query classification for cloud-related topics.
package classifier

import (
	"strings"
)

// Classification confidence thresholds and scoring weights
const (
	HighConfidenceThreshold   = 0.95
	MediumConfidenceThreshold = 0.8
	LowConfidenceThreshold    = 0.2
	HighCloudScoreThreshold   = 0.5
	CloudProviderWeight       = 0.4
	CloudServiceWeight        = 0.3
	CloudKeywordWeight        = 0.5
)

// ClassificationResult represents the result of query classification
type ClassificationResult struct {
	IsCloudRelated  bool    `json:"is_cloud_related"`
	Category        string  `json:"category"`
	Confidence      float64 `json:"confidence"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
}

// QueryClassifier handles topic classification for cloud-related queries
type QueryClassifier struct {
	cloudKeywords    []string
	cloudProviders   []string
	cloudServices    []string
	rejectedTopics   []string
	rejectedKeywords []string
}

// NewQueryClassifier creates a new instance of QueryClassifier
func NewQueryClassifier() *QueryClassifier {
	return &QueryClassifier{
		cloudKeywords: []string{
			"cloud", "migration", "hybrid", "architecture", "infrastructure", "deployment",
			"scaling", "load balancing", "network", "security", "compliance", "disaster recovery",
			"backup", "storage", "database", "compute", "container", "kubernetes", "serverless",
			"api gateway", "microservices", "devops", "cicd", "monitoring", "logging",
			"vpc", "subnet", "firewall", "encryption", "authentication", "authorization",
			"lift and shift", "replatform", "refactor", "modernization", "optimization",
			"cost management", "governance", "automation", "orchestration", "provisioning",
		},
		cloudProviders: []string{
			"aws", "amazon web services", "azure", "microsoft azure", "gcp", "google cloud",
			"google cloud platform", "oracle cloud", "ibm cloud", "alibaba cloud",
			"digitalocean", "linode", "vultr", "rackspace", "vmware", "openstack",
		},
		cloudServices: []string{
			"ec2", "s3", "rds", "lambda", "ecs", "eks", "route53", "cloudfront", "elb",
			"auto scaling", "cloudwatch", "iam", "cognito", "dynamodb", "redshift",
			"virtual machine", "blob storage", "app service", "functions", "aks",
			"cosmos db", "sql database", "active directory", "key vault", "application gateway",
			"compute engine", "cloud storage", "cloud sql", "cloud functions", "gke",
			"bigquery", "cloud run", "cloud build", "cloud armor", "cloud cdn",
			"mgn", "application migration service", "hcx", "site recovery", "backup",
			"expressroute", "direct connect", "vpn gateway", "private link", "service mesh",
		},
		rejectedTopics: []string{
			"politics", "political", "government", "election", "voting", "democrat", "republican",
			"weather", "temperature", "climate", "forecast", "rain", "snow", "storm",
			"recipe", "cooking", "food", "restaurant", "meal", "diet", "nutrition",
			"sports", "football", "basketball", "baseball", "soccer", "tennis", "golf",
			"entertainment", "movie", "tv show", "celebrity", "music", "concert", "album",
			"personal", "relationship", "dating", "marriage", "family", "health", "medical",
			"finance", "stock", "investment", "crypto", "bitcoin", "trading", "mortgage",
			"travel", "vacation", "hotel", "flight", "tourism", "destination", "visa",
			"education", "school", "university", "degree", "homework", "assignment", "exam",
			"legal", "lawyer", "court", "lawsuit", "contract", "patent", "copyright",
			"general programming", "python tutorial", "javascript basics", "html css",
			"math", "mathematics", "algebra", "calculus", "geometry", "statistics",
			"science", "physics", "chemistry", "biology", "astronomy", "geology",
		},
		rejectedKeywords: []string{
			"what is", "how to", "tell me about", "explain", "define", "meaning of",
			"tutorial", "guide", "learn", "study", "homework", "assignment", "project",
			"personal opinion", "what do you think", "your thoughts", "advice",
			"recommend", "suggest", "best", "favorite", "like", "dislike", "prefer",
		},
	}
}

// ClassifyQuery analyzes a query and determines if it's cloud-related
func (qc *QueryClassifier) ClassifyQuery(query string) ClassificationResult {
	query = strings.ToLower(strings.TrimSpace(query))

	if query == "" {
		return ClassificationResult{
			IsCloudRelated:  false,
			Category:        "empty",
			Confidence:      1.0,
			RejectionReason: "Empty query provided",
		}
	}

	// Check for explicitly rejected topics first
	for _, rejectedTopic := range qc.rejectedTopics {
		if strings.Contains(query, rejectedTopic) {
			return ClassificationResult{
				IsCloudRelated:  false,
				Category:        "rejected",
				Confidence:      HighConfidenceThreshold,
				RejectionReason: "Query contains non-cloud topic",
			}
		}
	}

	// Check for rejected keywords that indicate non-cloud intent
	rejectedKeywordMatches := 0
	for _, rejectedKeyword := range qc.rejectedKeywords {
		if strings.Contains(query, rejectedKeyword) {
			rejectedKeywordMatches++
		}
	}

	// Calculate cloud-related score
	cloudScore := qc.calculateCloudScore(query)

	// If high rejected keyword matches and low cloud score, reject
	if rejectedKeywordMatches >= 2 && cloudScore < LowConfidenceThreshold {
		return ClassificationResult{
			IsCloudRelated:  false,
			Category:        "general",
			Confidence:      MediumConfidenceThreshold,
			RejectionReason: "Query appears to be general/educational rather than cloud-specific",
		}
	}

	// Determine category and confidence based on cloud score
	if cloudScore >= HighCloudScoreThreshold {
		category := qc.determineCloudCategory(query)
		return ClassificationResult{
			IsCloudRelated: true,
			Category:       category,
			Confidence:     cloudScore,
		}
	} else if cloudScore >= LowConfidenceThreshold {
		// Ambiguous queries - err on the side of caution for cloud-related
		category := qc.determineCloudCategory(query)
		return ClassificationResult{
			IsCloudRelated: true,
			Category:       category,
			Confidence:     cloudScore,
		}
	}

	return ClassificationResult{
		IsCloudRelated:  false,
		Category:        "non-cloud",
		Confidence:      1.0 - cloudScore,
		RejectionReason: "Query does not appear to be cloud-related",
	}
}

// calculateCloudScore calculates a score from 0-1 indicating how cloud-related a query is
func (qc *QueryClassifier) calculateCloudScore(query string) float64 {
	score := 0.0
	words := strings.Fields(query)
	totalWords := len(words)

	if totalWords == 0 {
		return 0.0
	}

	// Cloud provider matches (high weight)
	for _, provider := range qc.cloudProviders {
		if strings.Contains(query, provider) {
			score += CloudProviderWeight
		}
	}

	// Cloud service matches (high weight)
	for _, service := range qc.cloudServices {
		if strings.Contains(query, service) {
			score += CloudServiceWeight
		}
	}

	// General cloud keyword matches (medium weight)
	keywordMatches := 0
	for _, keyword := range qc.cloudKeywords {
		if strings.Contains(query, keyword) {
			keywordMatches++
		}
	}

	// Normalize keyword matches
	if keywordMatches > 0 {
		keywordScore := float64(keywordMatches) / float64(totalWords)
		score += keywordScore * CloudKeywordWeight
	}

	// Cap the score at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// determineCloudCategory determines the specific cloud category for a query
func (qc *QueryClassifier) determineCloudCategory(query string) string {
	// Check for AWS-specific services and terms
	awsTerms := []string{"aws", "amazon", "s3", "ec2", "rds", "lambda", "eks", "cloudfront", "route53", "iam", "dynamo"}
	for _, term := range awsTerms {
		if strings.Contains(query, term) {
			return "aws"
		}
	}

	// Check for Azure-specific services and terms
	azureTerms := []string{"azure", "microsoft", "blob storage", "app service", "aks", "cosmos", "active directory"}
	for _, term := range azureTerms {
		if strings.Contains(query, term) {
			return "azure"
		}
	}

	// Check for GCP-specific services and terms
	gcpTerms := []string{"gcp", "google cloud", "compute engine", "cloud storage", "gke", "bigquery", "cloud run"}
	for _, term := range gcpTerms {
		if strings.Contains(query, term) {
			return "gcp"
		}
	}

	// Check for specific categories
	if strings.Contains(query, "migration") || strings.Contains(query, "lift") {
		return "migration"
	}
	if strings.Contains(query, "hybrid") {
		return "hybrid"
	}
	if strings.Contains(query, "disaster") || strings.Contains(query, "recovery") {
		return "disaster-recovery"
	}
	if strings.Contains(query, "security") || strings.Contains(query, "compliance") {
		return "security"
	}
	if strings.Contains(query, "network") || strings.Contains(query, "vpc") {
		return "networking"
	}
	if strings.Contains(query, "storage") || strings.Contains(query, "database") {
		return "storage"
	}
	if strings.Contains(query, "compute") || strings.Contains(query, "vm") {
		return "compute"
	}

	return "general-cloud"
}

// GetRejectionMessage returns a user-friendly rejection message
func (qc *QueryClassifier) GetRejectionMessage(_ ClassificationResult) string {
	return "I'm specialized in cloud architecture and solutions. " +
		"Please ask about AWS, Azure, GCP, migrations, security, compliance, or infrastructure topics."
}
