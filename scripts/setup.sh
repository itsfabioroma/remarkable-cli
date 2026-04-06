#!/bin/bash
# setup dependencies and build remarkable-cli

set -e

echo "fetching Go dependencies..."
GONOSUMDB=* GONOSUMCHECK=* go mod tidy

echo "building..."
go build -o remarkable .

echo "running tests..."
go test ./...

echo "done! binary at ./remarkable"
echo "try: ./remarkable ls --transport ssh"
