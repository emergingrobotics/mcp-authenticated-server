#!/usr/bin/env bash
set -euo pipefail

# EVAL-05: Run the eval suite N times to measure stability.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RUNS="${1:-25}"
EVAL_FILE="${2:-${SCRIPT_DIR}/../data/evals/evals.json}"

echo "Running eval stability test: ${RUNS} iterations"
echo "Eval file: ${EVAL_FILE}"
echo "---"

RESULTS=()

for i in $(seq 1 "${RUNS}"); do
    OUTPUT=$("${SCRIPT_DIR}/eval.sh" "${EVAL_FILE}" 2>&1) || true
    RATE=$(echo "${OUTPUT}" | grep -oP 'Pass rate: \K[0-9.]+' || echo "0")
    RESULTS+=("${RATE}")
    echo "Run ${i}/${RUNS}: ${RATE}%"
done

MIN=$(printf '%s\n' "${RESULTS[@]}" | sort -n | head -1)
MAX=$(printf '%s\n' "${RESULTS[@]}" | sort -n | tail -1)
SUM=$(printf '%s\n' "${RESULTS[@]}" | awk '{s+=$1} END {print s}')
AVG=$(awk "BEGIN { printf \"%.1f\", ${SUM}/${RUNS} }")

echo "---"
echo "Stability Report (${RUNS} runs):"
echo "  Min: ${MIN}%"
echo "  Max: ${MAX}%"
echo "  Avg: ${AVG}%"
