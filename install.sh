#!/usr/bin/env sh
set -eu

REPO="${BOOKSHELF_REPO:-aloglu/bookshelf}"
INSTALL_DIR="${BOOKSHELF_INSTALL_DIR:-$HOME/.local/share/bookshelf}"
BIN_DIR="${BOOKSHELF_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/bookshelf"
VERSION="${BOOKSHELF_VERSION:-latest}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

TMP_DIR=""
BACKUP_DIR=""
INSTALL_IN_PROGRESS=0

cleanup() {
  if [ "$INSTALL_IN_PROGRESS" -eq 1 ] && [ -n "$BACKUP_DIR" ] && [ -d "$BACKUP_DIR" ]; then
    echo "Installation did not complete; restoring user library data." >&2
    mkdir -p "$INSTALL_DIR"
    restore_user_data
  fi
  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
  if [ -n "$BACKUP_DIR" ] && [ -d "$BACKUP_DIR" ]; then
    rm -rf "$BACKUP_DIR"
  fi
}
trap cleanup EXIT HUP INT TERM

uninstall() {
  if [ -f "$BIN_PATH" ]; then
    rm -f "$BIN_PATH"
    echo "Removed $BIN_PATH"
  fi
  if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    echo "Removed $INSTALL_DIR"
  fi
}

case "${1:-}" in
  uninstall|--uninstall)
    uninstall
    exit 0
    ;;
  --upgrade)
    ;;
  "")
    ;;
  *)
    echo "Usage: install.sh [--upgrade|--uninstall]" >&2
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
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=dev" \
      -o "$TMP_DIR/package/bookshelf" ./cmd/bookshelf
  )
  cp -a "$SCRIPT_DIR/public" "$TMP_DIR/package/public"
  mkdir -p "$TMP_DIR/package/library/manual-covers"
  cp "$SCRIPT_DIR/library/books.json" "$TMP_DIR/package/library/books.json"
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

backup_user_data() {
  BACKUP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/bookshelf-backup.XXXXXX")
  if [ -d "$INSTALL_DIR/library" ]; then
    cp -a "$INSTALL_DIR/library" "$BACKUP_DIR/library"
  fi
  if [ -d "$INSTALL_DIR/public/data/covers" ]; then
    mkdir -p "$BACKUP_DIR/public/data"
    cp -a "$INSTALL_DIR/public/data/covers" "$BACKUP_DIR/public/data/covers"
  fi
}

restore_user_data() {
  if [ -d "$BACKUP_DIR/library" ]; then
    rm -rf "$INSTALL_DIR/library"
    cp -a "$BACKUP_DIR/library" "$INSTALL_DIR/library"
  fi
  if [ -d "$BACKUP_DIR/public/data/covers" ]; then
    rm -rf "$INSTALL_DIR/public/data/covers"
    mkdir -p "$INSTALL_DIR/public/data"
    cp -a "$BACKUP_DIR/public/data/covers" "$INSTALL_DIR/public/data/covers"
  fi
}

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/bookshelf-install.XXXXXX")

LOCAL_SOURCE=0
case "$0" in
  *install.sh)
    if [ -f "$SCRIPT_DIR/cmd/bookshelf/main.go" ] && [ -d "$SCRIPT_DIR/public" ]; then
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
if [ ! -f "$TMP_DIR/package/public/index.html" ]; then
  echo "Install failed: release does not contain the public site." >&2
  exit 1
fi

backup_user_data
INSTALL_IN_PROGRESS=1
mkdir -p "$BIN_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cp -a "$TMP_DIR/package/public" "$INSTALL_DIR/public"
cp -a "$TMP_DIR/package/library" "$INSTALL_DIR/library"
restore_user_data
install -m 0755 "$TMP_DIR/package/bookshelf" "$BIN_PATH"

BOOKSHELF_INSTALL_DIR="$INSTALL_DIR" BOOKSHELF_BIN_PATH="$BIN_PATH" "$BIN_PATH" build
INSTALL_IN_PROGRESS=0

echo "Installed Bookshelf command: $BIN_PATH"
echo "Installed Bookshelf files: $INSTALL_DIR"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH to run bookshelf from any directory." ;;
esac
