# Stage 1: Build the Go binary
FROM golang:1.25 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/mcp-server ./cmd/server

# Stage 2: Final runtime image
# NOTE: The embedding server (e.g., llama-server) is NOT included in this image.
# It must be run as a separate external process with bare-metal GPU access.
# See README.md for setup instructions.
FROM docker.io/library/debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# SEC-11: Run as non-root user
RUN groupadd --gid 1000 appuser && \
    useradd --uid 1000 --gid appuser --shell /bin/bash --create-home appuser

WORKDIR /app

COPY --from=builder /out/mcp-server /app/mcp-server
COPY scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh /app/mcp-server

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 9090

ENTRYPOINT ["/app/entrypoint.sh"]
