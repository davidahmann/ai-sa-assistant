# Create main directories

mkdir -p cmd/ingest cmd/retrieve cmd/websearch cmd/synthesize cmd/teamsbot
mkdir -p internal/chunker internal/chroma internal/metadata internal/openai internal/synth internal/teams
mkdir -p docs/playbooks
mkdir -p configs

# --- Create Top-Level Files ---

# .gitignore

cat <<EOF > .gitignore

# Binaries

*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary

*.test

# Output of the go coverage tool

*.out

# Config files

configs/config.yaml
metadata.db
.env

# Go workspace file

go.work

# VSCode

.vscode/
EOF

# README.md

cat <<EOF > README.md

# AI-Powered Cloud SA Assistant

This project is an AI assistant designed to accelerate pre-sales research for Solutions Architects. It runs as a set of Go microservices and interacts with users via Microsoft Teams.

## Architecture

- **Ingestion Service:** Parses documents, generates embeddings, and loads them into ChromaDB.
- **Retrieval API:** Performs hybrid search (metadata + vector) to find relevant context.
- **Web Search Service:** Fetches live information from the web for freshness.
- **Synthesis Service:** Uses a large language model (GPT-4o) to generate answers, diagrams, and code.
- **Teams Bot Adapter:** Serves as the user interface within Microsoft Teams.

## Getting Started

1. **Configure:** Copy \`configs/config.template.yaml\` to \`configs/config.yaml\` and fill in your API keys.
2. **Launch Services:** \`docker-compose up --build\`
3. **Ingest Data:** \`docker-compose run ingest\`
4. **Interact:** Send a message to your configured Teams channel.
EOF

# go.mod

cat <<EOF > go.mod
module github.com/your-org/ai-sa-assistant

go 1.23.5

require (
 github.com/gin-gonic/gin v1.10.0
 github.com/sashabaranov/go-openai v1.26.0
 github.com/spf13/viper v1.19.0
 github.com/mattn/go-sqlite3 v1.14.22
 go.uber.org/zap v1.27.0
)
EOF

# docker-compose.yml

cat <<EOF > docker-compose.yml
version: '3.8'

services:
  chromadb:
    image: chromadb/chroma:latest
    ports:
      - "8000:8000"
    volumes:
      - chromadb_data:/chroma/.chroma/

  retrieve:
    build:
      context: .
      dockerfile: cmd/retrieve/Dockerfile
    ports:
      - "8081:8080"
    volumes:
      - ./configs:/app/configs
    depends_on:
      - chromadb

  synthesize:
    build:
      context: .
      dockerfile: cmd/synthesize/Dockerfile
    ports:
      - "8082:8080"
    volumes:
      - ./configs:/app/configs

  teamsbot:
    build:
      context: .
      dockerfile: cmd/teamsbot/Dockerfile
    ports:
      - "8080:8080"
    volumes:
      - ./configs:/app/configs
    depends_on:
      - retrieve
      - synthesize

volumes:
  chromadb_data:
EOF

# --- Create Config & Docs Placeholders ---

# configs/config.template.yaml

cat <<EOF > configs/config.template.yaml
openai:
  apikey: "sk-..."

teams:
  webhook_url: "https://your-org.webhook.office.com/..."

services:
  retrieve_url: "http://retrieve:8080"
  synthesize_url: "http://synthesize:8080"
  websearch_url: "http://websearch:8080"

chroma:
  url: "http://chromadb:8000"

metadata:
  db_path: "./metadata.db"
EOF

# docs/playbooks/aws-lift-shift-guide.md

echo "# AWS Lift and Shift Guide" > docs/playbooks/aws-lift-shift-guide.md

# docs/metadata.json

echo '{"doc_id": "aws-lift-shift-guide.md", "scenario": "migration", "cloud": "aws"}' > docs/metadata.json

# --- Create Go Service Placeholders (cmd/*) ---

for service in ingest retrieve websearch synthesize teamsbot; do
  echo "package main

import \"fmt\"

func main() {
    fmt.Println(\"Starting $service service...\")
}" > cmd/\$service/main.go

# Create placeholder Dockerfile for services that run as servers

  if [[ "\$service" != "ingest" ]]; then
    cat <<EOF > cmd/\$service/Dockerfile
FROM golang:1.23.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
WORKDIR /app/cmd/$service
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/configs/ ./configs/
CMD ["./server"]
EOF
  fi
done

# --- Create Go Logic Placeholders (internal/*) ---

cat <<EOF > internal/chunker/chunker.go
package chunker

// Splitter splits text into chunks.
func Splitter(text string, chunkSize int) []string {
    // Placeholder for chunking logic
    return []string{text}
}
EOF

cat <<EOF > internal/chroma/client.go
package chroma

// Client is a wrapper for the ChromaDB API.
type Client struct {
    // Add ChromaDB client fields
}
EOF

cat <<EOF > internal/metadata/store.go
package metadata

// Store handles queries to the SQLite metadata database.
type Store struct {
    // Add SQLite db connection
}
EOF

cat <<EOF > internal/openai/client.go
package openai

import "github.com/sashabaranov/go-openai"

// Client is a wrapper for the go-openai client.
type Client struct {
    *openai.Client
}
EOF

cat <<EOF > internal/synth/builder.go
package synth

// BuildPrompt combines context into a final prompt for the LLM.
func BuildPrompt(query string, chunks []string, webResults []string) string {
    // Placeholder for prompt engineering logic
    return query
}
EOF

cat <<EOF > internal/teams/adaptive_card.go
package teams

// GenerateCard creates the JSON for a Teams Adaptive Card.
func GenerateCard(responseText string, diagramURL string) string {
    // Placeholder for Adaptive Card JSON template
    return \`{"type": "AdaptiveCard", "version": "1.5", "body": [{"type": "TextBlock", "text": "\$responseText"}]}\`
}
EOF

# .pre-commit-config.yaml

repos:

# Standard hooks for cleaning files

- repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.6.0
    hooks:
  - id: trailing-whitespace         # Trims trailing whitespace
  - id: end-of-file-fixer            # Ensures files end in a newline
  - id: check-yaml                   # Checks YAML file syntax
  - id: check-json                   # Checks JSON file syntax
  - id: check-added-large-files      # Prevents committing large files
  - id: detect-private-key         # Looks for private keys

# Go-specific hooks

- repo: https://github.com/golangci/golangci-lint
    rev: v1.59.1
    hooks:
  - id: golangci-lint
        args: [--fix] # Auto-fix issues where possible

- repo: https://github.com/dnephin/pre-commit-golang
    rev: v0.5.1
    hooks:
  - id: go-fmt        # Enforces go fmt
  - id: go-mod-tidy   # Enforces go mod tidy

# Secret scanning (generic)

- repo: https://github.com/gitleaks/gitleaks
    rev: v8.18.4
    hooks:
  - id: gitleaks-detect
        name: Detect hardcoded secrets

# Final message

echo ""
echo "✅ Repository scaffold created successfully for Go 1.23.5!"
echo "Navigate into the 'ai-sa-assistant' directory and open it in VS Code to get started."
