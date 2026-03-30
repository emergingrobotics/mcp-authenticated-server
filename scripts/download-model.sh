#!/usr/bin/env bash
set -euo pipefail

# EMBED-08: Download a GGUF model file to the models/ directory.

MODEL_REPO="${1:-}"
MODEL_FILE="${2:-}"
MODELS_DIR="${MODELS_DIR:-$(dirname "$0")/../models}"

if [[ -z "${MODEL_REPO}" ]]; then
    echo "Usage: $0 <huggingface-repo> [model-filename]" >&2
    echo "Example: $0 nomic-ai/nomic-embed-text-v1.5-GGUF nomic-embed-text-v1.5.Q4_K_M.gguf" >&2
    exit 1
fi

mkdir -p "${MODELS_DIR}"

if command -v huggingface-cli >/dev/null 2>&1; then
    echo "Using huggingface-cli to download from ${MODEL_REPO}..."
    if [[ -n "${MODEL_FILE}" ]]; then
        huggingface-cli download "${MODEL_REPO}" "${MODEL_FILE}" \
            --local-dir "${MODELS_DIR}" \
            --local-dir-use-symlinks False
    else
        huggingface-cli download "${MODEL_REPO}" \
            --local-dir "${MODELS_DIR}" \
            --local-dir-use-symlinks False
    fi
else
    if [[ -z "${MODEL_FILE}" ]]; then
        echo "huggingface-cli not found and no model filename specified." >&2
        echo "Install huggingface-cli: pip install huggingface_hub[cli]" >&2
        exit 1
    fi
    URL="https://huggingface.co/${MODEL_REPO}/resolve/main/${MODEL_FILE}"
    DEST="${MODELS_DIR}/${MODEL_FILE}"
    echo "Downloading ${URL} to ${DEST}..."
    curl -L --progress-bar -o "${DEST}" "${URL}"
fi

echo "Download complete. Models directory:"
ls -lh "${MODELS_DIR}"
