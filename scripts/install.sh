#!/usr/bin/env bash
# one-shot installer for remarkable-cli
# usage: curl -fsSL https://raw.githubusercontent.com/itsfabioroma/remarkable-cli/main/scripts/install.sh | bash
#        bash scripts/install.sh [--source] [--dir DIR]
set -euo pipefail

REPO_OWNER="itsfabioroma"
REPO_NAME="remarkable-cli"
REPO_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}"
API_URL="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
FORCE_SOURCE=0
TMPDIR_=""

# tty-aware color setup
if [ -t 2 ]; then IS_TTY=1; RED=$'\033[31m'; RST=$'\033[0m'; else IS_TTY=0; RED=""; RST=""; fi

# consistent error output + exit
error() { printf '%serror:%s %s\n' "$RED" "$RST" "$*" >&2; exit 1; }
info()  { printf '%s\n' "$*" >&2; }

# cleanup temp dir on any exit
cleanup() { [ -n "$TMPDIR_" ] && [ -d "$TMPDIR_" ] && rm -rf "$TMPDIR_"; }
trap cleanup EXIT

# map uname -s/-m to go release naming
detect_platform() {
  local os arch
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux)  os=linux ;;
    *) error "unsupported OS: $(uname -s)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) error "unsupported arch: $(uname -m)" ;;
  esac
  OS="$os"; ARCH="$arch"
}

# fetch latest release tag from github api; empty on 404/missing
discover_tag() {
  local body
  body=$(curl -fsSL "$API_URL" 2>/dev/null || true)
  [ -z "$body" ] && { echo ""; return; }
  # extract "tag_name": "vX.Y.Z"
  printf '%s' "$body" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1
}

# download archive + checksums to tmpdir
download_release() {
  local tag="$1" fname="remarkable_${OS}_${ARCH}.tar.gz"
  local base="${REPO_URL}/releases/download/${tag}"
  TMPDIR_=$(mktemp -d)
  info "downloading ${fname} (${tag})"
  curl -fsSL -o "$TMPDIR_/$fname" "$base/$fname" || error "failed to download $fname"
  curl -fsSL -o "$TMPDIR_/checksums.txt" "$base/checksums.txt" || error "failed to download checksums.txt"
  ARCHIVE="$TMPDIR_/$fname"; ARCHIVE_NAME="$fname"
}

# sha256 verify via shasum or sha256sum
verify_checksum() {
  local expected got
  # match "<hash>  <file>" or "<hash> *<file>"; strip optional leading '*' on field 2
  expected=$(awk -v f="$ARCHIVE_NAME" '{n=$2; sub(/^\*/,"",n); if(n==f){print $1; exit}}' "$TMPDIR_/checksums.txt")
  [ -z "$expected" ] && error "checksum entry for $ARCHIVE_NAME not found"
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$ARCHIVE" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
  else
    error "no sha256sum or shasum available"
  fi
  [ "$expected" = "$got" ] || error "checksum mismatch: expected $expected got $got"
  info "checksum ok"
}

# extract archive into tmpdir
extract_archive() {
  ( cd "$TMPDIR_" && tar -xzf "$ARCHIVE_NAME" ) || error "extract failed"
  [ -f "$TMPDIR_/remarkable" ] || error "binary 'remarkable' not found in archive"
  BIN_SRC="$TMPDIR_/remarkable"
}

# install binary, elevating with sudo only when target dir isn't writable
install_binary() {
  local target="$INSTALL_DIR/remarkable"
  mkdir -p "$INSTALL_DIR" 2>/dev/null || true
  info "installing -> $target"
  if [ -w "$INSTALL_DIR" ] || { [ ! -e "$INSTALL_DIR" ] && mkdir -p "$INSTALL_DIR" 2>/dev/null; }; then
    install -m 0755 "$BIN_SRC" "$target"
  else
    sudo install -m 0755 "$BIN_SRC" "$target" || error "install to $target failed"
  fi
  INSTALLED="$target"
}

# from-source fallback: needs go >= 1.21
source_fallback() {
  command -v go >/dev/null 2>&1 || {
    info "go not found"
    info "  macOS: brew install go"
    info "  Linux: https://go.dev/dl/"
    error "install Go >= 1.21 and retry (or download a release binary)"
  }
  local gv maj min
  gv=$(go env GOVERSION | sed 's/go//')
  maj=$(echo "$gv" | cut -d. -f1); min=$(echo "$gv" | cut -d. -f2)
  { [ "$maj" -gt 1 ] || { [ "$maj" -eq 1 ] && [ "$min" -ge 21 ]; }; } || error "go >= 1.21 required (have $gv)"

  # use cwd if inside repo, else clone fresh
  local src
  if [ -f "go.mod" ] && grep -q "module github.com/${REPO_OWNER}/${REPO_NAME}" go.mod 2>/dev/null; then
    src="$PWD"
  else
    TMPDIR_=$(mktemp -d)
    src="$TMPDIR_/src"
    info "cloning $REPO_URL"
    git clone --depth 1 "$REPO_URL" "$src" || error "git clone failed"
  fi

  # build to a stable temp path regardless of cwd
  local out="${TMPDIR_:-$(mktemp -d)}/remarkable-build"
  TMPDIR_="${TMPDIR_:-$(dirname "$out")}"
  info "building from source..."
  ( cd "$src" && go build -o "$out" . ) || error "go build failed"
  BIN_SRC="$out"
  install_binary
}

# release path: discover tag, download, verify, extract, install
release_install() {
  local tag
  tag=$(discover_tag)
  if [ -z "$tag" ]; then
    info "no release published yet — falling back to source build"
    source_fallback
    return
  fi
  download_release "$tag"
  verify_checksum
  extract_archive
  install_binary
}

# print version + next steps
post_install() {
  local v
  v=$("$INSTALLED" --version 2>/dev/null || echo "remarkable")
  info ""
  info "installed: $v"
  info "  -> $INSTALLED"
  info ""
  info "next steps:"
  info "  remarkable auth      # connect reMarkable Cloud"
  info "  remarkable connect   # SSH setup wizard (optional)"
}

main() {
  # arg parse: --source forces build-from-source, --dir overrides install dir
  while [ $# -gt 0 ]; do
    case "$1" in
      --source) FORCE_SOURCE=1; shift ;;
      --dir) INSTALL_DIR="${2:-}"; [ -z "$INSTALL_DIR" ] && error "--dir requires an argument"; shift 2 ;;
      --dir=*) INSTALL_DIR="${1#--dir=}"; shift ;;
      -h|--help) info "usage: install.sh [--source] [--dir DIR]"; exit 0 ;;
      *) error "unknown flag: $1" ;;
    esac
  done

  detect_platform
  if [ "$FORCE_SOURCE" -eq 1 ]; then
    source_fallback
  else
    release_install
  fi
  post_install
}

main "$@"
