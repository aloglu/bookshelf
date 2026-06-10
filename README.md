# Bookshelf

An interactive web-based library to showcase your book collection. You can interact with a demo page [here](https://aloglu.github.io/bookshelf).

## Features

- **Three Unique Views**:
  - **Shelf**: A realistic, scrolling bookshelf with physics-based momentum and tilt animations.
  - **Stacks**: A clean, grid-based list view for efficiency.
  - **Coverflow**: A coverflow view of your library.
- **Dynamic Physics**: Smooth scrolling and tilt mechanics that react to your scroll velocity.
- **Performance**: Lazy loading for covers and DOM elements.
- **Search & Filter**: Instant fuzzy search and filtering by author, title, or year.
- **Auto-Theming**: Extracts cover colors to color-code book spines and details.
- **Guided Library Management**: A Linux CLI for building, adding, editing, removing, and validating books.

## Requirements

- Linux
- Node.js 18 or newer
- Optional: ImageMagick for spine color extraction

## Installation

Install the `bookshelf` command:

```bash
curl -fsSL https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh | bash
```

If you prefer `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh | bash
```

The installer places the project in `~/.local/share/bookshelf` and creates `~/.local/bin/bookshelf`.
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

Open `index.html` in your browser.

## Managing Your Library

Run the interactive manager:

```bash
bookshelf
```

The manager asks what you want to do:

- Build or refresh the library
- Add a new book
- Modify an existing book
- Remove a book
- Validate the library

`data/books.json` is the editable source of truth. `data/books.js` is generated for the frontend.

## Direct Commands

The guided workflow is intended for normal use, but direct commands are available for automation and future web tooling:

```bash
bookshelf build
bookshelf add
bookshelf update
bookshelf remove
bookshelf validate
```

Examples:

```bash
bookshelf add --title "Dune" --author "Frank Herbert" --isbn "9780441172719"
bookshelf build --fetch-covers
bookshelf update --id-or-isbn "9780441172719" --binding "Hardcover"
bookshelf remove --id-or-isbn "9780441172719"
```

## Covers

Existing covers live in `data/covers`.

To override a cover manually, place an image in `data/manual-covers` using the book ISBN or id as the filename:

```text
data/manual-covers/9780441172719.jpg
```

Then run:

```bash
bookshelf build
```

To fetch missing ISBN covers from Open Library:

```bash
bookshelf build --fetch-covers
```

## License

Released under the [MIT License](https://github.com/aloglu/bookshelf/blob/main/LICENSE).

## Acknowledgements

I was inspired by [Marius Balaj](https://balajmarius.com/writings/vibe-coding-a-bookshelf-with-claude-code/)’s own bookshelf project.
