# Bookshelf

An interactive web-based library to showcase your book collection.

## Features

- **Three Unique Views**:
  - **Shelf**: A realistic, scrolling bookshelf with physics-based momentum and tilt animations.
  - **Stacks**: A clean, grid-based list view for efficiency.
  - **Coverflow**: A coverflow view of your library.
- **Dynamic Physics**: Smooth scrolling and tilt mechanics that react to your scroll velocity.
- **Performance**: Lazy loading for covers and DOM elements.
- **Search & Filter**: Instant fuzzy search and filtering by author, title, or year.
- **Auto-Theming**: Extracts cover colors to color-code book spines and details.
- **Guided Library Management**: A CLI for building, adding, editing, removing, and validating books.

## Requirements

- Linux is the supported target.
- Node.js 18 or newer
- Optional: ImageMagick for spine color extraction

The CLI is plain Node.js and may work on macOS, but macOS is not currently tested or documented as a supported platform. The installer intentionally uses the Linux/XDG-style per-user layout under `~/.local`.

## Installation

Install the `bookshelf` command:

```bash
curl -fsSL https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh | bash
```

The installer follows the usual per-user Linux layout:

- `~/.local/bin/bookshelf` is the command you run.
- `~/.local/share/bookshelf` contains the installed application files that the command uses.

If `~/.local/bin` is not in your `PATH`, add it to your shell profile.

When developing locally before changes are pushed to GitHub, run the installer from the checkout instead:

```bash
./install.sh
```

Uninstall:

```bash
bookshelf uninstall
```

If the installed command is broken, run:

```bash
curl -fsSL https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh | bash -s -- --uninstall
```

## Viewing the Shelf

Create a bookshelf project:

```bash
bookshelf init ~/my-bookshelf
```

Then open the generated site:

```bash
cd ~/my-bookshelf
xdg-open public/index.html
```

You can also open `public/index.html` from your file manager.

## Project Layout

The installed CLI and your bookshelf project are intentionally separate.

The installed CLI lives here:

```text
~/.local/bin/bookshelf
~/.local/share/bookshelf/
```

A bookshelf project created with `bookshelf init ~/my-bookshelf` separates editable source data from publishable site files:

```text
~/my-bookshelf/
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

Run `bookshelf` from inside the project directory. For automation, pass the project path explicitly:

```bash
bookshelf --project ~/my-bookshelf validate
```

You can also set:

```bash
export BOOKSHELF_PROJECT_DIR=~/my-bookshelf
```

No command depends on a hardcoded project path.

## Publishing the Shelf

To publish the static bookshelf, upload these files and folders to your host:

```text
public/index.html
public/css/
public/js/
public/data/
public/fonts/
public/img/
```

Upload the contents of `public/` as your website root. Do not upload `library/`; it is local editable source data.

Do not upload the installed CLI files:

```text
~/.local/bin/bookshelf
~/.local/share/bookshelf/
```

## Managing Your Library

Run the interactive manager:

```bash
cd ~/my-bookshelf
bookshelf
```

The manager asks what you want to do:

- Build or refresh the library
- Add a new book
- Modify an existing book
- Remove a book
- Validate the library
- Apply manual cover files

`library/books.json` is the editable source of truth. `public/data/books.js` is generated for the frontend.

## Direct Commands

The guided workflow is intended for normal use, but direct commands are available for automation and future web tooling:

```bash
bookshelf build
bookshelf add
bookshelf update
bookshelf remove
bookshelf covers
bookshelf validate
```

Examples:

```bash
bookshelf add --title "Dune" --author "Frank Herbert" --isbn "9780441172719"
bookshelf build --fetch-covers
bookshelf update --id-or-isbn "9780441172719" --binding "Hardcover"
bookshelf remove --id-or-isbn "9780441172719"
bookshelf covers --id-or-isbn "9780441172719"
```

From outside the project directory, add `--project`:

```bash
bookshelf --project ~/my-bookshelf build
bookshelf --project ~/my-bookshelf add --title "Dune" --author "Frank Herbert"
```

## Covers

Published covers live in `public/data/covers`.

To override or add a cover manually, place an image in `library/manual-covers` using the book ISBN or id as the filename:

```text
library/manual-covers/9780441172719.jpg
```

Then apply that one manual cover and regenerate the frontend data:

```bash
bookshelf covers --id-or-isbn "9780441172719"
```

This does not fetch remote covers. It copies the matching image into `public/data/covers`, updates the cover path, and refreshes `public/data/books.js`.

To apply all matching manual covers:

```bash
bookshelf covers --all
```

`bookshelf build` is still available as a full-library consistency pass. It scans every book, applies manual-cover overrides, checks cover paths, fills missing spine colors when possible, and regenerates `public/data/books.js`.

To fetch missing ISBN covers from Open Library:

```bash
bookshelf build --fetch-covers
```

## License

Released under the [MIT License](https://github.com/aloglu/bookshelf/blob/main/LICENSE).

## Acknowledgements

I was inspired by [Marius Balaj](https://balajmarius.com/writings/vibe-coding-a-bookshelf-with-claude-code/)’s own bookshelf project.
