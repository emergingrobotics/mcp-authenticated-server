# Stage 1: Build the Go binary
FROM golang:1.23 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/mcp-server ./cmd/server

# Stage 2: Build llama.cpp (placeholder)
# NOTE: llama.cpp submodule must be initialized before this stage will work.
# Run: git submodule update --init --recursive
FROM debian:bookworm-slim AS llama

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential cmake && \
    rm -rf /var/lib/apt/lists/*

# Placeholder: uncomment when llama.cpp submodule is present
# WORKDIR /llama
# COPY vendor/llama.cpp /llama
# RUN cmake -B build -DLLAMA_STATIC=ON && cmake --build build --target llama-server -j$(nproc)

# Stage 3: Final runtime image
FROM debian:bookworm-slim@sha256:ca3372ce30b03a591ec573ea975ad8b0ecaf0eb17a354416741f8001bbcae233

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# SEC-11: Run as non-root user
RUN groupadd --gid 1000 appuser && \
    useradd --uid 1000 --gid appuser --shell /bin/bash --create-home appuser

WORKDIR /app

COPY --from=builder /out/mcp-server /app/mcp-server
# Uncomment when llama.cpp build is active:
# COPY --from=llama /llama/build/bin/llama-server /app/llama-server

COPY scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh /app/mcp-server

RUN mkdir -p /app/models && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
