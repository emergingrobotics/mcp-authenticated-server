# Installing llama-server

llama-server is the HTTP inference server from the [llama.cpp](https://github.com/ggerganov/llama.cpp) project. It exposes an OpenAI-compatible `/v1/embeddings` endpoint that the MCP server uses for vector search and document ingestion.

Embedding inference is compute-intensive. For acceptable performance, run llama-server on bare metal with GPU access. Running it inside a container without GPU passthrough will be impractically slow for anything beyond small test corpora.

## Install from pre-built release (recommended)

Pre-built binaries are published on the [llama.cpp releases page](https://github.com/ggerganov/llama.cpp/releases).

### Linux

```bash
# Find the latest release at https://github.com/ggerganov/llama.cpp/releases
# Download the appropriate archive for your system. Examples:

# CPU only
curl -L -o llama.zip https://github.com/ggerganov/llama.cpp/releases/latest/download/llama-server-linux-x64.zip

# NVIDIA GPU (CUDA)
curl -L -o llama.zip https://github.com/ggerganov/llama.cpp/releases/latest/download/llama-server-linux-x64-cuda.zip

unzip llama.zip
sudo mv llama-server /usr/local/bin/
llama-server --version
```

### macOS

```bash
brew install llama.cpp
```

This installs `llama-server` into your PATH. Apple Silicon Macs use Metal acceleration automatically.

### Windows

Download the Windows release from the [releases page](https://github.com/ggerganov/llama.cpp/releases) and add the directory to your PATH.

## Build from source

If pre-built binaries are not available for your platform or you need custom build options:

```bash
git clone https://github.com/ggerganov/llama.cpp.git
cd llama.cpp

# CPU only
cmake -B build
cmake --build build --target llama-server -j$(nproc)

# NVIDIA GPU (CUDA)
cmake -B build -DGGML_CUDA=ON
cmake --build build --target llama-server -j$(nproc)

# AMD GPU (ROCm)
cmake -B build -DGGML_HIP=ON
cmake --build build --target llama-server -j$(nproc)

# Apple Silicon (Metal -- enabled by default on macOS)
cmake -B build
cmake --build build --target llama-server -j$(sysctl -n hw.ncpu)

# Install to PATH
sudo cp build/bin/llama-server /usr/local/bin/
```

## Verify installation

```bash
llama-server --version
```

If the command is not found, ensure the binary is in a directory on your PATH.

## Download an embedding model

llama-server requires a GGUF model file. The default model for this project is [nomic-embed-text-v1.5](https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF):

```bash
make download-model
```

This downloads `nomic-embed-text-v1.5.Q8_0.gguf` (~550 MB) to the `models/` directory.

For a smaller download at slightly reduced quality:

```bash
make download-model MODEL_FILE=nomic-embed-text-v1.5.Q4_K_M.gguf
```

## Run the embedding server

```bash
make embed-server
```

This starts llama-server with the default model on port 8079. Override with:

```bash
make embed-server EMBED_MODEL=models/other-model.gguf EMBED_PORT=9090
```

To run manually:

```bash
llama-server \
  --model models/nomic-embed-text-v1.5.Q8_0.gguf \
  --embedding \
  --port 8079 \
  --host 127.0.0.1 \
  --n-gpu-layers -1
```

Key flags:
- `--embedding` enables the `/v1/embeddings` endpoint (required)
- `--n-gpu-layers -1` offloads all layers to GPU (`0` for CPU only)
- `--host 127.0.0.1` binds to loopback only (use `0.0.0.0` if the MCP server runs on a different host)
- `--port 8079` must match `embed.host` in `config.toml`

## Verify the embedding server is running

```bash
curl http://localhost:8079/health
```

Expected: `{"status":"ok"}`.

Test an embedding request:

```bash
curl -s http://localhost:8079/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model":"nomic-embed-text","input":["hello world"]}' | head -c 200
```

Expected: a JSON response with an `embedding` array.

## GPU troubleshooting

**NVIDIA:** Ensure CUDA drivers are installed (`nvidia-smi` should show your GPU). If llama-server was built without CUDA, rebuild with `-DGGML_CUDA=ON` or download the CUDA release binary.

**AMD:** Ensure ROCm is installed. Rebuild with `-DGGML_HIP=ON`.

**Apple Silicon:** Metal is enabled by default. No extra configuration needed.

**CPU fallback:** If no GPU is available, llama-server runs on CPU. Embedding will be slower but functional. For test/development workloads this is acceptable.
