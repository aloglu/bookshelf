# Bookshelf

An interactive web bookshelf and a standalone terminal application for managing
the books that appear on it.

## Features

- Shelf, stack, and coverflow website views
- Fuzzy search and sorting
- Interactive terminal library browser
- Add and edit forms
- Multi-select and batch removal
- Manual and automatically downloaded covers
- Pure-Go cover conversion and spine-color extraction
- Source-versus-published status checks
- Script-friendly direct commands and JSON output
- Self-upgrade support

## Installation

Linux on x86-64 and ARM64 is supported.

```bash
curl -fsSL https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh | sh
```

The installer downloads a precompiled, checksum-verified release and creates:

```text
~/.local/bin/bookshelf
~/.local/share/bookshelf/
```

Users do not need Go, Node.js, Bubble Tea, ImageMagick, or a package manager.
Bubble Tea and the other Go libraries are compiled into the Bookshelf binary.

If `~/.local/bin` is not already in `PATH`, add it to your shell profile.

Upgrade:

```bash
bookshelf upgrade
```

Uninstall:

```bash
bookshelf uninstall
```

Back up `~/.local/share/bookshelf/library` first if you want to keep the
library after uninstalling.

## Interactive manager

Run:

```bash
bookshelf
```

The manager presents a searchable and paginated library. Common keys include:

```text
/        Search
Space    Select or unselect a book
a        Add a book
e/Enter  Edit the current book
d        Remove the current or selected books
c        Apply manual covers
b        Build the published library
v        Validate
q        Quit
```

Add and edit actions open guided forms. Removal supports selecting multiple
books and reviewing the complete selection before confirmation.

Set `BOOKSHELF_ACCESSIBLE=1` to use screen-reader-friendly standard prompts
instead of full-screen forms.

## Direct commands

Interactive workflows are the default, but direct commands remain available
for scripts and automation:

```bash
bookshelf list --plain
bookshelf list --json
bookshelf add --title "Dune" --author "Frank Herbert" --isbn "9780441172719"
bookshelf update --id-or-isbn "9780441172719" --binding "Hardcover"
bookshelf remove "9780441172719" "9780441172696" --yes
bookshelf build --fetch-covers
bookshelf covers --id-or-isbn "9780441172719"
bookshelf covers --all
bookshelf validate
bookshelf upgrade
```

`bookshelf edit` is an alias for `bookshelf update`.

Non-interactive removal requires `--yes`. Use `--remove-covers` to remove the
associated published cover files too.

## Library status

`library/books.json` is the editable source of truth. The website reads the
generated `public/data/books.js`.

The list command compares the two and reports:

- `ready`: source and published records match and a cover is present
- `missing cover`: source and published records match without a cover
- `stale`: the published record differs
- `not generated`: the book is absent from published data

`bookshelf validate` compares complete records rather than only array lengths,
so same-sized but stale generated libraries are detected.

## Installed layout

```text
~/.local/share/bookshelf/
  library/
    books.json
    manual-covers/
  public/
    index.html
    css/
    js/
    data/
      books.js
      covers/
    fonts/
    img/
```

The installer and `bookshelf upgrade` preserve `library/` and published covers,
then regenerate `books.js` from the source library.

## Covers

Published covers live in:

```text
~/.local/share/bookshelf/public/data/covers/
```

For a manual cover, place a JPEG, PNG, WebP, or BMP file in
`library/manual-covers` named after the ISBN or book id:

```text
~/.local/share/bookshelf/library/manual-covers/9780441172719.webp
```

Apply it:

```bash
bookshelf covers --id-or-isbn "9780441172719"
```

Bookshelf converts manual images to JPEG and calculates spine colors internally.

Fetch missing ISBN covers from Open Library:

```bash
bookshelf build --fetch-covers
```

## Publishing

Build the static site:

```bash
bookshelf build
```

Preview it locally by opening:

```text
~/.local/share/bookshelf/public/index.html
```

Publish the contents of `public/`. Do not upload `library/`, which contains the
editable local source.

## Migrating an older installation

Copy an older `books.json` and manual covers into the installed library:

```bash
cp old-backup/data/books.json ~/.local/share/bookshelf/library/books.json
mkdir -p ~/.local/share/bookshelf/library/manual-covers
cp -a old-backup/data/manual-covers/. ~/.local/share/bookshelf/library/manual-covers/
bookshelf build
```

Legacy files in `library/covers` are migrated into `public/data/covers` during
the next build.

Do not restore an old `books.js`; it is generated from `books.json`.

## Development

Building from source requires Go. Normal installations do not.

```bash
go test ./...
go build ./cmd/bookshelf
```

Run against the repository data:

```bash
./bookshelf
```

Install a local checkout into the normal per-user layout:

```bash
./install.sh
```

Tagged releases are built as static Linux binaries for amd64 and arm64. Release
archives include the binary and public-site assets; `checksums.txt` is verified
by the installer.

## License

Released under the [MIT License](LICENSE).

## Acknowledgements

Inspired by [Marius Balaj's bookshelf](https://balajmarius.com/writings/vibe-coding-a-bookshelf-with-claude-code/).
