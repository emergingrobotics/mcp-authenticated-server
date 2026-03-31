.PHONY: help prereqs download-models embed-server reranker-server run-inference-servers

ROOT_DIR := $(dir $(MAKEFILE_LIST))..
CONFIG   ?= $(ROOT_DIR)/config.toml

# Embedding model defaults
EMBED_MODEL ?= $(ROOT_DIR)/models/nomic-embed-text-v1.5.Q8_0.gguf
EMBED_PORT  ?= 8079
EMBED_HOST  ?= 0.0.0.0
EMBED_GPU   ?= -1
EMBED_BATCH ?= 2048

# Reranker model defaults
RERANKER_MODEL ?= $(ROOT_DIR)/models/bge-reranker-v2-m3-Q8_0.gguf
RERANKER_PORT  ?= 8081
RERANKER_HOST  ?= 0.0.0.0
RERANKER_GPU   ?= -1

help:
	@echo "Available targets (installers):"
	@echo "  prereqs          Print prerequisite install instructions"
	@echo "  download-models  Download all GGUF models listed in config.toml"
	@echo "  embed-server     Start llama-server for embeddings (foreground)"
	@echo "  reranker-server  Start llama-server for reranking (foreground)"
	@echo "  run-inference-servers  Start embedding and reranker servers in the background"

prereqs:
	@echo "Install a container engine (podman preferred):"
	@echo "  Fedora/RHEL: sudo dnf install podman podman-compose"
	@echo "  Ubuntu/Debian: sudo apt install podman podman-compose"
	@echo "  macOS: brew install podman podman-compose"
	@echo "  Alternative: install Docker Desktop from https://www.docker.com"
	@echo ""
	@echo "Install Go tools:"
	@echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
	@echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"

download-models:
	$(ROOT_DIR)/scripts/download-models.sh $(CONFIG)

embed-server:
	@command -v llama-server >/dev/null 2>&1 || { echo "llama-server not found in PATH. See docs/installing-llama-server.md"; exit 1; }
	@test -f $(EMBED_MODEL) || { echo "Model not found: $(EMBED_MODEL). Run 'make download-models' first."; exit 1; }
	llama-server \
		--model $(EMBED_MODEL) \
		--embedding \
		--port $(EMBED_PORT) \
		--host $(EMBED_HOST) \
		--n-gpu-layers $(EMBED_GPU) \
		--batch-size $(EMBED_BATCH) \
		--ubatch-size $(EMBED_BATCH)

reranker-server:
	@command -v llama-server >/dev/null 2>&1 || { echo "llama-server not found in PATH. See docs/installing-llama-server.md"; exit 1; }
	@test -f $(RERANKER_MODEL) || { echo "Model not found: $(RERANKER_MODEL). Run 'make download-models' first."; exit 1; }
	llama-server \
		--model $(RERANKER_MODEL) \
		--reranking \
		--port $(RERANKER_PORT) \
		--host $(RERANKER_HOST) \
		--n-gpu-layers $(RERANKER_GPU) \
		--ctx-size 8192

run-inference-servers:
	@command -v llama-server >/dev/null 2>&1 || { echo "llama-server not found in PATH. See docs/installing-llama-server.md"; exit 1; }
	@test -f $(EMBED_MODEL) || { echo "Model not found: $(EMBED_MODEL). Run 'make download-models' first."; exit 1; }
	@test -f $(RERANKER_MODEL) || { echo "Model not found: $(RERANKER_MODEL). Run 'make download-models' first."; exit 1; }
	@echo "starting embed server on port $(EMBED_PORT)..."
	@nohup llama-server \
		--model $(EMBED_MODEL) \
		--embedding \
		--port $(EMBED_PORT) \
		--host $(EMBED_HOST) \
		--n-gpu-layers $(EMBED_GPU) \
		--batch-size $(EMBED_BATCH) \
		--ubatch-size $(EMBED_BATCH) \
		> /tmp/llama-embed.log 2>&1 & \
	echo "  PID: $$!  log: /tmp/llama-embed.log"
	@echo "starting reranker server on port $(RERANKER_PORT)..."
	@nohup llama-server \
		--model $(RERANKER_MODEL) \
		--reranking \
		--port $(RERANKER_PORT) \
		--host $(RERANKER_HOST) \
		--n-gpu-layers $(RERANKER_GPU) \
		--ctx-size 8192 \
		> /tmp/llama-reranker.log 2>&1 & \
	echo "  PID: $$!  log: /tmp/llama-reranker.log"
