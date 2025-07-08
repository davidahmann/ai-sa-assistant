Of course. This type of structured workflow document is invaluable for maintaining quality and consistency in a project.

Here is a similar file, tailored specifically for your AI SA Assistant project, following the structure and level of detail from your example.

Fix Issue #$ARGUMENTS — Follow the Streamlined AI SA Assistant Workflow
A. Read This Section First for Project Context Only

The AI SA Assistant is an internal, demo-level solution designed to showcase the power of modern RAG architectures. Our mission is to transform the pre-sales workflow for Solutions Architects by turning hours of manual research into seconds of actionable, trusted, and visually impressive plans. We target the most time-consuming SA tasks: lift-and-shift migrations, hybrid architecture design, disaster recovery planning, and security compliance analysis.

Key Demo Features in Detail:

Hybrid Retrieval Engine: A sophisticated pipeline that combines metadata filtering (SQLite), dense vector search (ChromaDB), and a live web search fallback to produce highly relevant, timely, and trustworthy context.

On-Demand Plan Generation: SAs can request complex plans and receive a comprehensive, synthesized response in seconds.

Automated Architecture Diagramming: The assistant can generate and render architecture diagrams (using Mermaid.js) based on the user's request.

Actionable Code Scaffolding: Delivers ready-to-use code snippets (e.g., Terraform, AWS CLI commands) directly within the response.

Seamless Teams Integration: All interactions happen within Microsoft Teams, using Adaptive Cards to present rich, interactive responses.

Component Breakdown (Go Microservices):

Ingestion Service (cmd/ingest): Go. A CLI tool that parses documents, generates embeddings via OpenAI, and loads them into ChromaDB.

Retrieval API (cmd/retrieve): Go. A REST API that handles hybrid search (metadata + vector).

Web Search Service (cmd/websearch): Go. A service that calls an external API to fetch live web results for freshness.

Synthesis Service (cmd/synthesize): Go. A REST API that uses a large language model (GPT-4o) to generate answers, diagrams, and code.

Teams Bot Adapter (cmd/teamsbot): Go. The user-facing service that orchestrates backend calls and posts Adaptive Cards to Teams.

Tech Stack

Go for all microservices.

Docker & Docker Compose for containerization and local orchestration.

Gin for the REST API framework.

Viper for configuration management.

OpenAI API, ChromaDB, SQLite for the core RAG pipeline.

Microsoft Teams Adaptive Cards for the UI.

Designed For

Solutions Architects preparing for customer engagements.

Pre-sales managers seeking to improve team efficiency and win rates.

Engineers demonstrating the value of modern RAG and LLM-powered solutions.

B. Quick Issue Analysis

echo "🔍 Reviewing issue #$ARGUMENTS…"
gh issue view $ARGUMENTS --json title,body,state,labels

Understand the context and scope of the issue.

Review the existing microservice architecture.

Place new files in the appropriate cmd/ or internal/ packages. Do not create new top-level folders unless absolutely necessary.

C. Implementation Approach

Fix the Root Cause

Identify and address the core issue in the appropriate Go service(s).

Think about the boundaries between services. Ensure the fix is placed in the correct service to maintain separation of concerns.

Follow Strict Coding Standards

Validate your code against the standards below. Use the pre-commit hooks for automation.

Go (1.23.5): gofmt, golangci-lint

Docker: Use multi-stage builds in Dockerfiles to create minimal final images.

Docker Compose: Use v2.x syntax. Clearly define dependencies with depends_on.

YAML / JSON / Markdown: Must be well-formatted.

Test Your Changes

Unit Tests: If you modify logic in an internal/ package or a service handler, add or update unit tests to cover the change.

Integration Tests: If your change affects the interaction between two services (e.g., teamsbot calling synthesize), ensure you run integration tests that validate this contract.

Verify the Fix

Run the full application stack using docker-compose up.

Manually test the user flow affected by your change to confirm the issue is resolved and no regressions were introduced.

D. Quick Testing Script

This script is designed for our Go monorepo structure. It runs formatters, linters, and tests across the entire project to ensure that changes (especially in the internal/ directory) do not break other services.

# !/bin/bash
set -e

echo "Running Go formatter..."
go fmt ./...

echo "Running Go linter..."
golangci-lint run ./...

echo "Running all Go unit tests..."
go test -v ./...

echo "✅ All local checks passed."

GitHub issue updated with a summary of the work completed.

Root cause addressed per issue requirements.

All applicable coding standards followed.

All impacted unit tests pass.

No breaking changes introduced to API contracts between services.

Basic functionality verified manually with docker-compose.

F. Close Issue
Do not commit or push any code
If the issue has been fully implemented and all acceptance criteria are met, close the issue and add a descriptive comment summarizing the work using the gh CLI.

# Example

gh issue close $ARGUMENTS --comment "Fixed the issue by adding error handling in the synthesize service. All unit tests pass and the Teams card now displays a user-friendly error message on failure."
