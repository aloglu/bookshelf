#!/usr/bin/env sh
set -eu

REPO="${BOOKSHELF_REPO:-aloglu/bookshelf}"
INSTALL_DIR="${BOOKSHELF_INSTALL_DIR:-$HOME/.local/share/bookshelf}"
BIN_DIR="${BOOKSHELF_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/bookshelf"
VERSION="${BOOKSHELF_VERSION:-latest}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

TMP_DIR=""

cleanup() {
  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT HUP INT TERM

uninstall() {
  if [ ! -x "$BIN_PATH" ]; then
    echo "Bookshelf command was not found at $BIN_PATH." >&2
    echo "No files were removed. Review $INSTALL_DIR manually if it contains a previous installation." >&2
    exit 1
  fi
  exec "$BIN_PATH" uninstall "$@"
}

case "${1:-}" in
  uninstall|--uninstall)
    shift
    uninstall "$@"
    ;;
  --upgrade)
    ;;
  "")
    ;;
  *)
    echo "Usage: install.sh [--upgrade|--uninstall [--purge|--delete-data] [--yes]]" >&2
    exit 2
    ;;
esac

platform() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    linux) ;;
    *)
      echo "Unsupported operating system: $os" >&2
      exit 1
      ;;
  esac

  machine=$(uname -m)
  case "$machine" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *)
      echo "Unsupported CPU architecture: $machine" >&2
      exit 1
      ;;
  esac
  printf '%s_%s\n' "$os" "$arch"
}

download() {
  url=$1
  destination=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
  else
    echo "curl or wget is required to install Bookshelf." >&2
    exit 1
  fi
}

verify_checksum() {
  archive=$1
  checksums=$2
  archive_name=$(basename "$archive")
  expected=$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksums")
  if [ -z "$expected" ]; then
    echo "Checksum for $archive_name was not found." >&2
    exit 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | awk '{ print $1 }')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive" | awk '{ print $1 }')
  else
    echo "sha256sum or shasum is required to verify Bookshelf." >&2
    exit 1
  fi

  if [ "$actual" != "$expected" ]; then
    echo "Checksum verification failed for $archive_name." >&2
    exit 1
  fi
}

prepare_local_checkout() {
  if ! command -v go >/dev/null 2>&1; then
    echo "Go is required only when installing from a source checkout." >&2
    echo "Normal users should run the curl installer, which downloads a precompiled binary." >&2
    exit 1
  fi
  echo "Building Bookshelf from local source..."
  mkdir -p "$TMP_DIR/package"
  (
    cd "$SCRIPT_DIR"
    CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=dev" \
      -o "$TMP_DIR/package/bookshelf" ./cmd/bookshelf
  )
}

prepare_release() {
  target=$(platform)
  archive_name="bookshelf_${target}.tar.gz"
  if [ -n "${BOOKSHELF_RELEASE_BASE_URL:-}" ]; then
    base_url="${BOOKSHELF_RELEASE_BASE_URL%/}"
  elif [ "$VERSION" = "latest" ]; then
    base_url="https://github.com/$REPO/releases/latest/download"
  else
    base_url="https://github.com/$REPO/releases/download/$VERSION"
  fi

  archive="$TMP_DIR/$archive_name"
  checksums="$TMP_DIR/checksums.txt"
  echo "Downloading Bookshelf for $target..."
  download "$base_url/$archive_name" "$archive"
  download "$base_url/checksums.txt" "$checksums"
  verify_checksum "$archive" "$checksums"
  mkdir -p "$TMP_DIR/package"
  tar -xzf "$archive" -C "$TMP_DIR/package"
}

remove_completions() {
  data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"

  rm -f \
    "$data_home/bash-completion/completions/bookshelf" \
    "$data_home/zsh/site-functions/_bookshelf" \
    "$config_home/fish/completions/bookshelf.fish"
}

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/bookshelf-install.XXXXXX")

LOCAL_SOURCE=0
case "$0" in
  *install.sh)
    if [ -f "$SCRIPT_DIR/go.mod" ] && [ -f "$SCRIPT_DIR/cmd/bookshelf/main.go" ]; then
      LOCAL_SOURCE=1
    fi
    ;;
esac

if [ "$LOCAL_SOURCE" -eq 1 ]; then
  prepare_local_checkout
else
  prepare_release
fi

if [ ! -x "$TMP_DIR/package/bookshelf" ]; then
  echo "Install failed: release does not contain the Bookshelf binary." >&2
  exit 1
fi
mkdir -p "$BIN_DIR"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP_DIR/package/bookshelf" "$BIN_PATH"
remove_completions

BOOKSHELF_INSTALL_DIR="$INSTALL_DIR" BOOKSHELF_BIN_PATH="$BIN_PATH" "$BIN_PATH" _init

echo "Installed Bookshelf command: $BIN_PATH"
echo "Installed Bookshelf files: $INSTALL_DIR"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH to run bookshelf from any directory." ;;
esac
