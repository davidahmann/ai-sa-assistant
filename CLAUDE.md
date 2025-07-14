AI-Powered Cloud SA Assistant

1. Project Philosophy & Core Value Proposition

The AI-Powered Cloud SA Assistant is an internal, demo-level solution designed to showcase the power of modern RAG (Retrieval-Augmented Generation) architectures. Our mission is to transform the pre-sales workflow for Solutions Architects by turning hours of manual research into seconds of actionable, trusted, and visually impressive plans. We target the most time-consuming SA tasks: lift-and-shift migrations, hybrid architecture design, disaster recovery planning, and security compliance analysis.

Core Philosophy:

We believe that the best AI assistants are not just black boxes but are transparent systems that combine the best of internal knowledge, live external data, and powerful synthesis. Our strategy is to build a modular, Go-based microservices application that demonstrates a best-in-class hybrid retrieval pipeline. The "wow" factor comes from the speed, accuracy, and the assistant's ability to generate not just text, but also architecture diagrams and code scaffolds directly within Microsoft Teams.

Key Demo Features in Detail:

Hybrid Retrieval Engine: A sophisticated pipeline that combines metadata filtering (SQLite), dense vector search (ChromaDB), and a live web search fallback to produce highly relevant, timely, and trustworthy context for the LLM.

On-Demand Plan Generation: SAs can request complex plans (e.g., "AWS lift-and-shift plan for 120 VMs") and receive a comprehensive, synthesized response in seconds.

Automated Architecture Diagramming: The assistant can generate and render architecture diagrams (using Mermaid.js) based on the user's request, providing instant visual clarity for complex solutions.

Actionable Code Scaffolding: Delivers ready-to-use code snippets (e.g., Terraform, AWS CLI commands) directly within the response, accelerating PoC and implementation work.

Seamless Teams Integration: All interactions happen within Microsoft Teams, using Adaptive Cards to present rich, interactive responses that include text, diagrams, sources, and feedback buttons.

2. Architecture & Technology Stack Deep Dive

This stack describes the components of the ai-sa-assistant project, designed to run locally via Docker Compose for the demo.

Component Breakdown (Go Microservices):

Ingestion Service (cmd/ingest): Go. A CLI tool that reads source documents (Markdown, PDFs), chunks them, generates embeddings via the OpenAI API, and loads them into the ChromaDB vector store.

Retrieval API (cmd/retrieve): Go. A REST API (Gin/Fiber) that handles the core RAG logic. It filters documents by metadata (from SQLite), performs vector searches against ChromaDB, and includes fallback logic. **Owns and manages the metadata.db SQLite database**, including initialization and all metadata operations.

Web Search Service (cmd/websearch): Go. A simple service that detects "freshness" keywords and calls an external API (e.g., ChatGPT browsing) to fetch live web results. Runs on port 8083.

Synthesis Service (cmd/synthesize): Go. A REST API that gathers context from the Retrieval and Web Search services, constructs a detailed prompt, and calls the OpenAI Chat Completion API to generate the final answer. Runs on port 8082.

Teams Bot Adapter (cmd/teamsbot): Go. The user-facing service that listens for messages from a Teams Incoming Webhook, orchestrates calls to the backend services, and posts the final Adaptive Card back to the channel. Runs on port 8080.

Local Demo Infrastructure Stack in Detail:

Orchestration: Docker Compose. A single docker-compose.yml file is used to define, launch, and network all five Go microservices and the ChromaDB container.

Vector Store: ChromaDB. Runs in a dedicated Docker container, accessible to other services over the internal Docker network.

Metadata Store: SQLite. A simple metadata.db file, managed by the Retrieval API, used for fast and efficient metadata filtering before the vector search.

Configuration: Viper. All configuration, including API keys and service endpoints, is managed via a config.yaml file and environment variables.

3. Coding Standards

We enforce strict Go standards, validated automatically in our CI pipeline.

Go 1.23.5: gofmt for formatting, golangci-lint v2.2.1 for linting.

**Dependencies (with versions)**:

- github.com/gin-gonic/gin v1.10.0 (HTTP framework)
- github.com/sashabaranov/go-openai v1.26.0 (OpenAI API client)
- github.com/spf13/viper v1.19.0 (Configuration management)
- github.com/mattn/go-sqlite3 v1.14.22 (SQLite driver)
- go.uber.org/zap v1.27.0 (Structured logging)
- github.com/spf13/cobra v1.8.0 (CLI framework)

Docker: All services must have a minimal, multi-stage Dockerfile.

Docker Compose: v2.x syntax.

**Frontend/JavaScript Standards**:

We enforce strict JavaScript/frontend code quality standards for the Web UI component.

Node.js 18+: Required for running frontend tooling and development dependencies.

JavaScript Tooling Stack:

- ESLint v9.16.0: Modern flat config format with comprehensive linting rules
- Prettier v3.4.2: Code formatting with 4-space indentation matching Go style
- Husky v9.1.7: Git hooks for automated code quality checks
- lint-staged v15.2.10: Run linters on staged files only

**Frontend File Structure**:

- `cmd/webui/static/app.js`: Main frontend application (1600+ lines)
- `cmd/webui/static/sw.js`: Service worker for PWA functionality
- `cmd/webui/static/libs/`: Third-party libraries (Mermaid.js, Prism.js)

**Code Quality Commands**:

- `npm run lint`: Run ESLint with zero warnings policy
- `npm run format`: Auto-fix code formatting with Prettier
- `npm run validate`: Run both format check and linting
- Pre-commit hooks: Automatically format and lint JS files before commit

**CI Integration**: JavaScript linting and formatting checks are integrated into the GitHub Actions CI pipeline alongside Go linting.

4. Testing Strategy

Our testing strategy is designed to ensure each component is reliable before integration.

Pre-commit: Local hooks run gofmt, golangci-lint v2.2.1 --fast, Prettier formatting, and ESLint for JavaScript files.

Unit Tests (CI): On every Pull Request, a suite of unit tests is run for each Go service, validating internal logic (e.g., chunking, prompt templating, API handlers).

Integration Tests (CI): The PR pipeline uses Docker Compose to spin up the full stack and run tests that verify service-to-service communication (e.g., Teams Bot -> Synthesis -> Retrieval -> ChromaDB).

E2E Tests (Manual): The four core demo scenarios are run manually from a Teams client to verify the final output in the Adaptive Card before any major change is merged.

Quality Gates: Pull Requests cannot be merged unless they pass all Unit and Integration test stages. We aim for ≥80% backend unit test coverage.

5. Build & Test Prerequisites

All contributors must have these tools installed locally for a consistent development experience.

Go 1.23.5: To test: go test ./... in each service directory.

Node.js 18+: Required for frontend development. Install JavaScript dependencies: npm install. Run frontend validation: npm run validate.

Docker & Docker Compose: Required for running the full application stack. To launch: docker-compose up.

API Keys & Config: A valid config.yaml file or environment variables (OPENAI_API_KEY, TEAMS_WEBHOOK_URL) must be present. **Configuration Precedence**: Environment variables always override config.yaml values, allowing for flexible deployment scenarios. The config package validates that all required configurations are present at startup.

6. Key Features & Data Models

Core Data Model: The Synthesized Response

The central data object is the SynthesizedResponse, a JSON object returned by the Synthesis Service. It contains the complete, user-facing payload: main_text (the prose answer), diagram_code (Mermaid.js syntax), code_snippets (e.g., Terraform), and sources (a list of document and URL citations).

Feature Deep Dive: Hybrid Retrieval Pipeline

This is the intelligent core of the assistant. When a query is received by the Retrieval API:

Metadata Filter (Optional): If the query contains filters (e.g., scenario: "lift-and-shift"), it first queries metadata.db via SQLite to get a list of relevant doc_ids.

Chunk Search: The user query is embedded, and a vector search is run against ChromaDB. The search is restricted to the doc_ids from the previous step, if applicable.

Fallback Search: If the initial search returns too few results or the confidence scores are low, the doc_id filter is dropped, and a broader vector search is performed.

The top N chunks are returned to be used as context.

Feature Deep Dive: Automated Diagram Generation

This feature provides the "wow" factor of instant visualization.

The Synthesis Service includes a specific instruction in its prompt to the LLM: "Based on the following context, generate a Mermaid.js 'graph TD' diagram representing the high-level architecture."

The LLM returns the diagram syntax within a specific markdown code block.

The Teams Bot Adapter extracts this Mermaid code and passes it to an image rendering service (or uses a library) to generate a PNG, which is then embedded in the Adaptive Card.

Generated mermaid
// Example Mermaid.js output from the LLM
graph TD;
    subgraph On-Premises;
        VMs[120 Windows & Linux VMs];
    end;
    subgraph AWS Cloud;
        VPC[VPC: 10.0.0.0/16];
        PublicSubnet[Public Subnet];
        PrivateSubnet[Private Subnet];
        MGN[AWS MGN Replication];
        EC2[EC2 Instances];
    end;
    VMs -->|Block-Level Replication| MGN;
    MGN --> EC2;
    VPC --> PublicSubnet & PrivateSubnet;
    PrivateSubnet --> EC2;

7. Git Workflow & Release Process

Branching: We use a trunk-based development model. All work is done on short-lived feature branches (feature/*) branched from main.

Versioning: This is a demo project, so we use simple version tags (v1.0, v1.1-demo-ready). The version is stored in a VERSION file.

Release Process: Fully containerized. A "release" consists of building and pushing the final Docker images for each service to a container registry.

8. Common Issues & Prevention

OpenAI API Failures: Rate limits or invalid keys are common. Our go-openai client wrapper **implements exponential backoff with 3 retry attempts** and structured logging for 4xx errors. Base delay starts at 1 second and doubles with each retry attempt. Ensure keys are correctly loaded by Viper.

Poor Synthesis Quality: If the LLM produces irrelevant or generic answers, the root cause is usually poor retrieval context. Use logging to inspect the chunks and sources being fed into the synthesis prompt.

ChromaDB Container Fails: The ChromaDB container must be running before the other services. Our docker-compose.yml **uses health checks and depends_on conditions** to enforce the correct startup order.

Teams Adaptive Card Rendering Issues: Malformed JSON will cause cards to fail silently. All generated card JSON **is validated using structured types** before posting to the webhook. **Standard error response format**: All services return consistent JSON error objects with {"error": "user-friendly message"} structure and appropriate HTTP status codes (400, 500, 503).

9. Demo Scenarios Explained

The system showcases different RAG pipeline capabilities through four distinct demo scenarios:

### Demo 1: AWS Lift-and-Shift Migration

**User Prompt**: @SA-Assistant Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS, including EC2 instance recommendations, VPC/subnet topology, and the latest AWS MGN best practices from Q2 2025.

**Pipeline Flow**:

- **Metadata Filter**: Filters for docs with scenario="migration" AND cloud="aws"
- **Chunk Retrieval**: Extracts EC2 sizing recommendations and VPC design patterns
- **Freshness Detection**: "Q2 2025" triggers web search for latest AWS MGN updates
- **Synthesis**: Combines internal playbook with live updates, generates Mermaid VPC diagram and AWS CLI commands

**Output**: Comprehensive migration plan with architecture diagram, instance sizing table, and implementation scripts.

### Demo 2: Azure Hybrid Architecture Extension

**User Prompt**: @SA-Assistant Outline a hybrid reference architecture connecting our on-prem VMware environment to Azure, covering ExpressRoute configuration, VMware HCX migration, active-active failover, and June 2025 Azure Hybrid announcements.

**Pipeline Flow**:

- **Metadata Filter**: Filters for docs with scenario="hybrid" AND cloud="azure"
- **Chunk Retrieval**: Extracts ExpressRoute peering steps and VMware HCX workflows
- **Freshness Detection**: "June 2025" triggers web search for Azure Arc for VMware updates
- **Synthesis**: Generates ExpressRoute topology diagram and PowerShell configuration scripts

**Output**: Hybrid architecture guide with network diagrams, configuration templates, and migration workflows.

### Demo 3: Azure Disaster Recovery as a Service

**User Prompt**: @SA-Assistant Design a DR solution in Azure for critical workloads with RTO = 2 hours and RPO = 15 minutes, including geo-replication options, failover orchestration, and cost-optimized standby.

**Pipeline Flow**:

- **Metadata Filter**: Filters for docs with scenario="disaster-recovery"
- **Chunk Retrieval**: Extracts Azure Site Recovery configuration and RTO/RPO patterns
- **Fallback Search**: Expands search when initial results are insufficient
- **Synthesis**: Creates DR architecture diagram with specific RTO/RPO configurations

**Output**: DR solution blueprint with architecture diagrams, cost analysis, and orchestration procedures.

### Demo 4: Security Compliance Assessment

**User Prompt**: @SA-Assistant Summarize HIPAA and GDPR encryption, logging, and policy enforcement requirements for our AWS landing zone, and include any recent AWS compliance feature updates.

**Pipeline Flow**:

- **Metadata Filter**: Filters for docs with scenario="security" AND tags containing "compliance"
- **Chunk Retrieval**: Extracts HIPAA and GDPR encryption standards and logging requirements
- **Freshness Detection**: "recent" triggers web search for latest AWS compliance features
- **Synthesis**: Generates executive-friendly compliance checklist with actionable items

**Output**: Compliance assessment with requirements matrix, implementation checklist, and recent updates summary.

**Performance Target**: All demo scenarios complete in under 30 seconds with rich, multi-modal responses.

10. CI/CD Safeguards

Dependency Scanning: We use govulncheck to scan for known vulnerabilities in our Go dependencies.

Secret Scanning: GitHub secret scanning is enabled on the repository to prevent accidental key commits.

Container Image Scanning: Docker images are scanned for OS-level vulnerabilities upon build.

11. Security of the Local Demo Architecture

The demo is designed to run in an isolated environment and is secure by default for this purpose.

Secret Management: No secrets are hardcoded. All API keys are injected via a config.yaml file (which is in .gitignore) or environment variables managed by Docker Compose.

Network Isolation: All five microservices and ChromaDB communicate over a private, user-defined Docker bridge network. Only the Teams Bot Adapter's port would theoretically be exposed if needed, but for a webhook-based interaction, no inbound ports are required.

12. Developer Instructions

Adhere to this guide—all architecture and Go code must follow these principles.

Run go test ./... and golangci-lint run locally before opening Pull Requests.

Document new services or major changes to the RAG pipeline in this Claude.md file.

Build services to be stateless; any required state should be in ChromaDB or SQLite.
