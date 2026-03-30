#!/usr/bin/env bash
set -euo pipefail

# EMBED-08: Download a GGUF model file to the models/ directory.

DEFAULT_REPO="nomic-ai/nomic-embed-text-v1.5-GGUF"
DEFAULT_FILE="nomic-embed-text-v1.5.Q8_0.gguf"

MODEL_REPO="${1:-${DEFAULT_REPO}}"
MODEL_FILE="${2:-${DEFAULT_FILE}}"
MODELS_DIR="${MODELS_DIR:-$(dirname "$0")/../models}"

echo "Repository: ${MODEL_REPO}"
echo "File:       ${MODEL_FILE}"
echo "Dest:       ${MODELS_DIR}"

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
