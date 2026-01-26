# Bookshelf

An interactive web-based library to showcase your book collection. You can interact with a demo page [here](https://aloglu.github.io/bookshelf).

## Features

- **Three Unique Views**:
  - **Shelf**: A realistic, scrolling bookshelf with physics-based momentum and tilt animations.
  - **Stacks**: A clean, grid-based list view for efficiency (mobile-friendly).
  - **Coverflow**: A coverflow view of your library.
- **Dynamic Physics**: Smooth scrolling and tilt mechanics that react to your scroll velocity.
- **Performance**: Implements lazy loading for covers and DOM elements, ensuring smooth performance even with large libraries.
- **Search & Filter**: Instant fuzzy search and filtering by author, title, or year.
- **Auto-Theming**: Extracts distinct color palettes from book covers to color-code the spine and details.
- **Data Pipeline**: Python and PowerShell scripts included to:
  - Read from a simple Excel spreadsheet (or JSON).
  - Automatically fetch high-res covers from OpenLibrary or Goodreads.
  - Extract spine colors for the UI.

## Getting Started

### Prerequisites
- A modern web browser.
- **Optional**: Python 3.x (with `urllib` standard lib) *or* PowerShell (Windows).
- **Optional**: Microsoft Excel (for managing the library easily).

### Installation

1. Clone or download this repository.
2. Open `index.html` in your browser.

## Managing Your Library

This project includes build scripts to generate the `books.json` and `books.js` data files from a source of truth (Excel or manual JSON).

### 1. The Source Data
The build scripts look for a file named `My Library.xlsx` in the root directory. If this file is missing, they will default to reading `data/books.json` directly.

**Excel Format** (`My Library.xlsx`):
Create an Excel file with the following headers:
- **Title** (Required)
- **Author**
- **ISBN** (Highly Recommended for fetching covers)
- **Translator**
- **Publisher**
- **Binding** (Hardcover/Paperback)
- **Published** (Year)

### 2. Building the Library
Run the build script to generate the web-ready data files and fetch missing covers.

**Using Windows (PowerShell):**
```powershell
.\build\build-library.ps1
```

**Using Python (Mac/Linux/Windows):**
```bash
python3 build/build-library.py
```

The interactive menus of both scripts provide the following options:
1. **Open Library**: Attempts to fetch covers from the Open Library API. This is the fastest method but may have lower resolution images or gaps in coverage.
2. **Goodreads**: Scrapes high-quality covers from Goodreads. This method is slower due to rate-limiting (to avoid being blocked) but generally yields the best aesthetic results.
3. **Offline Mode**: Useful for regenerating the `books.js` and `books.json` files from your source (Excel) without making any network requests. Use this if you just want to update metadata or titles.
4. **Apply Manual Covers**: Scans the `data/manual-covers` directory for images matching a book’s ISBN or ID and forcibly applies them, overriding any downloaded images.

### 3. Verification
Check `data/covers/` to ensure images were downloaded. You can override any cover by placing a `.jpg` in `data/manual-covers/` with the book’s ISBN as the filename.

## Acknowledgements

I was inspired by [Marius Balaj](https://balajmarius.com/writings/vibe-coding-a-bookshelf-with-claude-code/)’s own bookshelf project. This project too was mostly built by Codex and Gemini.
