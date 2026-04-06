#!/bin/bash
set -e

# one-shot setup: clone → build → connect → done
# usage: bash scripts/setup.sh [device-ip]

HOST="${1:-10.11.99.1}"

echo "building remarkable-cli..."
GONOSUMDB=* GONOSUMCHECK=* go mod tidy 2>/dev/null
go build -o remarkable .
echo "  built: ./remarkable"

echo ""
echo "connecting to device at $HOST..."
./remarkable connect "$HOST" 2>&1

echo ""
echo "done. try:"
echo "  ./remarkable ls"
echo "  ./remarkable export \"Notebook Name\""
echo "  ./remarkable splash set image.png"
