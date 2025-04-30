#!/bin/bash
# Simple script to start the MkDocs development server

# Check if mkdocs is installed
if ! command -v mkdocs &> /dev/null; then
    echo "MkDocs is not installed. Installing mkdocs-material..."
    pip install mkdocs-material
fi

# Navigate to the project root and start the server
cd "$(dirname "$0")/.."
echo "Starting MkDocs server at http://127.0.0.1:8000/"
mkdocs serve
