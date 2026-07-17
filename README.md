# Bookshelf

An interactive web bookshelf and a standalone terminal application for managing
the books that appear on it.

## Features

- Shelf, stack, and coverflow website views
- Multiword library filtering and sorting
- Interactive terminal library browser
- Add and edit forms
- Multi-select and batch removal
- JSON and Excel-compatible CSV exports
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

The command asks for confirmation, removes the executable and generated
website, and preserves books, fetched and manual covers, and settings. To
permanently delete all Bookshelf data as well, use:

```bash
bookshelf uninstall --purge
```

Non-interactive uninstall requires `--yes`. `--delete-data` is an explicit
alias for `--purge`.

## Interactive manager

Running `bookshelf` without a subcommand displays command help. Open the
interactive manager with:

```bash
bookshelf list
```

The browser presents a searchable, paginated, and read-only library. Common
keys include:

```text
/        Search
Esc      Back, cancel, or quit
```

`bookshelf edit` opens a single-book picker and guided form. `bookshelf remove`
supports selecting multiple books across pages and reviewing the complete
selection before confirmation.
`q` remains a
compatibility shortcut outside text fields, but Escape is the consistent
navigation key throughout the interface.

Set `BOOKSHELF_ACCESSIBLE=1` to use screen-reader-friendly standard prompts
instead of full-screen forms.

## Direct commands

Interactive workflows are the default, but direct commands remain available
for scripts and automation:

```bash
bookshelf list --plain
bookshelf list --json
bookshelf add --title "Dune" --author "Frank Herbert" --isbn "9780441172719"
bookshelf import books.csv --skip-duplicates
bookshelf export books.csv
bookshelf export backup.json
bookshelf add --from books.json --dry-run
bookshelf edit --id-or-isbn "9780441172719" --binding "Hardcover"
bookshelf remove "9780441172719" "9780441172696" --yes
bookshelf build
bookshelf preview
bookshelf covers --id-or-isbn "9780441172719"
bookshelf covers --all
bookshelf covers --missing
bookshelf validate
bookshelf settings --default-view coverflow --default-sort author
bookshelf upgrade
```

Batch import accepts JSON or CSV. JSON can be a top-level array or an object
with a `books` array. CSV requires a `title` column and also recognizes `id`,
`author`, `isbn`, `slug`, `translator`, `publisher`, `binding`, and `published`.
Use `--format json|csv` when reading standard input with `bookshelf import -`.
Imports are saved together and trigger one final build; `--no-build`,
`--skip-duplicates`, and `--dry-run` adjust that behavior.

Export infers JSON or CSV from the destination filename:

```bash
bookshelf export books.csv
bookshelf export backup.json
bookshelf export - --format json
```

CSV exports use UTF-8 with Excel-compatible headings and line endings, retain
non-ASCII metadata, and can be imported into Bookshelf again. Existing files
are protected unless `--force` is supplied.

Run `bookshelf help COMMAND`, `bookshelf COMMAND help`, or
`bookshelf COMMAND --help` to see command-specific options.

Use `--slug` when adding or updating a book to choose its website URL:

```bash
bookshelf add --title "A Book" --isbn "978-0-00-000000-0" --slug "my-book"
```

Every book receives a generated, URL-safe title slug using broad Unicode
transliteration. Open the interactive website settings or configure individual
values directly:

```bash
bookshelf settings
bookshelf settings --permalink-style title-slug
bookshelf settings --statistics hide
bookshelf settings --default-view coverflow
bookshelf settings --default-sort author
bookshelf settings --sort-direction descending
bookshelf settings --site-title "My Library" --site-subtitle "Books worth sharing"
bookshelf settings --random hide
bookshelf settings --isbn-links wikipedia
bookshelf settings --footer-text "Curated by [Alex](https://example.com)"
```

A custom per-book slug always takes priority over that setting. Formatted ISBN,
compact ISBN, generated title slug, custom slug, and the permanent internal ID
remain valid aliases regardless of which style generates copied links. When a
preferred ISBN style is unavailable, the generated title slug is used.
The default view applies on desktop; mobile always uses the stacks view.
Custom footer text supports safe inline Markdown links, emphasis, bold text,
inline code, and line breaks. Raw HTML is displayed as text.

Non-interactive removal requires `--yes`. Use `--remove-covers` to remove the
associated durable cover files too.

## Library status

`data/books.json` is the editable source of truth. The website reads the
generated `public/data/books.js`.

The list command compares the two and reports:

- `Published`: source and published records match
- `Changes Not Published`: the published record differs
- `Not Published`: the book is absent from published data

The Covers screen separately reports `Has Cover` in the normal metadata color
or `Cover Missing` in red. Publication status appears only in the List screen.

`bookshelf validate` compares complete records rather than only array lengths,
so same-sized but stale generated libraries are detected.

## Installed layout

```text
~/.local/share/bookshelf/
  data/
    books.json
    settings.json      # created when website settings are saved
    covers/            # durable fetched and processed covers
    manual-covers/
    cover-report.json
  public/
    ...                # generated, disposable publishing output
    data/
      books.js
      covers/
```

A fresh installation starts with an empty library and generated index, even
when installed from a checkout containing development books or covers. Website
templates are embedded in the executable. Upgrades replace only that executable
and regenerate `public/` without fetching or processing covers; everything in
`data/` remains untouched. `bookshelf upgrade` checks the latest GitHub release
first and does nothing when that version is already installed; use `--check` to
check only or `--force` to reinstall it.

## Covers

Canonical covers live in durable user storage:

```text
~/.local/share/bookshelf/data/covers/
```

`bookshelf build` copies them into `public/data/covers/`; that published copy
can be deleted and recreated at any time.

ISBN-backed covers retain their entered formatting in searchable filenames,
such as `978-0-441-17271-9.jpg` or `9780441172719.jpg`.
Books without ISBNs use their permanent internal book ID. The durable filename
is recorded separately from the website URL, and Bookshelf renames the file
when an ISBN changes.

For a manual cover, place a JPEG, PNG, WebP, or BMP file in
`data/manual-covers` named after the ISBN or book id:

```text
~/.local/share/bookshelf/data/manual-covers/978-0-441-17271-9.webp
```

Apply matching manual files by choosing “manual” interactively or directly:

```bash
bookshelf covers --id-or-isbn "9780441172719" --source manual
```

Bookshelf converts manual images to JPEG and calculates spine colors internally.

Fetch covers interactively from Goodreads, Open Library, Google Books, or an
automatic fallback:

```bash
bookshelf covers
bookshelf covers --all
bookshelf covers --missing
bookshelf covers --all --source goodreads
bookshelf covers BOOK_ID --url "https://example.com/cover.jpg"
```

Use `--missing` to retry every book that does not currently have a durable
cover. Existing covers are skipped unless `--replace` is supplied. Interactive
fetching uses a progress screen. Escape pauses the operation and offers to keep
the completed downloads, discard the entire session, or continue. Downloads
remain staged until they are committed, so discarding leaves existing data
unchanged. The progress bar distinguishes downloaded, skipped, missing, and
failed results.

Custom image URLs are offered only when one book is selected. After a
single-cover download, Bookshelf can open the result in the operating system's
default image viewer and displays the saved path. Batch results that were
skipped, missing, or failed are written to:

```text
~/.local/share/bookshelf/data/cover-report.json
```

## Publishing

Build the static site:

```bash
bookshelf build
```

Preview it locally:

```bash
bookshelf preview
```

The preview command starts a local server, opens the website in the default
browser, and runs until you press Ctrl+C. Use `--port PORT` to select another
port or `--no-open` to keep the browser closed.

Publish the contents of `public/`. Do not upload `data/`, which contains the
editable local source and durable cover collection.

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

Tagged releases are built as self-contained static Linux binaries for amd64 and
arm64. Release archives contain only the binary; `checksums.txt` is verified by
the installer.

## License

Released under the [MIT License](LICENSE).

## Acknowledgements

Inspired by [Marius Balaj's bookshelf](https://balajmarius.com/writings/vibe-coding-a-bookshelf-with-claude-code/).
