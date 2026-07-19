# Bookshelf

An interactive web bookshelf and a standalone terminal application for managing
the books that appear on it.

## Features

- Shelf, stack, and coverflow website views
- Multiword library filtering and sorting
- Interactive terminal library browser
- Add and edit forms
- Multi-select and batch removal
- Per-book public website visibility
- JSON, Excel-compatible CSV, and complete ZIP-based Bookshelf archives
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
alias for `--purge`. Purging removes Bookshelf-owned data while preserving
unrelated files if a custom installation directory is shared with other data.

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
selection before confirmation. `bookshelf visibility` shows or hides one or
more selected books on the public website without removing them from the
library.

`q` remains a
compatibility shortcut outside text fields, but Escape is the consistent
navigation key throughout the interface.

Set `BOOKSHELF_ACCESSIBLE=1` to use screen-reader-friendly, line-oriented
prompts instead of the full-screen interface. Numbered choices replace
highlighted menus; while editing, press Enter to keep an existing value or
enter `-` to clear it.

## Direct commands

Interactive workflows are the default, but direct commands remain available
for scripts and automation:

```bash
bookshelf list --plain
bookshelf list --json
bookshelf status
bookshelf status --json
bookshelf add --title "Dune" --author "Frank Herbert" --isbn "9780441172719"
bookshelf import books.csv --skip-duplicates
bookshelf export books.csv
bookshelf export backup.json
bookshelf export library.bookshelf
bookshelf add --from books.json --dry-run
bookshelf edit --id-or-isbn "9780441172719" --binding "Hardcover"
bookshelf visibility --hide "9780441172719"
bookshelf visibility --show "9780441172719"
bookshelf remove "9780441172719" "9780441172696" --yes
bookshelf build
bookshelf preview
bookshelf covers --id-or-isbn "9780441172719"
bookshelf covers --all
bookshelf covers --missing
bookshelf covers --attention
bookshelf validate
bookshelf settings --default-view coverflow --default-sort author
bookshelf upgrade
```

Batch import accepts JSON or CSV. JSON can be a top-level array or an object
with a `books` array. CSV requires a `title` column and also recognizes `id`,
`author`, `isbn`, `slug`, `translator`, `publisher`, `binding`, `published`,
and `website visibility`.
Use `--format json|csv` when reading standard input with `bookshelf import -`.
Imports are saved together and trigger one final build; `--no-build`,
`--skip-duplicates`, and `--dry-run` adjust that behavior.

Export infers JSON, CSV, or Bookshelf archive format from the destination filename:

```bash
bookshelf export books.csv
bookshelf export backup.json
bookshelf export library.bookshelf
bookshelf export - --format json
```

CSV exports use UTF-8 with Excel-compatible headings and line endings, retain
non-ASCII metadata, protect formula-like cells when opened in spreadsheet
software, and can be imported into Bookshelf again without changing those
values. Existing files are protected unless `--force` is supplied.

A `.bookshelf` file is a standard ZIP archive containing `manifest.json`,
`books.json`, `settings.json`, fetched covers, and manual covers. It can be
opened with any ZIP program. Generated `public/` files are omitted and rebuilt
after import. Archive v2 preserves website visibility and is the supported
Bookshelf archive format.

Importing an archive into an empty library restores it directly. With an
existing library, Bookshelf asks whether to merge, replace, or cancel:

```bash
bookshelf import library.bookshelf --merge
bookshelf import library.bookshelf --merge --skip-duplicates
bookshelf import library.bookshelf --replace
```

Merge retains current website settings and copies covers belonging to imported
books. Replace restores the archive’s books, settings, fetched covers, and
manual covers as a complete backup. The archive is fully validated before the
choice is shown. Replacing a non-empty library first writes a timestamped
safety archive under `backups/`. Bookshelf retains the five most recent
automatic safety archives and never prunes files with other names. Scripts must
specify `--merge` or `--replace` when the destination is not empty.

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
bookshelf settings --shelf-scroll-speed slow --coverflow-scroll-speed fast
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
- `Hidden from Website`: the book is intentionally retained only in the library
- `Still Visible on Website`: a pending hide has not been published

Visibility changes made through `bookshelf visibility` or Add/Edit are
published automatically. Hidden books and their generated cover files are
excluded from the website, while their original covers remain available for
backups, exports, and later restoration.

The Covers screen separately reports `Has Cover` in the normal metadata color
or `Cover Missing` in red. Publication status appears only in the List screen.

`bookshelf status` gives a quick summary of book and cover counts, publication
state, website freshness, unresolved cover work, and storage paths. Add
`--json` for scripts.

`bookshelf validate` compares complete records rather than only array lengths,
so same-sized but stale generated libraries are detected. Hard integrity or
publishing errors fail the command; repair-worthy issues such as suspicious
ISBN checksums, missing or orphaned covers, unsupported manual covers, and
likely duplicate ISBN-less books are reported as warnings.

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
      covers/          # 480px WebP detail covers
      thumbnails/      # 360px WebP covers used by list views
  backups/
    before-replace-*.bookshelf
```

A fresh installation starts with an empty library and generated index, even
when installed from a checkout containing development books or covers. Website
templates are embedded in the executable. Upgrades replace only that executable
and regenerate `public/` without fetching covers; unchanged WebP variants are
reused and only changed covers are processed. Everything in `data/` remains
untouched. `bookshelf upgrade` checks the latest GitHub release
first and does nothing when that version is already installed; use `--check` to
check only or `--force` to reinstall it.

Release installations use `~/.local/share/bookshelf`. Running `./install.sh`
from a source checkout builds a development binary that instead uses
`~/.local/share/bookshelf-dev`, even when launched outside the checkout.
Repository-local `data/`, `public/`, and `backups/` directories are ignored by
Git and never selected implicitly. A custom `BOOKSHELF_INSTALL_DIR` selected
during installation is remembered for later commands. Setting the variable for
an individual command remains available as an explicit temporary override.

## Covers

Canonical covers live in durable user storage:

```text
~/.local/share/bookshelf/data/covers/
```

`bookshelf build` creates lossy WebP derivatives: 360px-wide thumbnails for
shelf, stack, and coverflow views, and 480px-wide covers for the detail view.
Smaller originals are never upscaled. Durable source covers remain untouched;
both published directories are disposable and recreated by a build.

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
bookshelf covers --attention
bookshelf covers --all --source goodreads
bookshelf covers BOOK_ID --url "https://example.com/cover.jpg"
```

Use `--missing` to retry every book that does not currently have a durable
cover. Existing covers are skipped unless `--replace` is supplied; choosing a
custom URL for one book is itself an explicit replacement. Interactive fetching
uses a progress screen. Escape pauses the operation and offers to keep
the completed downloads, discard the entire session, or continue. Downloads
remain staged until they are committed, so discarding leaves existing data
unchanged. The progress bar distinguishes downloaded, skipped, missing, and
failed results.

Use `--attention` to revisit unresolved books from the most recent fetch
report. Books that were removed or have since received covers are omitted. You
can select a smaller set, retry another source, or provide a custom URL when
only one book is selected.

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
node scripts/browser-smoke.mjs
mkdir -p build
go build -o build/bookshelf ./cmd/bookshelf
```

The browser smoke test uses Node.js 22 and Chromium or Chrome when available,
and otherwise skips locally when no browser is installed; CI requires it. The
compiled binary is written beneath the ignored `build/` directory so it cannot
replace the tracked development launcher.

Run directly from the checkout with an isolated, repository-local library:

```bash
./bookshelf
```

Install a local checkout as the isolated per-user development profile:

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
