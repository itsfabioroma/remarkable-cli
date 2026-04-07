#!/usr/bin/env bash
# one-shot installer for remarkable-cli
set -euo pipefail

REPO="https://github.com/itsfabioroma/remarkable-cli"
BIN="/usr/local/bin/remarkable"

# require go >= 1.21
if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found" >&2
  echo "  macOS: brew install go" >&2
  echo "  Linux: https://go.dev/dl/" >&2
  exit 1
fi
GO_VER=$(go env GOVERSION | sed 's/go//')
GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 21 ]; }; then
  echo "error: go >= 1.21 required (have $GO_VER)" >&2
  exit 1
fi

# build from current repo or fresh clone
if [ -f "go.mod" ] && grep -q "remarkable-cli" go.mod 2>/dev/null; then
  SRC="$PWD"
else
  SRC=$(mktemp -d)
  echo "cloning $REPO -> $SRC"
  git clone --depth 1 "$REPO" "$SRC"
fi

# build
echo "building..."
( cd "$SRC" && go build -o remarkable . )

# install (sudo if needed)
echo "installing -> $BIN"
if [ -w "$(dirname "$BIN")" ]; then
  install -m 0755 "$SRC/remarkable" "$BIN"
else
  sudo install -m 0755 "$SRC/remarkable" "$BIN"
fi

echo
echo "installed: $($BIN --version 2>/dev/null || echo remarkable)"
echo "next steps:"
echo "  remarkable connect   # SSH setup wizard"
echo "  remarkable auth      # cloud login"
