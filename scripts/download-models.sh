#!/usr/bin/env bash
set -euo pipefail

# Download all GGUF models referenced in config.toml.
# Reads hf_repo / hf_file from [embed] and [reranker] sections.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG="${1:-${ROOT_DIR}/config.toml}"

if [ ! -f "${CONFIG}" ]; then
    echo "ERROR: config file not found: ${CONFIG}"
    echo "  Copy config.toml.example to config.toml first."
    exit 1
fi

MODELS_DIR="${MODELS_DIR:-${ROOT_DIR}/models}"
mkdir -p "${MODELS_DIR}"

# Parse hf_repo and hf_file from a given TOML section.
# Usage: parse_section <section_name>
# Sets HF_REPO and HF_FILE variables.
parse_section() {
    local section="$1"
    HF_REPO=""
    HF_FILE=""
    local in_section=false
    while IFS= read -r line; do
        # Strip leading/trailing whitespace
        line="${line#"${line%%[![:space:]]*}"}"
        # Skip comments and empty lines
        [[ -z "$line" || "$line" == \#* ]] && continue
        # Detect section headers
        if [[ "$line" == \[* ]]; then
            if [[ "$line" == "[${section}]" ]]; then
                in_section=true
            else
                in_section=false
            fi
            continue
        fi
        if $in_section; then
            case "$line" in
                hf_repo*)
                    HF_REPO=$(echo "$line" | sed 's/^hf_repo[[:space:]]*=[[:space:]]*"\(.*\)"/\1/')
                    ;;
                hf_file*)
                    HF_FILE=$(echo "$line" | sed 's/^hf_file[[:space:]]*=[[:space:]]*"\(.*\)"/\1/')
                    ;;
            esac
        fi
    done < "${CONFIG}"
}

download_count=0

# Download a model if hf_repo and hf_file are set and the file is not already present.
download_if_needed() {
    local label="$1"
    local repo="$2"
    local file="$3"

    if [ -z "${repo}" ] || [ -z "${file}" ]; then
        echo "  ${label}: no hf_repo/hf_file configured, skipping."
        return
    fi

    if [ -f "${MODELS_DIR}/${file}" ]; then
        echo "  ${label}: ${file} already present, skipping."
        return
    fi

    echo "  ${label}: downloading ${repo} / ${file}..."
    "${SCRIPT_DIR}/download-model.sh" "${repo}" "${file}"
    download_count=$((download_count + 1))
}

echo "Config: ${CONFIG}"
echo "Models: ${MODELS_DIR}"
echo ""

parse_section "embed"
download_if_needed "embed" "${HF_REPO}" "${HF_FILE}"

parse_section "reranker"
download_if_needed "reranker" "${HF_REPO}" "${HF_FILE}"

echo ""
if [ "${download_count}" -eq 0 ]; then
    echo "All models already present."
else
    echo "Downloaded ${download_count} model(s)."
fi
echo ""
echo "Models directory:"
ls -lh "${MODELS_DIR}"
