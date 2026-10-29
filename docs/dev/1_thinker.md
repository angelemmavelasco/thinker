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