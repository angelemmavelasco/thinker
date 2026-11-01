# llm for entities extraction
# curl -L -O https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf

# llm for embedding
# curl -L -o ./models/bge-m3-q4_k_m.gguf \
  https://huggingface.co/ggml-org/bge-m3-GGUF/resolve/main/bge-m3-q4_k_m.gguf


SHELL := /bin/bash
MODELS_DIR := ./models

# QWEN 2.5 3B
LLM_FILE := $(MODELS_DIR)/qwen2.5-3b-instruct-q4_k_m.gguf
LLM_URL := https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf

# BGE M3
EMBED_FILE := $(MODELS_DIR)/bge-small-en-v1.5-q4_k_m.gguf
EMBED_URL := https://huggingface.co/CompendiumLabs/bge-small-en-v1.5-gguf/resolve/main/bge-small-en-v1.5-q4_k_m.gguf

.PHONY: setup download-models run-llm run-embeddings up down

setup: download-models
	@echo "-> Validating Go dependencies"
	go mod download

download-models:
	@mkdir -p $(MODELS_DIR)
	@if [ ! -f "$(LLM_FILE)" ]; then \
		echo "-> Downloading Qwen 2.5 3B GGUF (~2.0 GB)..."; \
		curl -L -o "$(LLM_FILE)" "$(LLM_URL)"; \
	else \
		echo "-> Qwen is already exists in the disk."; \
	fi
	@if [ ! -f "$(EMBED_FILE)" ]; then \
		echo "-> Downloading Embedding model GGUF (~130 MB)..."; \
		curl -L -o "$(EMBED_FILE)" "$(EMBED_URL)"; \
	else \
		echo "-> Embedding model already exists in the disk."; \
	fi


# run llm at port 8081
run-llm:
	llama-server -m $(LLM_FILE) --port 8081 -c 4096 --threads 4

# run embedding at port 8082
run-embeddings:
	llama-server -m $(EMBED_FILE) --port 8082 -c 512 --threads 2 --embedding

# up container
up: download-models
	docker compose up -d

## stop all containers
down:
	docker compose down