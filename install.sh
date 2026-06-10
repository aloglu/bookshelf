#!/usr/bin/env sh
set -eu

REPO_URL="${BOOKSHELF_REPO_URL:-https://github.com/aloglu/bookshelf.git}"
ARCHIVE_URL="${BOOKSHELF_ARCHIVE_URL:-https://github.com/aloglu/bookshelf/archive/refs/heads/main.tar.gz}"
INSTALL_DIR="${BOOKSHELF_INSTALL_DIR:-$HOME/.local/share/bookshelf}"
BIN_DIR="${BOOKSHELF_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/bookshelf"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

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

if [ "${1:-}" = "--uninstall" ] || [ "${1:-}" = "uninstall" ]; then
  uninstall
  exit 0
fi

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js is required before installing bookshelf." >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
mkdir -p "$(dirname "$INSTALL_DIR")"

BACKUP_DIR=""
backup_user_data() {
  if [ -d "$INSTALL_DIR" ]; then
    BACKUP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/bookshelf-backup.XXXXXX")
    if [ -d "$INSTALL_DIR/library" ]; then
      mkdir -p "$BACKUP_DIR"
      cp -a "$INSTALL_DIR/library" "$BACKUP_DIR/library"
    fi
    if [ -d "$INSTALL_DIR/public/data/covers" ]; then
      mkdir -p "$BACKUP_DIR/public/data"
      cp -a "$INSTALL_DIR/public/data/covers" "$BACKUP_DIR/public/data/covers"
    fi
    if [ -f "$INSTALL_DIR/public/data/books.js" ]; then
      mkdir -p "$BACKUP_DIR/public/data"
      cp -a "$INSTALL_DIR/public/data/books.js" "$BACKUP_DIR/public/data/books.js"
    fi
  fi
}

restore_user_data() {
  if [ -n "$BACKUP_DIR" ] && [ -d "$BACKUP_DIR" ]; then
    if [ -d "$BACKUP_DIR/library" ]; then
      rm -rf "$INSTALL_DIR/library"
      mkdir -p "$INSTALL_DIR"
      cp -a "$BACKUP_DIR/library" "$INSTALL_DIR/library"
    fi
    if [ -d "$BACKUP_DIR/public/data/covers" ]; then
      rm -rf "$INSTALL_DIR/public/data/covers"
      mkdir -p "$INSTALL_DIR/public/data"
      cp -a "$BACKUP_DIR/public/data/covers" "$INSTALL_DIR/public/data/covers"
    fi
    if [ -f "$BACKUP_DIR/public/data/books.js" ]; then
      mkdir -p "$INSTALL_DIR/public/data"
      cp -a "$BACKUP_DIR/public/data/books.js" "$INSTALL_DIR/public/data/books.js"
    fi
    rm -rf "$BACKUP_DIR"
  fi
}

backup_user_data

if [ -f "$SCRIPT_DIR/cli/bookshelf.mjs" ]; then
  rm -rf "$INSTALL_DIR"
  mkdir -p "$INSTALL_DIR"
  (
    cd "$SCRIPT_DIR"
    tar --exclude .git -cf - .
  ) | (
    cd "$INSTALL_DIR"
    tar -xf -
  )
elif command -v git >/dev/null 2>&1; then
  if [ -d "$INSTALL_DIR/.git" ]; then
    git -C "$INSTALL_DIR" pull --ff-only
  else
    rm -rf "$INSTALL_DIR"
    git clone "$REPO_URL" "$INSTALL_DIR"
  fi
elif command -v curl >/dev/null 2>&1; then
  TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/bookshelf.XXXXXX")
  ARCHIVE_PATH="$TMP_DIR/bookshelf.tar.gz"
  curl -fsSL "$ARCHIVE_URL" -o "$ARCHIVE_PATH"
  rm -rf "$INSTALL_DIR"
  mkdir -p "$INSTALL_DIR"
  tar -xzf "$ARCHIVE_PATH" -C "$INSTALL_DIR" --strip-components=1
  rm -rf "$TMP_DIR"
else
  echo "git or curl is required for this installer." >&2
  exit 1
fi

restore_user_data

if [ ! -f "$INSTALL_DIR/cli/bookshelf.mjs" ]; then
  echo "Install failed: $INSTALL_DIR/cli/bookshelf.mjs was not found." >&2
  echo "If installing from GitHub, commit and push the CLI files first." >&2
  exit 1
fi

if [ ! -f "$INSTALL_DIR/public/index.html" ]; then
  echo "Install failed: $INSTALL_DIR/public/index.html was not found." >&2
  echo "If installing from GitHub, commit and push the public site files first." >&2
  exit 1
fi

if [ ! -f "$INSTALL_DIR/library/books.json" ]; then
  echo "Install failed: $INSTALL_DIR/library/books.json was not found." >&2
  echo "If installing from GitHub, commit and push the library files first." >&2
  exit 1
fi

cat > "$BIN_PATH" <<EOF
#!/usr/bin/env sh
set -eu
export BOOKSHELF_INSTALL_DIR="$INSTALL_DIR"
export BOOKSHELF_BIN_PATH="$BIN_PATH"
exec node "$INSTALL_DIR/cli/bookshelf.mjs" "\$@"
EOF

chmod +x "$BIN_PATH"

echo "Installed bookshelf command: $BIN_PATH"
echo "Installed bookshelf files: $INSTALL_DIR"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH to run bookshelf from any directory." ;;
esac
