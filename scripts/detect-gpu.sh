#!/usr/bin/env bash
set -euo pipefail

# Detect GPU availability and set LLAMA_* environment variables.

if command -v nvidia-smi &>/dev/null; then
    echo "NVIDIA GPU detected"
    export LLAMA_CUBLAS=1
    nvidia-smi --query-gpu=name,memory.total --format=csv,noheader
elif [ -d /sys/class/drm ] && ls /sys/class/drm/card*/device/vendor 2>/dev/null | xargs grep -l "0x1002" &>/dev/null; then
    echo "AMD GPU detected"
    export LLAMA_HIPBLAS=1
elif [ "$(uname -s)" = "Darwin" ] && sysctl -n machdep.cpu.brand_string 2>/dev/null | grep -qi "apple"; then
    echo "Apple Silicon detected (Metal)"
    export LLAMA_METAL=1
else
    echo "No GPU detected, using CPU"
    export LLAMA_CUBLAS=0
fi
