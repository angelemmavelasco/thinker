# Thinker

Thinker is a backend system for organizing ideas as an associative knowledge graph. Users submit notes or raw thoughts, then the system extracts concepts and computes semantic relationships between them. Ideas are represented as graph nodes; semantically related ideas are connected by undirected edges whose weights express their proximity.

The repository currently provides the Go HTTP application, PostgreSQL access, sqlc-generated queries, and server-side HTML views. The graph-processing pipeline is an architectural target and is not yet implemented in the current codebase.

## Overview

Traditional note systems often rely on folders or exact keyword search. thinker uses a semantic graph so that an idea can belong to several conceptual areas without maintaining a rigid hierarchy.

A future graph record should include:

- ideas as graph nodes;
- normalized keywords and entities as supporting concepts;
- vector embeddings for ideas and concepts;
- undirected weighted edges between closely related ideas or concepts;
- edge weights derived from explicit overlap and vector similarity.

The current application manages the foundational `users` and `ideas` PostgreSQL tables, exposes JSON and server-rendered views, and provides named route resolution and generated database access.

## Architecture and Data Flow

The target idea-processing flow is:

```text
Raw idea text
    |
    v
Input validation and persistence
    |
    v
LLM normalization and keyword/entity extraction
    |
    v
Embedding generation
    |
    v
Similarity and shared-concept calculation
    |
    v
Undirected weighted graph edge generation
    |
    v
Graph query, traversal, and ranked retrieval
```

The responsibilities are:

1. **Submission:** The HTTP layer validates and persists a user's idea.
2. **Normalization:** An LLM converts the raw text into a consistent representation and extracts relevant keywords and entities.
3. **Embedding:** The normalized text and extracted concepts are converted to vector representations.
4. **Comparison:** Cosine similarity and shared-concept relationships determine the semantic distance between nodes.
5. **Edge generation:** Related ideas are connected with an undirected edge. The weight combines or derives from mutual relevance and semantic proximity.
6. **Retrieval:** Graph traversal and similarity search can expose related ideas even when their wording does not match exactly.

### Runtime request flow

The current HTTP request path is:

```text
main.go
  -> config.LoadConfig
  -> database.NewConnector
  -> database.Queries
  -> routes.SetUpRouter
  -> domain handler
  -> sqlc query or HTML template
```

The application initializes its PostgreSQL connection once and passes generated query methods to handlers through `database.Queries`. HTML requests are rendered with layouts and views assembled by `multitemplate`; JSON endpoints use Gin's response renderer directly.

## Tech Stack

- **Go 1.26.5:** Application runtime and server.
- **Gin:** HTTP routing and request handling.
- **PostgreSQL 16:** Relational persistence.
- **pgvector:** Intended vector-storage and similarity-search extension for embeddings and graph traversal. It is not enabled in the current Docker Compose service.
- **sqlc:** Compile-time generation of type-safe Go database methods from SQL.
- **pgx/v5:** PostgreSQL driver and connection pool API.
- **html/template:** Server-side template parsing and rendering.
- **gin-contrib/multitemplate:** Named template sets composed from layouts and views.
- **Tailwind CSS:** Browser-side page styling through the base template.
- **Docker Compose:** Local PostgreSQL process and persistent volume management.
- **godotenv:** Loading local environment variables from `.env`.

## Project Structure

```text
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

### Package boundaries

- `internal/routes` registers the complete routes and connects them to handlers.
- `internal/urls` is the source of truth for named routes. It exposes both direct lookup and template-friendly reverse lookup without introducing a config or handler dependency back into templates.
- `internal/handlers` translates HTTP requests and database or rendering failures into Gin responses. Domain handlers remain separated from route registration.
- `internal/database` contains the SQL schema and queries, sqlc-generated models and methods, and PostgreSQL connection setup.
- `internal/config` loads environment configuration and combines each HTML view with base layouts.
- `internal/models` holds domain-level types that are not exclusively generated database models.

The database is initialized with SQL rather than by runtime ORM migrations. sqlc reads `internal/database/schema.sql` for generated types and `internal/database/query.sql` for compile-time query signatures.

## Routes

| Route | Handler purpose | Response |
| --- | --- | --- |
| `GET /` | Application health-style message | JSON |
| `GET /api/users` | List users | JSON |
| `GET /web/users` | List users | HTML |
| `GET /web/ideas` | List ideas | HTML |

Routes are registered by name through `internal/urls/urls.go`. Template views call `{{ url "route:name" }}` for reverse lookup, while Go code calls `urls.Get` or `urls.URLFor`.

## Getting Started

### Prerequisites

Install:

- Go 1.26.5 or later
- Docker Engine and Docker Compose
- sqlc 1.31.1 or a compatible later release
- A PostgreSQL client such as `psql` for schema initialization

### Environment configuration

Create a `.env` file in the repository root. The application expects these variables:

```dotenv
DB_NAME=thinker
DB_USERNAME=postgres
DB_PASSWORD=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
CONN_STRING=postgresql://postgres:postgres@127.0.0.1:5432/thinker
LLM_API_KEY=replace-with-a-local-development-key
```

`docker-compose.yaml` uses `DB_NAME`, `DB_USERNAME`, `DB_PASSWORD`, and `DB_PORT`. The application currently connects through `CONN_STRING`. Keep `CONN_STRING` aligned with the Compose variables.

The repository does not ship a committed environment file. Do not commit credentials or API keys.

### Start PostgreSQL

```bash
docker compose up -d db
```

The Compose service exposes PostgreSQL on `DB_PORT` and persists data in the `postgres-data` Docker volume.

### Initialize the database schema

The current Compose service starts the standard PostgreSQL image and does not mount or execute `internal/database/schema.sql` automatically. Apply the schema explicitly:

```bash
psql "$CONN_STRING" -f internal/database/schema.sql
```

The shell can read `CONN_STRING` from `.env` only if the environment file has been exported. An equivalent explicit command is:

```bash
psql "postgresql://postgres:postgres@127.0.0.1:5432/thinker" \
  -f internal/database/schema.sql
```

The existing schema creates `users` and `ideas`. It does not yet define vector columns, graph edge tables, or the pgvector extension.

### Regenerate database code

After changing `internal/database/schema.sql` or `internal/database/query.sql`, regenerate the sqlc output:

```bash
sqlc generate
```

Generated files are written to `internal/database` according to `sqlc.yaml`. Do not edit those files manually.

### Run the application

```bash
go run main.go
```

The server listens on port `8080` by default. Verify the available surfaces at:

- `http://localhost:8080/`
- `http://localhost:8080/api/users`
- `http://localhost:8080/web/users`
- `http://localhost:8080/web/ideas`

### Run checks

```bash
go test ./...
go vet ./...
```

## Current Status

The implemented foundation includes:

- Gin HTTP routing;
- PostgreSQL connection setup;
- sqlc-generated user and idea queries;
- JSON user listing;
- server-rendered user and idea views;
- named routes and reverse URL lookup;
- local PostgreSQL and persistent-volume setup;
- focused tests for route and template rendering behavior.

The LLM extraction and graph-processing pipeline is not yet implemented.

## Next Milestones

- Add authentication, password handling, authorization, and role-based access control.
- Add explicit pgvector support through a compatible PostgreSQL image and schema migration.
- Store embeddings for ideas and extracted concepts.
- Integrate an LLM processor behind a bounded worker pool for asynchronous normalization and extraction.
- Persist normalized keywords and entities with provenance to source ideas.
- Generate canonical undirected graph edges and store weighted similarity values.
- Add cosine-similarity searches and graph traversals for related ideas.
- Move schema application into reproducible migrations and make startup checks deterministic.
- Add API-level validation, pagination, transaction handling, and structured error responses.
- Expand tests across database, handlers, routing, and rendered templates.
