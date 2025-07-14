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
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/your-org/ai-sa-assistant/internal/chroma"
	"github.com/your-org/ai-sa-assistant/internal/openai"
)

const (
	TestChromaURL        = "http://localhost:8001"
	TestCollectionName   = "test_demo_collection"
	TestDocumentCount    = 10
	DefaultEmbeddingDim  = 1536
)

// TestDocument represents a test document to be seeded
type TestDocument struct {
	ID       string
	Content  string
	Metadata map[string]interface{}
}

func main() {
	log.Println("🌱 Starting test data seeding...")

	// Check if ChromaDB test instance is running
	if !isChromaDBReady() {
		log.Fatal("❌ ChromaDB test instance not ready. Please start it first with: make start-test-infra")
	}

	// Create ChromaDB client
	client, err := chroma.NewClient(TestChromaURL)
	if err != nil {
		log.Fatalf("❌ Failed to create ChromaDB client: %v", err)
	}

	// Create or get collection
	collection, err := ensureTestCollection(client)
	if err != nil {
		log.Fatalf("❌ Failed to create test collection: %v", err)
	}

	// Generate test documents
	documents := generateTestDocuments()
	log.Printf("📄 Generated %d test documents", len(documents))

	// Create embeddings (mock embeddings for testing)
	embeddings := generateMockEmbeddings(len(documents))
	log.Printf("🔗 Generated %d mock embeddings", len(embeddings))

	// Add documents to collection
	if err := addDocumentsToCollection(collection, documents, embeddings); err != nil {
		log.Fatalf("❌ Failed to add documents to collection: %v", err)
	}

	log.Println("✅ Test data seeding completed successfully!")
	log.Printf("📊 Collection '%s' now contains %d documents", TestCollectionName, len(documents))
	log.Printf("🔍 Test ChromaDB available at: %s", TestChromaURL)
}

func isChromaDBReady() bool {
	client, err := chroma.NewClient(TestChromaURL)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.IsHealthy(ctx)
}

func ensureTestCollection(client *chroma.Client) (*chroma.Collection, error) {
	ctx := context.Background()

	// Try to get existing collection
	collection, err := client.GetCollection(ctx, TestCollectionName)
	if err == nil {
		log.Printf("📁 Using existing collection: %s", TestCollectionName)
		return collection, nil
	}

	// Create new collection
	collection, err = client.CreateCollection(ctx, TestCollectionName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	log.Printf("📁 Created new collection: %s", TestCollectionName)
	return collection, nil
}

func generateTestDocuments() []TestDocument {
	documents := []TestDocument{
		{
			ID:      "aws-ec2-guide",
			Content: "AWS EC2 provides scalable computing capacity in the cloud. Launch instances with various configurations including t2.micro, t3.medium, and c5.large instance types. Configure security groups and VPCs for network isolation.",
			Metadata: map[string]interface{}{
				"scenario":      "migration",
				"cloud":         "aws",
				"service":       "ec2",
				"complexity":    "intermediate",
				"last_updated": "2024-01-15",
			},
		},
		{
			ID:      "azure-vm-deployment",
			Content: "Azure Virtual Machines offer flexible compute resources. Use ARM templates or Azure CLI to deploy VMs with Windows or Linux operating systems. Configure network security groups and load balancers for high availability.",
			Metadata: map[string]interface{}{
				"scenario":      "migration",
				"cloud":         "azure",
				"service":       "vm",
				"complexity":    "intermediate",
				"last_updated": "2024-01-20",
			},
		},
		{
			ID:      "aws-rds-setup",
			Content: "Amazon RDS provides managed relational databases. Choose from MySQL, PostgreSQL, Oracle, and SQL Server engines. Configure Multi-AZ deployments for high availability and automated backups.",
			Metadata: map[string]interface{}{
				"scenario":      "database",
				"cloud":         "aws",
				"service":       "rds",
				"complexity":    "advanced",
				"last_updated": "2024-01-10",
			},
		},
		{
			ID:      "security-best-practices",
			Content: "Cloud security best practices include: Enable MFA for all users, use IAM roles and policies, encrypt data at rest and in transit, regularly rotate access keys, and monitor with CloudTrail or Azure Activity Log.",
			Metadata: map[string]interface{}{
				"scenario":      "security",
				"cloud":         "multi",
				"service":       "iam",
				"complexity":    "advanced",
				"last_updated": "2024-01-25",
			},
		},
		{
			ID:      "hybrid-connectivity",
			Content: "Establish hybrid connectivity using VPN or dedicated connections. AWS Direct Connect and Azure ExpressRoute provide private network connections. Configure BGP routing and network ACLs for secure communication.",
			Metadata: map[string]interface{}{
				"scenario":      "hybrid",
				"cloud":         "multi",
				"service":       "networking",
				"complexity":    "advanced",
				"last_updated": "2024-01-18",
			},
		},
		{
			ID:      "disaster-recovery-plan",
			Content: "Implement disaster recovery with RTO of 2 hours and RPO of 15 minutes. Use cross-region replication, automated backups, and failover procedures. Test recovery processes regularly.",
			Metadata: map[string]interface{}{
				"scenario":      "disaster-recovery",
				"cloud":         "aws",
				"service":       "backup",
				"complexity":    "advanced",
				"last_updated": "2024-01-22",
			},
		},
		{
			ID:      "kubernetes-deployment",
			Content: "Deploy applications using Kubernetes on EKS or AKS. Configure pods, services, and ingress controllers. Use Helm charts for package management and implement monitoring with Prometheus.",
			Metadata: map[string]interface{}{
				"scenario":      "containerization",
				"cloud":         "multi",
				"service":       "kubernetes",
				"complexity":    "advanced",
				"last_updated": "2024-01-12",
			},
		},
		{
			ID:      "cost-optimization",
			Content: "Optimize cloud costs by right-sizing instances, using reserved instances, implementing auto-scaling, and monitoring with Cost Explorer. Set up billing alerts and use spot instances for non-critical workloads.",
			Metadata: map[string]interface{}{
				"scenario":      "cost-optimization",
				"cloud":         "aws",
				"service":       "billing",
				"complexity":    "intermediate",
				"last_updated": "2024-01-28",
			},
		},
		{
			ID:      "serverless-architecture",
			Content: "Build serverless applications using AWS Lambda or Azure Functions. Implement event-driven architectures with API Gateway, DynamoDB, and CloudWatch. Use Infrastructure as Code with Terraform or CloudFormation.",
			Metadata: map[string]interface{}{
				"scenario":      "modernization",
				"cloud":         "multi",
				"service":       "lambda",
				"complexity":    "intermediate",
				"last_updated": "2024-01-16",
			},
		},
		{
			ID:      "monitoring-logging",
			Content: "Implement comprehensive monitoring and logging with CloudWatch, Azure Monitor, or third-party tools. Set up dashboards, alerts, and log aggregation. Use distributed tracing for microservices.",
			Metadata: map[string]interface{}{
				"scenario":      "observability",
				"cloud":         "multi",
				"service":       "monitoring",
				"complexity":    "intermediate",
				"last_updated": "2024-01-14",
			},
		},
	}

	return documents
}

func generateMockEmbeddings(count int) [][]float32 {
	embeddings := make([][]float32, count)
	for i := 0; i < count; i++ {
		embedding := make([]float32, DefaultEmbeddingDim)
		// Generate simple mock embeddings with some variation
		for j := 0; j < DefaultEmbeddingDim; j++ {
			embedding[j] = float32(i+j) / 1000.0
		}
		embeddings[i] = embedding
	}
	return embeddings
}

func addDocumentsToCollection(collection *chroma.Collection, documents []TestDocument, embeddings [][]float32) error {
	ctx := context.Background()

	// Prepare data for ChromaDB
	ids := make([]string, len(documents))
	documents_text := make([]string, len(documents))
	metadatas := make([]map[string]interface{}, len(documents))

	for i, doc := range documents {
		ids[i] = doc.ID
		documents_text[i] = doc.Content
		metadatas[i] = doc.Metadata
	}

	// Add documents to collection
	if err := collection.Add(ctx, ids, embeddings, metadatas, documents_text); err != nil {
		return fmt.Errorf("failed to add documents to collection: %w", err)
	}

	return nil
}

// Helper function to check if OpenAI API key is available
func hasOpenAIKey() bool {
	return os.Getenv("OPENAI_API_KEY") != ""
}

// Generate real embeddings using OpenAI API (if available)
func generateRealEmbeddings(documents []TestDocument) ([][]float32, error) {
	if !hasOpenAIKey() {
		log.Println("⚠️  No OpenAI API key found, using mock embeddings")
		return generateMockEmbeddings(len(documents)), nil
	}

	log.Println("🔗 Generating real embeddings using OpenAI API...")

	client, err := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	embeddings := make([][]float32, len(documents))
	for i, doc := range documents {
		embedding, err := client.GenerateEmbedding(context.Background(), doc.Content)
		if err != nil {
			log.Printf("⚠️  Failed to generate embedding for document %s: %v", doc.ID, err)
			// Use mock embedding as fallback
			embedding = make([]float32, DefaultEmbeddingDim)
			for j := 0; j < DefaultEmbeddingDim; j++ {
				embedding[j] = float32(i+j) / 1000.0
			}
		}
		embeddings[i] = embedding
		log.Printf("✅ Generated embedding for document: %s", doc.ID)
	}

	return embeddings, nil
}

// printSummary prints a summary of the seeded data
func printSummary(documents []TestDocument) {
	log.Println("\n📊 Test Data Summary:")
	log.Println("====================")

	scenarioCount := make(map[string]int)
	cloudCount := make(map[string]int)

	for _, doc := range documents {
		if scenario, ok := doc.Metadata["scenario"].(string); ok {
			scenarioCount[scenario]++
		}
		if cloud, ok := doc.Metadata["cloud"].(string); ok {
			cloudCount[cloud]++
		}
	}

	log.Println("📋 Documents by scenario:")
	for scenario, count := range scenarioCount {
		log.Printf("  %s: %d", scenario, count)
	}

	log.Println("☁️  Documents by cloud:")
	for cloud, count := range cloudCount {
		log.Printf("  %s: %d", cloud, count)
	}

	log.Printf("\n🎯 Total documents: %d", len(documents))
	log.Printf("📍 Collection: %s", TestCollectionName)
	log.Printf("🔗 ChromaDB URL: %s", TestChromaURL)
}