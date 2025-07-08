

Phase 1: Foundational Backend & Data Ingestion (Epic 1)

Goal: To build the core infrastructure and data pipeline that ingests, understands, and securely stores our internal playbooks and vendor documentation. This phase lays the groundwork for all subsequent intelligence.

Story 1.1: Core Infrastructure Setup

As a System Administrator, I want to set up and validate the core infrastructure services so that the development environment is ready for the application.

Task P1-E1.S1.T1: Deploy and Validate ChromaDB Container

Description: Launch the official ChromaDB Docker container and ensure it is running and accessible over the local network.

Acceptance Criteria:

The ChromaDB container is running, confirmed by the output of docker ps.

The container is accessible on the host at http://localhost:8000.

A curl http://localhost:8000/api/v1/health command returns a JSON response containing {"nanosecond heartbeat": ...}.

Dependencies: None.

Tech Stack: Docker, Docker Compose.

Task P1-E1.S1.T2: Initialize SQLite Metadata Store

Description: Create the SQLite database file (metadata.db) and populate it with initial data from the project's docs/metadata.json file.

Acceptance Criteria:

A file named metadata.db is created in the project's root directory.

The metadata.db file contains a table named metadata.

Running sqlite3 metadata.db "SELECT COUNT(*) FROM metadata;" returns a non-zero number matching the number of entries in metadata.json.

Dependencies: None.

Tech Stack: SQLite CLI.

Task P1-E1.S1.T3: Implement Configuration Management

Description: Set up the configuration loading mechanism for the Go applications. This involves creating the template file and a simple Go utility to read from it.

Acceptance Criteria:

A configs/config.template.yaml file exists.

A configs/config.yaml file (git-ignored) is created and populated with valid credentials (e.g., OpenAI API key).

A Go function in an internal/config package can successfully load and unmarshal the config.yaml file into a Go struct without errors.

Dependencies: None.

Tech Stack: Go, spf13/viper library.

Story 1.2: Document Parsing and Chunking

As the System, I want to parse various source documents (Markdown, PDF) into standardized text chunks so that they can be processed for embedding.

Task P1-E1.S2.T1: Implement Markdown Document Parser

Description: Develop a function within the internal/chunker package that takes a path to a Markdown file, reads its content, and prepares it for chunking.

Acceptance Criteria:

The function correctly reads the full content of a .md file into a string.

The function cleans the text by removing excessive newlines or other non-essential formatting characters.

Dependencies: None.

Tech Stack: Go (standard library).

Task P1-E1.S2.T2: Implement Text Chunking Logic

Description: Create a generic text-splitting function in the internal/chunker package. This function will take a single large string and split it into smaller, non-overlapping chunks of a specified size (e.g., 500 words).

Acceptance Criteria:

The function splits a given text into an array of strings ([]string).

Each chunk in the returned array is less than or equal to the specified chunkSize.

The logic correctly handles splitting on sentence boundaries to maintain semantic meaning where possible.

The last chunk contains the remaining text, even if it's smaller than the chunk size.

Dependencies: P1-E1.S2.T1.

Tech Stack: Go (standard library).

Unit Test Requirements: Must include tests for:

Text shorter than the chunk size (returns a single chunk).

Text longer than the chunk size (returns multiple chunks).

An empty string input (returns an empty array).

Text with various punctuation and line breaks.

Story 1.3: Embedding Generation and Storage

As the System, I want to generate vector embeddings for each text chunk and store them in ChromaDB so they are available for semantic search.

Task P1-E1.S3.T1: Implement OpenAI Embedding Client

Description: Create a client wrapper in the internal/openai package to interact with the OpenAI Embeddings API. This abstracts the API call logic.

Acceptance Criteria:

The client is initialized with the API key from the configuration file (via the task P1-E1.S1.T3).

A method exists that takes a []string of text chunks and returns a [][]float32 of embeddings.

The method correctly handles API requests and gracefully returns errors from the API.

Dependencies: P1-E1.S1.T3.

Tech Stack: Go, sashabaranov/go-openai library.

Unit Test Requirements: Mock the OpenAI API client to test:

Correct request formation.

Proper handling of a successful API response.

Graceful error handling for API failures (e.g., 401, 429 errors).

Task P1-E1.S3.T2: Implement ChromaDB Client Wrapper

Description: Create a client wrapper in the internal/chroma package to handle communication with the ChromaDB REST API. The first implementation will focus on adding documents.

Acceptance Criteria:

The client can be initialized with the ChromaDB URL from the configuration file.

A method AddDocuments exists that takes embeddings, their corresponding text chunks, and metadata.

The method successfully makes a POST request to the ChromaDB /api/v1/collections/{name}/add endpoint.

The method handles HTTP status codes and returns an error if the ingestion fails.

Dependencies: P1-E1.S1.T1, P1-E1.S1.T3.

Tech Stack: Go (standard net/http library).

Unit Test Requirements: Use an HTTP mocking library to test:

Correct formation of the JSON payload for the ChromaDB API.

Proper handling of a 200 OK response.

Error handling for non-200 responses.

Testing Tasks for Phase 1

Task P1-E1.T1: Create Ingestion Service CLI

Description: Build the main application logic for the cmd/ingest service. This CLI tool will tie together the parsing, chunking, embedding, and storage logic into a single, executable pipeline.

Acceptance Criteria:

The CLI tool can be executed via go run ./cmd/ingest.

The tool reads document paths from a directory specified via a command-line flag (e.g., --docs-path=./docs).

The tool orchestrates the full pipeline: Read -> Parse -> Chunk -> Embed -> Store.

It logs progress (e.g., "Ingesting file X...", "Storing N chunks...") using a structured logger like Zap.

Dependencies: P1-E1.S2.T2, P1-E1.S3.T1, P1-E1.S3.T2.

Tech Stack: Go, spf13/cobra (optional, for CLI flags), go.uber.org/zap.

Task P1-E1.T2: Phase 1 Integration Test

Description: Perform a full end-to-end test of the ingestion pipeline to ensure all components work together correctly. This is a manual test procedure for this phase.

Acceptance Criteria:

The docker-compose.yml file successfully launches the ChromaDB container.

The cmd/ingest CLI tool runs to completion without errors.

After the CLI finishes, a manual curl request to the ChromaDB /api/v1/collections/cloud_assistant/get endpoint shows that the count of documents is greater than zero and matches the number of chunks generated.

Dependencies: P1-E1.S1.T1, P1-E1.T1.

Tech Stack: Docker Compose, curl.

Of course. Here is the detailed, task-by-task work plan for the second epic, following the same rigorous structure.

Phase 2: Core RAG Pipeline & Synthesis API (Epic 2)

Goal: To develop the intelligent heart of the assistant—a set of APIs that can retrieve the most relevant information from our knowledge base, supplement it with live web data, and synthesize it into a coherent answer.

Story 2.1: Basic Vector Search Retrieval

As a Solutions Architect (SA), I want to submit a query and receive the most relevant chunks from our internal documents so that I can get a foundational answer to my question.

Task P2-E2.S1.T1: Create the Retrieval API Server

Description: Build the skeleton for the retrieve service. This includes setting up an HTTP server using the Gin framework and defining the main API endpoint (/search).

Acceptance Criteria:

The cmd/retrieve/main.go file initializes and runs a Gin server.

The server listens on the port defined in the configuration (e.g., 8081).

A POST /search endpoint is registered but contains only placeholder logic initially.

The service starts without errors when run via go run or Docker.

Dependencies: P1-E1.S1.T3.

Tech Stack: Go, gin-gonic/gin.

Task P2-E2.S1.T2: Implement Vector Search in ChromaDB Client

Description: Extend the internal/chroma client wrapper to include a method for querying/searching for documents.

Acceptance Criteria:

A Search method is added to the ChromaDB client.

The method accepts a query vector ([]float32) and a number of results to return (top_n).

It correctly constructs and sends a POST request to the ChromaDB /api/v1/collections/{name}/query endpoint.

It successfully parses the JSON response from ChromaDB into a Go struct containing the retrieved document chunks and their similarity scores.

Dependencies: P1-E1.S3.T2.

Tech Stack: Go (standard net/http library).

Unit Test Requirements: Mock the ChromaDB API to test:

Correct formation of the query JSON payload.

Successful parsing of a valid response.

Graceful error handling for API failures.

Task P2-E2.S1.T3: Implement the Basic Search Handler

Description: Wire the full logic for the POST /search endpoint. This involves receiving a user query, embedding it, and using the ChromaDB client to find relevant chunks.

Acceptance Criteria:

The handler receives a JSON payload containing the user's query string.

It calls the OpenAI client (from P1-E1.S3.T1) to convert the query string into a vector.

It calls the ChromaDB client's Search method (from P2-E2.S1.T2) with the query vector.

It returns a JSON response containing an array of the top 3-5 retrieved document chunks and their metadata.

Dependencies: P2-E2.S1.T1, P2-E2.S1.T2, P1-E1.S3.T1.

Tech Stack: Go, gin-gonic/gin.

Story 2.2: Advanced Retrieval with Metadata Filtering & Fallback

As an SA, I want to filter my search by document type (e.g., "playbook," "security") so that I get more precise and relevant results for my specific task.

Task P2-E2.S2.T1: Implement Metadata Query Logic

Description: Create a function in the internal/metadata store that queries the SQLite database to find document IDs matching a given set of filters (e.g., cloud=aws).

Acceptance Criteria:

The function connects to the metadata.db file.

It accepts a map or struct representing the filters.

It dynamically builds a SELECT doc_id FROM metadata WHERE ... query based on the input filters.

It returns a slice of strings ([]string) containing the matching document IDs.

Dependencies: P1-E1.S1.T2.

Tech Stack: Go, mattn/go-sqlite3.

Unit Test Requirements: Must include tests for:

A single filter that returns results.

Multiple filters that return results.

Filters that return no results (returns an empty slice).

Invalid or empty filter input.

Task P2-E2.S2.T2: Enhance Search Handler with Filtering

Description: Modify the POST /search handler to accept metadata filters. It should now perform the metadata query first and then pass the results to the ChromaDB client.

Acceptance Criteria:

The /search endpoint's JSON payload can now optionally include a filters object.

If filters are present, the handler first calls the metadata store (from P2-E2.S2.T1) to get the list of doc_ids.

The call to the ChromaDB client's Search method now includes this list of doc_ids to restrict the search space.

If no filters are provided, the search functions as it did in Story 2.1.

Dependencies: P2-E2.S1.T3, P2-E2.S2.T1.

Tech Stack: Go, gin-gonic/gin.

Task P2-E2.S2.T3: Implement Retrieval Fallback Logic

Description: Add logic to the /search handler to automatically re-run a broader search if the initial filtered search yields poor results.

Acceptance Criteria:

After a filtered search is performed, the handler checks the results.

If the number of returned chunks is below a threshold (e.g., < 3) OR their confidence scores are too low, a condition is triggered.

When triggered, the handler makes a second call to the ChromaDB Search method, this time without the doc_id filters.

The final response to the user indicates that a fallback search was performed.

Dependencies: P2-E2.S2.T2.

Tech Stack: Go.

Story 2.3: Live Web Search for Freshness

As an SA, I want the assistant to automatically search the web for recent updates when I ask for the "latest" information so that my plans are always current.

Task P2-E2.S3.T1: Implement Freshness Keyword Detection

Description: Create a simple utility function that scans a query string for keywords indicating a need for fresh, time-sensitive information.

Acceptance Criteria:

The function returns true if the query contains keywords like "latest," "recent," "update," or a specific month/year (e.g., "June 2025").

The function returns false otherwise.

The check is case-insensitive.

Dependencies: None.

Tech Stack: Go (standard library).

Unit Test Requirements: Must test for various keywords, a mix of cases, and queries with no keywords.

Task P2-E2.S3.T2: Create Web Search Service & API

Description: Build the skeleton for the websearch service with a POST /search endpoint. This service will be responsible for calling an external web search provider.

Acceptance Criteria:

The cmd/websearch/main.go file initializes and runs a Gin server.

The handler accepts a JSON payload with the query string.

It calls an external search provider (e.g., via the OpenAI API's browsing feature or another search API).

It returns a structured JSON response containing 2-3 web snippets and their source URLs.

Dependencies: P1-E1.S1.T3.

Tech Stack: Go, gin-gonic/gin, net/http.

Story 2.4: AI Synthesis of Retrieved Context

As an SA, I want a single, synthesized, and human-readable answer that combines insights from internal docs and web searches so I don't have to piece it together myself.

Task P2-E2.S4.T1: Create the Synthesis API Server

Description: Build the skeleton for the synthesize service with a POST /synthesize endpoint.

Acceptance Criteria:

The cmd/synthesize/main.go file initializes and runs a Gin server.

The server listens on the port defined in the configuration (e.g., 8082).

The /synthesize endpoint is registered and accepts a JSON payload containing the original query, the retrieved document chunks, and any web search results.

Dependencies: P1-E1.S1.T3.

Tech Stack: Go, gin-gonic/gin.

Task P2-E2.S4.T2: Implement Prompt Building Logic

Description: Develop a function in the internal/synth package that takes all the retrieved context and constructs a single, comprehensive prompt for the Large Language Model.

Acceptance Criteria:

The function accepts the original query, a slice of internal document chunks, and a slice of web search snippets.

It assembles a string that clearly delineates each piece of context (e.g., "--- Internal Document Context ---", "--- Live Web Search Results ---").

The prompt includes explicit instructions for the LLM on how to behave (e.g., "You are an expert Cloud Solutions Architect. Synthesize the provided information into a coherent plan...").

Dependencies: None.

Tech Stack: Go (standard library strings or bytes package).

Unit Test Requirements: Test that the prompt is correctly assembled with all parts present and correctly labeled.

Task P2-E2.S4.T3: Implement the Synthesis Handler

Description: Implement the full logic for the /synthesize handler. It will build the prompt and call the OpenAI Chat Completion API to get the final answer.

Acceptance Criteria:

The handler calls the prompt builder (from P2-E2.S4.T2) to create the final prompt.

It calls the OpenAI client (from P1-E1.S3.T1) using a powerful reasoning model (e.g., GPT-4o) with the constructed prompt.

It receives the text response from the LLM.

It returns a JSON object containing the final, synthesized answer.

Dependencies: P2-E2.S4.T1, P2-E2.S4.T2, P1-E1.S3.T1.

Tech Stack: Go, gin-gonic/gin.

Testing Tasks for Phase 2

Task P2-E2.T1: Integration Test for Retrieval API

Description: Create an automated test that validates the full functionality of the retrieve service.

Acceptance Criteria:

The test starts the ChromaDB container.

It makes a POST request to the running retrieve service's /search endpoint without filters and verifies a successful response.

It makes a second POST request with valid filters and verifies the response contains chunks only from the expected documents.

It makes a third POST request with filters that should yield no results, verifying that the fallback logic is triggered and a non-empty response is still returned.

Dependencies: P2-E2.S2.T3.

Tech Stack: Go (standard testing package, net/http).

Phase 3: "Showstopper" Features & Demo Polish (Epic 3)

Goal: To implement the high-impact features that create a "wow" moment. This phase transforms the assistant from a text-based search tool into a true, multi-modal planning partner, ensuring the demo is trustworthy, visually appealing, and professional.

Story 3.1: Automated Architecture Diagram Generation

As an SA, I want the assistant to generate a high-level architecture diagram based on my request so that I can instantly visualize the solution for myself and my customer.

Task P3-E3.S1.T1: Enhance Prompt Builder for Diagram Generation

Description: Modify the prompt building logic in the internal/synth package to explicitly instruct the LLM to generate a diagram using Mermaid.js syntax.

Acceptance Criteria:

The prompt sent to the LLM must contain a clear, specific instruction, such as: "Additionally, generate a high-level architecture diagram for this solution using Mermaid.js graph TD syntax."

The instruction must specify that the diagram code should be enclosed within a unique markdown code block (e.g., mermaid ...) for easy parsing.

Dependencies: P2-E2.S4.T2.

Tech Stack: Go.

Unit Test Requirements: The unit test for the prompt builder must assert that the Mermaid-specific instruction is present in the final prompt string.

Task P3-E3.S1.T2: Enhance Synthesis Service to Parse Diagram Code

Description: Upgrade the POST /synthesize handler to parse the LLM's full response. It must separate the main prose answer from the generated Mermaid diagram code.

Acceptance Criteria:

The /synthesize API's JSON response is changed from {"answer": "..."} to a structured object: {"main_text": "...", "diagram_code": "graph TD; A-->B;"}.

The handler correctly extracts the code from within the mermaid ... block.

If no diagram code block is found in the LLM response, the diagram_code field must be an empty string.

Dependencies: P3-E3.S1.T1.

Tech Stack: Go, gin-gonic/gin, regexp (for parsing).

Unit Test Requirements: Unit tests must cover parsing various LLM response formats: with a diagram, without a diagram, and with an empty diagram block.

Story 3.2: Actionable Code Scaffold Generation

As an SA, I want to receive ready-to-use code scaffolds (e.g., Terraform, CLI commands) for the proposed solution so I can accelerate my proof-of-concept work.

Task P3-E3.S2.T1: Enhance Prompt Builder for Code Generation

Description: Further modify the internal/synth prompt builder to also request relevant code snippets based on the user's query.

Acceptance Criteria:

The prompt must be appended with instructions like: "If applicable, also provide relevant code snippets for implementation, such as for Terraform or AWS CLI. Enclose each snippet in the appropriate markdown code block (e.g., ```terraform)."

Dependencies: P3-E3.S1.T1.

Tech Stack: Go.

Unit Test Requirements: The prompt builder unit test must be updated to assert that both the diagram and code generation instructions are present.

Task P3-E3.S2.T2: Enhance Synthesis Service to Parse Code Snippets

Description: Upgrade the /synthesize handler to parse multiple code snippets and their language identifiers from the LLM's response.

Acceptance Criteria:

The /synthesize API's JSON response is enhanced to include a code_snippets field.

This field must be a slice of structs, e.g., [{"language": "terraform", "code": "resource ..."}, {"language": "bash", "code": "aws ec2 ..."}].

The handler correctly parses multiple markdown code blocks and their language identifiers (e.g., terraform,bash).

Dependencies: P3-E3.S1.T2, P3-E3.S2.T1.

Tech Stack: Go, gin-gonic/gin, regexp.

Unit Test Requirements: Unit tests must cover parsing responses with no code, one code block, and multiple code blocks with different languages.

Story 3.3: Enhanced Trust with Inline Citations

As an SA, I need to trust the information provided, so I want to see exactly where each piece of information came from.

Task P3-E3.S3.T1: Pass Source Metadata to Synthesis Service

Description: Modify the data flow so that the source document ID (e.g., aws-lift-shift-guide.md) for each retrieved text chunk is passed through the system to the /synthesize endpoint.

Acceptance Criteria:

The request payload for POST /synthesize is updated to accept a list of context objects.

Each object must contain both the chunk's text and its source_id.

Dependencies: P2-E2.S4.T1.

Tech Stack: Go, gin-gonic/gin.

Task P3-E3.S3.T2: Enhance Prompt Builder for Inline Citations

Description: Modify the internal/synth prompt builder to instruct the LLM to perform inline citations and provide it with the source data to do so.

Acceptance Criteria:

The context provided to the LLM must be formatted to clearly associate each text chunk with its source_id.

The prompt instructions must include a rule like: "You MUST cite your sources. When using information from an internal document, end the sentence with its citation in brackets, like this: [source_id]."

Dependencies: P3-E3.S2.T1, P3-E3.S3.T1.

Tech Stack: Go.

Unit Test Requirements: Unit test must verify that the prompt correctly formats the context with source IDs and includes the citation rule.

Task P3-E3.S3.T3: Enhance Synthesis Service to Return Source List

Description: Modify the /synthesize handler to extract and de-duplicate all sources used for the response.

Acceptance Criteria:

The /synthesize API's JSON response is enhanced to include a sources field.

This field is a slice of strings containing all unique source document IDs and external web URLs that were provided as context.

Dependencies: P3-E3.S2.T2, P3-E3.S3.T1.

Tech Stack: Go, gin-gonic/gin.

Story 3.4: Demo Readiness & Professional Polish

As a Presenter, I need the system to be robust, observable, and well-documented so that I can run the demo flawlessly and the project can be maintained.

Task P3-E3.S4.T1: Implement Structured Logging in All Services

Description: Integrate the Zap structured logging library into all Go microservices (retrieve, synthesize, websearch, etc.), replacing all fmt.Println or log.Println statements.

Acceptance Criteria:

All services log key events, such as "service starting," "received API request," and "error occurred."

Logs are in JSON format and include structured fields (e.g., service_name, request_id, error).

Appropriate log levels (INFO, WARN, ERROR) are used for different events.

Dependencies: All main.go files from Phase 2.

Tech Stack: Go, go.uber.org/zap.

Task P3-E3.S4.T2: Implement Graceful API Error Handling

Description: Fortify all API endpoints to handle downstream failures gracefully and return a consistent, user-friendly JSON error format.

Acceptance Criteria:

If a service cannot connect to a dependency (e.g., synthesize cannot reach OpenAI), it must not crash.

It must return an appropriate HTTP status code (e.g., 503 Service Unavailable or 500 Internal Server Error).

The response body must be a JSON object like {"error": "A clear, user-friendly error message."}.

Dependencies: P3-E3.S4.T1.

Tech Stack: Go, gin-gonic/gin.

Testing Tasks for Phase 3

Task P3-E3.T1: Integration Test for Full Synthesis Payload

Description: Create an automated integration test for the synthesize service that mocks a complete request and validates the full, structured response.

Acceptance Criteria:

The test sends a valid POST request to /synthesize with a payload containing a query, chunks with source IDs, and web results.

The test asserts that the HTTP response is 200 OK.

The test asserts that the JSON response body contains non-empty fields for main_text, diagram_code, code_snippets, and sources.

Dependencies: P3-E3.S3.T3.

Tech Stack: Go (standard testing package, net/http).

Phase 4: Interactive Teams Bot Experience (Epic 4)

Goal: To deliver the assistant's power through a seamless and interactive Microsoft Teams interface. This phase focuses on the user-facing components that make the assistant feel like a true, integrated colleague.

Story 4.1: Core Bot Interaction and Response Delivery

As an SA, I want to invoke the assistant by @mentioning it in a Teams channel and get a response in a rich, easy-to-read format.

Task P4-E4.S1.T1: Create the Teams Bot Adapter Service

Description: Build the skeleton for the teamsbot service. This service will have a single endpoint (POST /teams-webhook) to receive messages from a Microsoft Teams Incoming Webhook.

Acceptance Criteria:

The cmd/teamsbot/main.go file initializes and runs a Gin server.

The server registers a POST /teams-webhook endpoint.

The service correctly reads the TEAMS_WEBHOOK_URL from the configuration for later use.

The service starts without errors.

Dependencies: P1-E1.S1.T3.

Tech Stack: Go, gin-gonic/gin.

Task P4-E4.S1.T2: Implement Backend Service Orchestration

Description: Implement the core logic within the /teams-webhook handler to call the backend services (retrieve, websearch, synthesize) in the correct sequence.

Acceptance Criteria:

The handler parses the inbound Teams message to extract the user's query text.

It makes a POST request to the /search endpoint of the retrieve service.

(Conditional) If needed, it makes a POST request to the websearch service.

It then makes a POST request to the /synthesize service, passing all the context gathered from the previous calls.

It successfully receives the final structured JSON response from the synthesize service.

Dependencies: P2-E2.T1, P3-E3.T1.

Tech Stack: Go (standard net/http client).

Task P4-E4.S1.T3: Implement Basic Adaptive Card Template

Description: Create a function in the internal/teams package that takes the synthesized response and populates a basic Adaptive Card JSON template.

Acceptance Criteria:

The function takes the main_text from the synthesis response.

It returns a valid JSON string representing an Adaptive Card.

The card template should, at a minimum, contain a TextBlock element displaying the main answer.

Dependencies: None.

Tech Stack: Go (standard text/template or encoding/json package).

Task P4-E4.S1.T4: Implement Response Posting to Teams

Description: Implement the final step in the orchestration flow: posting the generated Adaptive Card back to the Teams channel using the configured Incoming Webhook URL.

Acceptance Criteria:

After receiving a successful response from the synthesize service, the handler calls the Adaptive Card generator function.

It makes an HTTP POST request to the TEAMS_WEBHOOK_URL.

The body of the POST request is the generated Adaptive Card JSON.

The card appears correctly in the designated Teams channel.

Dependencies: P4-E4.S1.T2, P4-E4.S1.T3.

Tech Stack: Go (standard net/http client).

Story 4.2: Rich Visuals in Adaptive Cards

As an SA, I want to see the generated diagrams and code snippets presented cleanly and professionally within the Teams card.

Task P4-E4.S2.T1: Integrate Diagram Rendering

Description: Enhance the teamsbot service to handle diagram rendering. This involves taking the Mermaid.js code, converting it to an image, and hosting it publicly.

Acceptance Criteria:

If the diagram_code from the synthesis response is not empty, the teamsbot service makes a call to a public Mermaid.js rendering service (or a self-hosted one).

The service successfully receives an image URL in response.

Note: For simplicity, a public service like https://mermaid.ink can be used for the demo.

Dependencies: P3-E3.S1.T2.

Tech Stack: Go (net/http).

Task P4-E4.S2.T2: Enhance Adaptive Card with Images, Code, and Sources

Description: Upgrade the internal/teams Adaptive Card generator to be a rich, multi-element template.

Acceptance Criteria:

The function now accepts the full structured response from the synthesize service.

If a diagram URL is provided, it adds an Image element to the card.

It iterates through the code_snippets and adds a TextBlock with fontType: "Monospace" for each.

It iterates through the sources and adds a formatted list of sources to the bottom of the card.

Dependencies: P4-E4.S1.T3, P4-E4.S2.T1.

Tech Stack: Go (text/template or encoding/json).

Unit Test Requirements: Must test card generation with all fields present, and also with optional fields (like diagram_url) being empty.

Story 4.3: User Feedback Mechanism

As an SA, I want to provide quick feedback on the quality of an answer so that the system can be improved over time.

Task P4-E4.S3.T1: Add Feedback Buttons to Adaptive Card

Description: Modify the Adaptive Card template to include "👍" and "👎" buttons. These buttons will trigger a Action.Http post back to a new endpoint.

Acceptance Criteria:

The generated Adaptive Card JSON includes an actions array.

The array contains two actions of type Action.Http.

Each action posts to a new endpoint (e.g., POST /teams-feedback).

The payload of the feedback action includes the original query and a feedback value ("good" or "bad").

Dependencies: P4-E4.S2.T2.

Tech Stack: Go (text/template or encoding/json).

Task P4-E4.S3.T2: Create Feedback Endpoint and Logger

Description: Implement the POST /teams-feedback endpoint in the teamsbot service and a simple logger to store the feedback.

Acceptance Criteria:

A new /teams-feedback endpoint is registered.

The handler parses the incoming feedback payload.

The feedback data (query, response, feedback value) is logged to a structured log file (e.g., feedback.log) or a separate SQLite table.

The endpoint returns a 200 OK response to Teams.

Dependencies: P4-E4.S3.T1.

Tech Stack: Go, gin-gonic/gin, go.uber.org/zap.

Testing Tasks for Phase 4

Task P4-E4.T1: End-to-End Demo Scenario Test 1 (AWS Migration)

Description: Perform a full, manual E2E test of the first core demo scenario by sending a message from a real Teams client.

Acceptance Criteria:

All services are running via docker-compose up.

A message @SA-Assistant Generate a high-level lift-and-shift plan... is sent in the Teams channel.

Within 30 seconds, a rich Adaptive Card is posted in response.

The card contains a relevant text plan, a valid architecture diagram image, AWS CLI code snippets, and correct source citations.

Dependencies: All tasks in all four phases.

Tech Stack: Microsoft Teams Client, Docker Compose.

Task P4-E4.T2: End-to-End Demo Scenario Tests 2, 3, & 4

Description: Perform full, manual E2E tests for the remaining three demo scenarios (Azure Hybrid, Azure DR, and Security/Compliance).

Acceptance Criteria:

For each of the three remaining demo prompts, an appropriate and accurate Adaptive Card is returned.

The Azure Hybrid card shows an ExpressRoute diagram.

The Azure DR card provides a clear plan referencing Azure Site Recovery.

The Security/Compliance card is formatted as a checklist.

Dependencies: P4-E4.T1.

Tech Stack: Microsoft Teams Client, Docker Compose.
