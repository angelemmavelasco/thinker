# Thinker

Thinker is an associative knowledge graph and collaborative LLM wiki built for teams that need a shared digital brain. Instead of asking people to manually categorize documents into rigid folders or write bidirectional links by hand, the platform ingests raw notes and thoughts, extracts core concepts with a local language model, and compiles them into a living graph structure inspired by the LLM wiki pattern proposed by Andrej Karpathy.

By pairing a linked interface (graph that looks like a neural network) with role based access control, the system lets organizations maintain an autonomous second brain where knowledge compounds naturally as users write.

The codebase currently provides the core Go web service, PostgreSQL storage, sqlc generated database access, and server rendered HTML templates. The asynchronous entity extraction worker and graph expansion pipeline represent the immediate architectural roadmap.

## Overview

Most note taking platforms rely on folder hierarchies or basic keyword search. When teams scale, these structures quickly break because people organize information differently and maintenance becomes a burden. Standard retrieval augmented generation also falls short over time because it answers questions from scratch by slicing documents into arbitrary chunks without preserving overall context or persistent synthesis.

Thinker treats ideas as an active, compounding artifact:

* Every submitted note or thought becomes an idea node inside a semantic graph.

* A local language model compiles incoming content, extracting entities, identifying topics, and noting connections or contradictions.

* Explicit relationships create edges between ideas, allowing multi hop discovery without relying solely on brittle text matching.

* Role based access control ensures users only discover and traverse knowledge permitted for their organizational role.

* The search experience operates independently of manual tagging by traversing the graph structure and combining text matching with graph expansion.

The present build contains the foundation for user and idea records, named route handling, database access layer, and server rendered template views.

## Architecture and Data Flow

The target processing lifecycle for new ideas follows a clear asynchronous pipeline:

```
User submits idea through web interface
    |
    v
Gin handler validates and persists idea to PostgreSQL
    |
    v
Background worker claims task via internal queue
    |
    v
Local model extracts entities, summary, and semantic tags
    |
    v
Vector embedding generated and stored via pgvector
    |
    v
Graph relations and bidirectional links created
    |
    v
User traverses ideas via graph view or associative search

```

Pipeline responsibilities:

1. Submission: The Gin layer stores the raw idea immediately and returns an acknowledgment to the user.

2. Background extraction: A Go worker pool feeds the text to a local model such as Qwen 2.5 3B running via llama.cpp or an OpenAI compatible local runner, obtaining concise summaries and concept tags without introducing external runtimes.

3. Vector persistence: Embeddings are generated and persisted into PostgreSQL with pgvector for fast seed candidate discovery.

4. Graph compilation: New relationships link the note to existing ideas, maintaining an evolving web of knowledge.

5. Associative retrieval: Queries use text matching alongside graph traversal algorithms to find relevant context across multiple degrees of separation.

### Runtime Request Flow

The current request path inside the application:

```
main.go
  -> config.LoadConfig
  -> database.NewConnector
  -> database.Queries
  -> routes.SetUpRouter
  -> domain handler
  -> sqlc query or HTML template

```

The server initializes its database pool once at boot and supplies the compiled sqlc queries directly to handlers. Responses return either raw JSON for API consumers or HTML views built with standard templates and multitemplate layouts.

## Tech Stack

* Go 1.26.5: Core backend service and concurrency orchestration.

* Gin: HTTP routing, middleware, and request dispatching.

* PostgreSQL 16: Relational persistence for users, ideas, and graph edges.

* pgvector: Vector similarity indexing for semantic seed retrieval.

* Qwen 2.5 3B Instruct GGUF: Default local inference target running on llama.cpp or compatible daemon.

* sqlc: Compile time type safe database client generated from SQL queries.

* pgx v5: PostgreSQL driver and connection pool.

* html/template and multitemplate: Server rendered UI layouts.

* Tailwind CSS: Interface styling.

* Docker Compose: Local container environment for development services.

* godotenv: Environment variable loader for local workflows.

## Project Structure

```
.
├── internal/
│   ├── config/          # Environment loading and template renderer setup
│   ├── database/        # SQL schema, queries, sqlc output, and connection setup
│   ├── handlers/        # User and idea HTTP handlers
│   ├── models/          # Domain-level models and auxiliary structs
│   ├── routes/          # Gin router construction and endpoint registration
│   └── urls/            # Named-route catalog and reverse URL lookup
├── templates/
│   ├── includes/        # Shared template fragments
│   ├── layouts/         # Base HTML shells
│   └── views/           # Server-rendered idea and user pages
├── docker-compose.yaml  # Local PostgreSQL service definition
├── sqlc.yaml            # sqlc schema and query generation configuration
├── go.mod               # Go module and dependency declarations
└── main.go              # Application entry point

```

<!-- ### Package Boundaries

* internal/routes registers endpoints and maps them to their respective domain handlers.

* internal/urls provides reverse route resolution for templates and application logic, avoiding hardcoded endpoint paths.

* internal/handlers manages request parsing, delegates database work, and selects the rendering mechanism.

* internal/database houses the SQL schemas, sqlc definitions, and connection lifecycle.

* internal/config loads configuration settings and builds the multitemplate view registry.

* internal/models defines application entities outside the raw generated database rows.

The database relies on explicit SQL statements rather than an ORM. The build tooling generates types from internal/database/schema.sql and method signatures from internal/database/query.sql. -->

<!-- ## Routes

| Route | Handler purpose | Response | 
| ----- | ----- | ----- | 
| GET / | Application health status | JSON | 
| GET /api/users | Retrieve registered users | JSON | 
| GET /web/users | User management view | HTML | 
| GET /web/ideas | Idea listing and dashboard | HTML |  -->
<!-- 
Routes are registered using internal/urls. Templates use the url helper for reverse resolution, keeping hyperlinks maintainable across views. -->

## Getting Started

### Prerequisites

Ensure the following tools are installed:

* Go 1.26.5 or later

* Docker Engine with Docker Compose

* sqlc 1.31.1 or later

* PostgreSQL client utilities such as psql

* A local inference runner such as llama.cpp or Ollama when testing the model integration

### Environment Configuration

Create a .env file in the root directory:

```
DB_NAME=thinker
DB_USERNAME=postgres
DB_PASSWORD=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
CONN_STRING=postgresql://postgres:postgres@127.0.0.1:5432/thinker
LLM_BASE_URL=http://127.0.0.1:8081/v1
LLM_MODEL=qwen2.5-3b-instruct

```

Keep CONN_STRING consistent with the credentials set in docker-compose.yaml. Never commit personal environment files containing real secrets.

### Start the Database

```
docker compose up -d db

```

The database container runs on the configured port and stores data in a dedicated volume.

### Apply the Database Schema

Run the initial schema script against the running database instance:

```
psql postgresql://postgres:postgres@127.0.0.1:5432/thinker -f internal/database/schema.sql

```

The baseline schema provides the users and ideas tables. Tables for graph edges, permissions, and vector columns are applied as the graph engine milestones land.

### Generate Database Code

Whenever modifying internal/database/schema.sql or internal/database/query.sql, update the sqlc output:

```
sqlc generate

```

Generated files belong in internal/database and should not be modified manually.

### Run the Application

```
go run main.go

```

The web server starts on port 8080. Explore the active endpoints:

* http://localhost:8080/

* http://localhost:8080/api/users

* http://localhost:8080/web/users

* http://localhost:8080/web/ideas

### Run Test Suites

```
go test ./...
go vet ./...

```

## Current Status

The system currently includes:

* Gin HTTP setup and graceful initialization.

* Single pool PostgreSQL connector with sqlc integration.

* User listing through both JSON APIs and HTML pages.

* Basic idea listing views.

* Centralized route directory with reverse template resolution.

* Base layout composition using Tailwind CSS.

* Targeted routing and rendering unit tests.

## Next Milestones

* Implement authentication, session handling, and role based access control on notes and graph queries.

* Upgrade the PostgreSQL container setup to support pgvector natively.

* Set up a bounded Go worker pool for asynchronous note ingestion and relationship building.

* Connect the worker pool to a local Qwen 2.5 3B instance for concept extraction and summary generation.

* Create graph edge tables to store typed links and mutual relevance scores.

* Implement graph traversal queries with role aware filters using recursive SQL queries.

* Build an interactive visual graph explorer view in the web interface.