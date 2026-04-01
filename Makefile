.PHONY: help build test test-integration test-coverage lint govulncheck run \
       container-build container-up container-down container-logs \
       ingest ingest-add validate eval eval-stability \
       prereqs download-models embed-server reranker-server run-inference-servers stop-inference-servers clean

# Load environment from .envrc if present (strip 'export ' prefix for Make compatibility).
ifneq (,$(wildcard .envrc))
include .env.mk
.env.mk: .envrc
	@sed 's/^export //' .envrc > .env.mk
endif
export

# Auto-detect container engine: prefer podman, fall back to docker.
ENGINE ?= $(shell command -v podman >/dev/null 2>&1 && echo podman || echo docker)
COMPOSE = $(ENGINE) compose
CONFIG  ?= config.toml

# Default: print available targets.
help:
	@echo "Available targets:"
	@echo "  build            Build all binaries to bin/"
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
	@echo "  ingest           Ingest documents with drop (DIR=path, or uses config default)"
	@echo "  ingest-add       Ingest documents with upsert, no drop (DIR=path, or uses config default)"

	@echo "  validate         Validate configuration"
	@echo "  eval             Run evaluation script (set EVAL_FILE=path)"
	@echo "  eval-stability   Run stability evaluation script (set EVAL_FILE=path)"
	@echo "  clean            Remove build artifacts and coverage files"
	@echo ""
	@echo "Setup targets (from installers/):"
	@echo "  prereqs          Print prerequisite install instructions"
	@echo "  download-models  Download all GGUF models listed in config.toml"
	@echo "  embed-server     Start llama-server for embeddings (foreground)"
	@echo "  reranker-server  Start llama-server for reranking (foreground)"
	@echo "  run-inference-servers   Start embedding and reranker servers in the background"
	@echo "  stop-inference-servers  Stop background inference servers"

build:
	go build -o bin/mcp-server ./cmd/server/
	go build -o bin/mcp-ingest ./cmd/ingest/

	go build -o bin/mcp-validate ./cmd/validate/

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
	bin/mcp-server

container-build:
	$(ENGINE) build -t mcp-server .

container-up:
	$(COMPOSE) up -d

container-down:
	$(COMPOSE) down

container-logs:
	$(COMPOSE) logs -f

ingest:
ifdef DIR
	bin/mcp-ingest --drop --verbose --dir $(DIR)
else
	bin/mcp-ingest --drop --verbose
endif

ingest-add:
ifdef DIR
	bin/mcp-ingest --verbose --dir $(DIR)
else
	bin/mcp-ingest --verbose
endif

validate:
	bin/mcp-validate

eval:
ifndef EVAL_FILE
	$(error EVAL_FILE is required. Usage: make eval EVAL_FILE=data/evals/evals.json)
endif
	./scripts/eval.sh $(EVAL_FILE)

eval-stability:
ifndef EVAL_FILE
	$(error EVAL_FILE is required. Usage: make eval-stability EVAL_FILE=data/evals/evals.json)
endif
	./scripts/eval-stability.sh $(EVAL_FILE)

# Setup targets -- delegated to installers/Makefile
prereqs download-models embed-server reranker-server run-inference-servers stop-inference-servers:
	$(MAKE) -C installers $@

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html .env.mk
