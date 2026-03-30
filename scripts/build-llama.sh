#!/usr/bin/env bash
set -euo pipefail

# Build llama.cpp from source (or submodule).

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/.."
LLAMA_DIR="${LLAMA_DIR:-${PROJECT_ROOT}/vendor/llama.cpp}"
BUILD_DIR="${LLAMA_DIR}/build"

if [[ ! -d "${LLAMA_DIR}" ]]; then
    echo "llama.cpp source not found at ${LLAMA_DIR}" >&2
    echo "Initialize the submodule: git submodule update --init --recursive" >&2
    exit 1
fi

CMAKE_ARGS=()

# Detect GPU and set build flags
if "${SCRIPT_DIR}/detect-gpu.sh" --check cuda 2>/dev/null; then
    echo "CUDA detected, enabling GPU support..."
    CMAKE_ARGS+=("-DGGML_CUDA=ON")
elif "${SCRIPT_DIR}/detect-gpu.sh" --check rocm 2>/dev/null; then
    echo "ROCm detected, enabling GPU support..."
    CMAKE_ARGS+=("-DGGML_HIP=ON")
elif "${SCRIPT_DIR}/detect-gpu.sh" --check metal 2>/dev/null; then
    echo "Metal detected, enabling GPU support..."
    CMAKE_ARGS+=("-DGGML_METAL=ON")
else
    echo "No GPU detected, building CPU-only..."
fi

echo "Building llama.cpp..."
cmake -S "${LLAMA_DIR}" -B "${BUILD_DIR}" \
    -DLLAMA_STATIC=ON \
    "${CMAKE_ARGS[@]+"${CMAKE_ARGS[@]}"}"

cmake --build "${BUILD_DIR}" --target llama-server -j"$(nproc)"

echo "Build complete: ${BUILD_DIR}/bin/llama-server"
