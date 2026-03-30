#!/usr/bin/env bash
set -euo pipefail

# Install PostgreSQL with pgvector extension for local development.

echo "Installing PostgreSQL and pgvector..."

if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y postgresql postgresql-common
    sudo apt-get install -y postgresql-16-pgvector 2>/dev/null || {
        echo "pgvector package not available via apt. Building from source..."
        sudo apt-get install -y build-essential git postgresql-server-dev-16
        TMPDIR=$(mktemp -d)
        git clone --branch v0.7.4 https://github.com/pgvector/pgvector.git "${TMPDIR}/pgvector"
        cd "${TMPDIR}/pgvector"
        make
        sudo make install
        rm -rf "${TMPDIR}"
    }
elif command -v brew >/dev/null 2>&1; then
    brew install postgresql@16 pgvector
else
    echo "Unsupported package manager. Install PostgreSQL 16 and pgvector manually." >&2
    exit 1
fi

echo "PostgreSQL with pgvector installed."
echo "Start with: sudo systemctl start postgresql"
echo "Create the extension in your database: CREATE EXTENSION vector;"
