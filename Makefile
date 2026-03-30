.PHONY: help build test test-integration test-coverage lint govulncheck run \
       container-build container-up container-down container-logs \
       ingest schema validate eval eval-stability download-model embed-server prereqs clean

# Auto-detect container engine: prefer podman, fall back to docker.
ENGINE ?= $(shell command -v podman >/dev/null 2>&1 && echo podman || echo docker)
COMPOSE = $(ENGINE) compose

# Default: print available targets.
help:
	@echo "Available targets:"
	@echo "  build            Build the server binary to bin/mcp-server"
	@echo "  test             Run unit tests (short mode)"
	@echo "  test-integration Run all tests including integration (requires TEST_DATABASE_URL)"
	@echo "  test-coverage    Run tests with coverage report"
	@echo "  lint             Run golangci-lint"
	@echo "  govulncheck      Run govulncheck for known vulnerabilities"
	@echo "  run              Run the server (serve subcommand)"
	@echo "  container-build  Build container image"
	@echo "  container-up     Start containers via compose"
	@echo "  container-down   Stop containers via compose"
	@echo "  container-logs   Tail container logs"
	@echo "  ingest           Ingest documents (set DIR=path)"
	@echo "  schema           Run schema migrations"
	@echo "  validate         Validate configuration"
	@echo "  eval             Run evaluation script"
	@echo "  eval-stability   Run stability evaluation script"
	@echo "  download-model   Download embedding model"
	@echo "  embed-server     Start llama-server for embeddings"
	@echo "  prereqs          Print prerequisite install instructions"
	@echo "  clean            Remove build artifacts and coverage files"

build:
	go build -o bin/mcp-server ./cmd/server/

test:
	go test ./... -short -count=1

test-integration:
	go test ./... -count=1

test-coverage:
	go test ./... -coverprofile=coverage.out -short && go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

govulncheck:
	govulncheck ./...

run:
	go run ./cmd/server/ serve

container-build:
	$(ENGINE) build -t mcp-server .

container-up:
	$(COMPOSE) up -d

container-down:
	$(COMPOSE) down

container-logs:
	$(COMPOSE) logs -f

ingest:
	go run ./cmd/server/ ingest --dir $(DIR)

schema:
	go run ./cmd/server/ schema

validate:
	go run ./cmd/server/ validate

eval:
	./scripts/eval.sh

eval-stability:
	./scripts/eval-stability.sh

# Embedding model defaults
EMBED_MODEL ?= models/nomic-embed-text-v1.5.Q8_0.gguf
EMBED_PORT  ?= 8079
EMBED_HOST  ?= 127.0.0.1
EMBED_GPU   ?= -1
EMBED_BATCH ?= 2048

download-model:
	./scripts/download-model.sh $(MODEL_REPO) $(MODEL_FILE)

embed-server:
	@command -v llama-server >/dev/null 2>&1 || { echo "llama-server not found in PATH. See docs/installing-llama-server.md"; exit 1; }
	@test -f $(EMBED_MODEL) || { echo "Model not found: $(EMBED_MODEL). Run 'make download-model' first."; exit 1; }
	llama-server \
		--model $(EMBED_MODEL) \
		--embedding \
		--port $(EMBED_PORT) \
		--host $(EMBED_HOST) \
		--n-gpu-layers $(EMBED_GPU) \
		--batch-size $(EMBED_BATCH) \
		--ubatch-size $(EMBED_BATCH)

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

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
