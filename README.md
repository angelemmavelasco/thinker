# Thinker

Thinker is an associative knowledge graph and collaborative LLM wiki built for teams that need a shared digital brain. Instead of asking people to manually categorize documents into rigid folders or write bidirectional links by hand, the platform ingests raw notes and thoughts, extracts core concepts with a local language model, and compiles them into a living graph structure inspired by the LLM wiki pattern proposed by Andrej Karpathy.

By pairing a linked interface (graph that looks like a neural network) with role based access control, the system lets organizations maintain an autonomous second brain where knowledge compounds naturally as users write.

The codebase currently provides the core Go web service, PostgreSQL storage, sqlc generated database access, and server rendered HTML templates. The asynchronous entity extraction worker and graph expansion pipeline represent the immediate architectural roadmap.

## Overview

Most note taking platforms rely on folder hierarchies or basic keyword search. When teams scale, these structures quickly break because people organize information differently and maintenance becomes a burden. Standard retrieval augmented generation also falls short over time because it answers questions from scratch by slicing documents into arbitrary chunks without preserving overall context or persistent synthesis.

Thinker treats ideas as an active, compounding artifact:

- Every submitted note or thought becomes an idea node inside a semantic graph.
- A local language model compiles incoming content, extracting entities, identifying topics, and noting connections or contradictions.
- Explicit relationships create edges between ideas, allowing multi hop discovery without relying solely on brittle text matching.
- Role based access control ensures users only discover and traverse knowledge permitted for their organizational role.
- The search experience operates independently of manual tagging by traversing the graph structure and combining text matching with graph expansion.
The present build contains the foundation for user and idea records, named route handling, database access layer, and server rendered template views.

## Stack
- go 1.26.5
> - [github.com/gin-contrib/multitemplate v1.1.4](https://pkg.go.dev/github.com/gin-contrib/multitemplate@v1.1.4)
> - [github.com/gin-gonic/gin v1.12.0](https://pkg.go.dev/github.com/gin-gonic/gin@v1.12.0)
> - [github.com/jackc/pgx/v5 v5.11.0](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.11.0)
> - [github.com/joho/godotenv v1.5.1](https://pkg.go.dev/github.com/joho/godotenv@v1.5.1)

- LLM
> - [Qwen2.5-3B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF)
> - [MiniMax M3.1 Flash](https://www.minimax.io/blog/minimax-m31f)

- HTML5
- CSS
> - [Tailwind 4](https://tailwindcss.com/docs/installation/using-vite)
- JavaScript

## Architecture and data flow
```mermaid
flowchart LR

    P1 --> P2 --> P3 --> P4 --> P5 --> P6 --> P7 --> P8

    subgraph Col1 [" "]
        direction TB
        P1("1. Ingestion")
        C1_1["Gin HTTP router"]
        C1_2["RBAC middleware"]
        P1 ~~~ C1_1 ~~~ C1_2
    end

    subgraph Col2 [" "]
        direction TB
        P2("2. Validation")
        C2_1["Go input type<br/>validation handler"]
        P2 ~~~ C2_1
    end

    subgraph Col3 [" "]
        direction TB
        P3("3. Preprocessing")
        C3_1["Go chunkenizer"]
        C3_2["Go structs"]
        C3_3["SQLC payload<br/>validation"]
        P3 ~~~ C3_1 ~~~ C3_2 ~~~ C3_3
    end

    subgraph Col4 [" "]
        direction TB
        P4("4. Async dispatch")
        C4_1["Go buffered channel"]
        C4_2["Bounded worker pool"]
        P4 ~~~ C4_1 ~~~ C4_2
    end

    subgraph Col5 [" "]
        direction TB
        P5("5. Semantic<br/>extraction")
        C5_1["llama server"]
        C5_2["Qwen 2.5 3B GGUF"]
        C5_3["Grammar JSON"]
        P5 ~~~ C5_1 ~~~ C5_2 ~~~ C5_3
    end

    subgraph Col6 [" "]
        direction TB
        P6("6. Vector<br/>embedding")
        C6_1["Specialized embedding<br/>model"]
        T6["PGVector coords<br/>storage & indexation"]
        P6 ~~~ C6_1 ~~~ T6
    end

    subgraph Col7 [" "]
        direction TB
        P7("7. Linking")
        C7_1["Postgres KNN search"]
        C7_2["Go overlap concept<br/>linker"]
        C7_3["Go bidirectional<br/>injector"]
        P7 ~~~ C7_1 ~~~ C7_2 ~~~ C7_3
    end

    subgraph Col8 [" "]
        direction TB
        P8("8. Wiki and<br/>consulting")
        C8_1["Go graph traversal<br/>PPR"]
        C8_2["Gin HTML templates"]
        T8["Postgres knowledge<br/>graph"]
        P8 ~~~ C8_1 ~~~ C8_2 ~~~ T8
    end

    classDef blueStep fill:#a8cce8,stroke:#2b4c6f,stroke-width:1.5px,color:#000,rx:8,ry:8;
    classDef whiteBox fill:#ffffff,stroke:#333333,stroke-width:1.5px,color:#000;
    classDef orangeStep fill:#fed8b1,stroke:#333333,stroke-width:1.5px,color:#000,rx:8,ry:8;

    class P1,P2,P3,P4,P5,P6,P7,P8 blueStep;
    class C1_1,C1_2,C2_1,C3_1,C3_2,C3_3,C4_1,C4_2,C5_1,C5_2,C5_3,C6_1,C7_1,C7_2,C7_3,C8_1,C8_2 whiteBox;
    class T6,T8 orangeStep;

    style Col1 fill:none,stroke:none;
    style Col2 fill:none,stroke:none;
    style Col3 fill:none,stroke:none;
    style Col4 fill:none,stroke:none;
    style Col5 fill:none,stroke:none;
    style Col6 fill:none,stroke:none;
    style Col7 fill:none,stroke:none;
    style Col8 fill:none,stroke:none;
```


1. Ingestion
- Gin HTTP router
    > Access to a url path via web app.
- RBAC (Role-based access control) middleware
    > Controls permissions for every idea, keyword, and action according to the user role to enforce enterprise privacy.

2. Validation
- Go input type validation handler
    > Validates that submitted files (.pdf, .csv, .txt, .docx) match allowed extensions and size limits before processing.

3. Preprocessing
- Go chunkenizer
    > Splits long documents into manageable, coherent text segments so the local model can ingest them within its context limits.
- Go structs
    > Type-safe memory structures that hold and transform validated raw data across backend layers.
- SQLC payload validation
    > Validates and coerces incoming request parameters into generated database types to reject malformed data before hitting the database.

4. Async dispatch
- Go buffered channel
    > Queues validated document tasks in memory so the HTTP handler can respond immediately without blocking the client.
- Bounded worker pool
    > Limits the number of concurrent background tasks to prevent local CPU and memory exhaustion during heavy inference.

5. Semantic extraction
- llama server
    > High-performance C++ inference engine that exposes a lightweight local HTTP endpoint for the model without Python overhead.
- Qwen 2.5 3B GGUF
    > Fast, low-footprint local language model that reads the text to extract summaries, entities, and conceptual relations.
- Grammar JSON
    > Forces the model output into an exact schema via GBNF grammars to ensure deterministic parsing in Go.

6. Vector embedding
- specialized embedding model
    > Translates extracted concepts and idea texts into dense numerical vectors that capture geometric semantic meaning.
- PGVector vector cords storage and indexation
    > Stores embeddings in PostgreSQL and accelerates similarity searches through HNSW indexing.

7. Linking
- Postgres knn search
    > Queries the nearest neighbor vectors in the database to identify candidate ideas with high semantic similarity.
- Go overlap concept linker
    > Computes intersections between extracted entity tags to reinforce edges that share common named concepts.
- Go bidirectional injector
    > Creates reciprocal graph edges in the database and inserts bidirectional links into markdown documents.

8. Wiki and consulting
- Go graph traversal PPR
    > Executes Personalized PageRank walks across graph edges to retrieve deep, multi-hop related ideas without flat chunking.
- Gin HTML templates
    > Renders server-side web layouts and interactive graph views directly to the browser.
- Postgres knowledge graph
    > The persistent relational store that preserves all nodes, edges, and permission constraints as a compounding single source of truth.

### Architecture

Thinker is almost enteraly built on Go. This decision was taken due to the quickness and native concurrency support provided by the language.

This allows Thinker be structured in different modules confortably and following SOLID principles with an idiomatic Go approach.

```
└── thinker/
    └── docs/
    └── internal/
        └── config/
        └── database/
        └── handlers/
        └── models/
        └── routes/
        └── urls/
    └── static/
    └── templates/
        └── includes/
        └── layouts/
        └── pages/
        └── views/
    ├── .env
    ├── .gitignore
    ├── docker-compose.yaml
    ├── go.mod
    ├── go.sum
    ├── main.go
    ├── README.md
    ├── README.pdf
    ├── sqlc.yaml
    └── style_schema.md
```

## Current Status

The system currently includes:
- Gin HTTP setup and graceful initialization.
- Single pool PostgreSQL connector with sqlc integration.
- User listing through both JSON APIs and HTML pages.
- Basic idea listing views.
- Centralized route directory with reverse template resolution.
- Base layout composition using Tailwind CSS.
- Targeted routing and rendering unit tests.

## Next Milestones

- Implement authentication, session handling, and role based access control on notes and graph queries.
- Upgrade the PostgreSQL container setup to support pgvector natively.
- Set up a bounded Go worker pool for asynchronous note ingestion and relationship building.
- Connect the worker pool to a local Qwen 2.5 3B instance for concept extraction and summary generation.
- Create graph edge tables to store typed links and mutual relevance scores.
- Implement graph traversal queries with role aware filters using recursive SQL queries.
- Build an interactive visual graph explorer view in the web interface.

## References
1. https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f