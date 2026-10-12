# Bookshelf 2.0 — plan and decisions

This folder holds the agreed direction for the 2.0 rework. Read this file first when picking the work back up.

- `prototype.html` — the clickable design study (desktop and mobile, day and evening). Open it in a browser. It is a design reference, not app code: its data is built in and every server action is acted out.
- Live copy: https://claude.ai/artifact/4oRGjkGhDJNBr5hSA8Qm9t

## Ground rules

- 2.0 is a clean break. Bookshelf 1.x was a terminal app that built a static website; 2.0 is a self-hosted web app (a server in Docker, or a plain program) that serves the shelf and its admin. Nothing of 1.x carries over. The owner's 1.x library is moved over once by a script kept outside the repository (decided 2026-10-11: 1.x had no other users, so 2.0 holds no code for it).
- No dead code. Not carried over: the terminal interface, the static-site build, the shelf/stack/coverflow view switch, scroll speeds, the random-book button, the footer setting, the site subtitle ("tagline"). Old code may be reused where it is good (covers, thumbnails, spine colours, archives, import/export, backups), but only what 2.0 uses.
- Runs anywhere Docker runs (any NAS, Unraid, Synology, a VPS, a Raspberry Pi) and without Docker on Linux, macOS and Windows. Nothing may assume one platform.
- The website's copy is in American English.
- Never write to the owner's real 1.x library (`~/.local/share/bookshelf`); test against copies.

## Architecture (agreed 2026-10-12)

- **Server:** Go, one program. Serves the site, the admin and a JSON API from the same address. Builds into a small Docker image that runs as a non-root user (`PUID`/`PGID`).
- **Storage:** SQLite in one file, through a pure-Go driver (no cgo, so cross-builds stay simple). Covers and backups are plain files beside it. Everything lives in one data folder (`/data` in Docker).
- **Frontend:** a hybrid.
  - The shelf and the open book stay as a self-contained plain-JavaScript engine, ported from the study. It creates each spine once and only moves it, runs the flights with hand-built Web Animations, keeps text-bearing elements off compositor layers, and moves the eye with the book. A framework must never re-render these elements.
  - Everything around them is Svelte (with Vite): the header, settings, Add a book, the edit form, the stats page, sign-in, cards, sheets, toasts. They talk to the engine through a small explicit interface (open a book, refresh a spine, re-lay the shelves, put the book back).
  - Node is needed only to build the frontend; the Docker image builds it once and runs Go alone. The built files are embedded in the Go program.
- **Sign-in:** like Inkubator. `BOOKSHELF_ADMIN_USER` (default `admin`) and `BOOKSHELF_ADMIN_PASSWORD` in the container's settings. Week-long session per device (HTTP-only cookie). Lockout for 15 minutes after repeated wrong passwords. The password changes by changing the variable and restarting. The server refuses to start without a password.
- **Privacy is enforced by the server**, not by hiding controls: hidden books, and everything when the library is private, never leave the server for visitors.
- **Go** is chosen on the merits (decided 2026-10-12), not to match another project: a small fast HTTP server and image work (covers, thumbnails, spine colours) from the standard library, one self-contained cross-built program for every platform and Docker, and pure-Go SQLite. Reusing the good 1.x code is a bonus.

## Storage safety on Unraid (lesson from Inkubator 3.0.1)

On Unraid a data folder under `/mnt/user` goes through `shfs`, a FUSE layer with few worker threads. Inkubator 3.0.0 took a blocking file lock on every request; a burst of requests (a page of thumbnails) left every `shfs` worker waiting on the lock while the holder needed one to read its file, so `/mnt/user` deadlocked and Docker, the Unraid interface and network shares hung with it (Inkubator commit 68e499b). SQLite by default locks on every transaction, and its WAL mode needs a shared-memory file that FUSE and network filesystems handle badly, so Bookshelf must be built so this cannot happen:

- **One lock, taken once.** The database is opened at startup with `locking_mode=EXCLUSIVE` and that lock is held for the life of the program; no request ever waits on a file lock. A second Bookshelf on the same folder is refused at once with "another Bookshelf is already using …".
- **No shared-memory file.** In exclusive mode SQLite keeps the WAL index in heap memory; no `-shm` file is used. (Exclusive mode is set before the first access to the WAL database, and the journal mode is chosen accordingly; verify with a test.)
- **Requests don't touch the database.** The visitor-facing library (and the owner's) is kept in memory and rebuilt after a change; changes take turns on an in-process mutex and are written in one transaction. Covers and thumbnails are plain file reads.
- **Test:** hundreds of concurrent public and signed-in requests against a scratch library, asserting every answer succeeds and the process holds exactly one file lock throughout.
- **Docs:** the Unraid guide suggests `/mnt/cache/appdata/bookshelf` (or an exclusive appdata share), which bypasses `shfs`; troubleshooting explains the "already using" message. A hand edit of the database while Bookshelf runs is not supported (restore a backup instead).

## Design decisions (settled in the study)

**The shelf**
- One view on desktop and phone: thin, rounded wood planks on a coloured wall, spines coloured from their covers.
- Spine width follows the page count (`max(18, min(80, pages × 0.085 + 7))`), so the thinnest spine is about 21px. Every spine shows its title and its author's surname; the author line shortens before it can crowd the title.
- Spine height follows the title's length (`196 + 3.1 × characters`, clamped 222–280) with a few units of per-book irregularity; hardcover and cloth ×1.07, leather ×1.05. Real dimensions are not used (rarely in the catalogues).
- Bindings: paperback, hardcover, cloth, leather. There is no mass-market binding: mass market counts as paperback. Each binding keeps the spine in its cover's own colour; only texture and details differ (hardcover: rounded with hinge creases and board edges; cloth: woven texture, gilt lettering; leather: grain, gilt lettering, raised bands at head and tail, lettering kept clear of the bands). Can be turned off (Look → Show bindings on the spines).
- Hover: the spine rises a few whole pixels with a deeper shadow and a light, the same for every book wherever it stands. Position only, never a transform or opacity change on the spine (those make the lettering re-settle). Instant, no intent delay.
- Click: the book flies off the shelf, turns to face you and opens to a title page. Put back retraces the same path. Esc mid-flight turns the flight round; it is never queued.
- Select mode: chosen books show a slip of paper rising out of their top, plus an inner ring. Books never move.
- The book being read hangs a ribbon from the foot of its spine over the plank, and the ribbon rises with the spine on hover.
- Optional bookcase (off by default): sides with a base board, top, back panel, each on its own; all in the shelf wood.

**The open book**
- Left board: the cover plate in the cover's own proportions, the endpaper tinted from the spine colour. Right page: set like a title page (running head with binding, title sized to its length, subtitle split at the first ":", a lozenge in the book's colour, author in small caps, a dotted-leader colophon, the actions, a folio).
- Actions: Copy link, Search on … (one source: a "Search on X" action; several: a "Search on…" row that swaps in place, sharing one slot with the actions and the Remove question so nothing moves), and for the owner Edit, Reading now/Finished reading, Hide from website, Remove (asks on the page, then Undo).
- Edit happens on the title page itself: the same layout as fields; ISBN checksum and year/pages validation; Enter saves, Esc cancels. Cover tools on the inside board (replace cover, spine colour by hand, back to the cover's colour); on a phone a "Replace cover" bar sits on the cover at the head of the page.
- On a phone the open board runs off the left edge and the cover appears small at the head of the page; tapping it zooms it, tapping again puts it back.
- Books opened from the stats page open there: they rise from their cover, with no Previous/Next, and go back to it.

**Header**
- The site name (a link home, accent colour on hover) and, beside it, the tools in a fixed place: Search, sort (Author/Title/Year), day/evening, Stats, and for the owner Select, Add book, Settings, Sign out; for visitors Sign in. Search is an icon that grows into a field on click/tap (never hover) as an overlay toward free space, so nothing moves; Esc clears, then folds; Enter on an empty field folds.
- Beneath: "Now reading <title> by <author>" with a tiny cover on the left, the owner's social links on the right.
- On a phone: the title with Add book (owner, a plus), Search, and a menu button on one line; the menu sheet holds sort, day/evening, Statistics, Select, Settings, Sign in/out.

**Adding a book**
- ISBN lookup (Open Library, then Google Books) or a barcode scan (the camera, reading by itself). Duplicates are caught ("Show it"). Unknown or invalid ISBNs offer entry by hand. A confirm card with the cover and every field editable, then the book flies into its place on the shelf.
- A book without a cover gets a plain cloth cover with its title stamped on.
- ISBNs are normalised on entry (labels and punctuation stripped, ISBN-10 converted to ISBN-13), stored as bare digits and always shown hyphenated by the ISBN agency's range table (the server ships RangeMessage.xml).
- Periodicals (decided 2026-10-11): some periodicals publish book-like issues, which carry an ISSN rather than an ISBN. The field is "ISBN or ISSN" and takes either, or a periodical's 977 barcode (read back to its ISSN). An ISSN is checked by its own check digit and shown as `ISSN 1300-2880`; such a book also has an issue (as printed, `70–71`). The same issue can't be shelved twice; other issues of the same periodical can. An ISSN can't be looked up, so it goes straight to entry by hand with the ISSN filled in. Books with an ISSN take their address from the title.

**Stats page** (`#stats`, the Stats icon; off for visitors if the owner turns it off)
- Now reading first, under its own heading: cover, title, progress (share and page on the left, the owner's page field on the same line at the right, the bar beneath), and four facts (days since starting, pages to go, share of the library that is shorter, share that is older).
- The library in numbers: five uniform tiles (books, pages, authors, average length, years written), then books by decade (only decades that have books, labels fitted to the room), by page band, by binding, in translation, authors with two or more books, top publishers, and the oldest/newest/longest/shortest books.
- One accent hue, hairline grids, a Table view for every chart, hover readouts that only repeat what the table holds.

**Reading** (designed 2026-10-11, study v47–48)
- Any number of books can be read at once, each with its own ribbon. The header names one book with its author, two by title ("Now reading A and B"), more as "A and 2 more", with their small covers fanned; with several, it opens a short list (cover, title, author, share read) to choose from. The stats page has a "Now reading" card per book, one beneath another (side by side broke the card's facts row).
- On a book's page (owner): "Reading now" and "Mark as read" when it isn't being read; "Finished reading" and "Set aside" while it is. Finishing offers "Add a note" in its toast; setting aside can be undone.
- Mark as read: a small card (like sign-in) asking when it was finished (on a day, in a year, or "I don't remember") with an optional note and its public switch.
- The line above the title reads "Paperback · Read twice" (or "Readings"), which opens the book's Readings card: each reading newest first (the one in progress, then finished and set-aside ones) with its dates, how long it took, where a set-aside reading stopped, and a finished reading's note; each can be corrected (how it ended, when it started, when it ended as precisely as known) or deleted. This keeps the book's page from growing.
- Notes are private unless made public, per note (decided 2026-10-11). A public note is shown to visitors at once: their line above the title gets "A note" (or "2 notes"), opening a read-only card with each public note and the day the book was finished, nothing else of the reading. The library view carries each book's public notes for this. The notes blog (later) will gather the same public notes.

**Settings** (a drawer, tabs Site / Look / Library / Account). Every change saves at once and a quiet "Settings saved" pill says so.
- Site (regrouped 2026-10-12): Header (name, which takes inline Markdown; icon; icon beside the name; social links: Goodreads, The StoryGraph, Instagram, no others); Footer (on/off and its Markdown text); Visitors (private library: visitors see only a sign-in page; stats page on/off; a line on what is never shown); Book pages (each book's address `/9780679723424`, `/978-0-679-72342-4` or `/pale-fire`; "Search on" sources: Open Library, Google Books, Goodreads).
- Look: When it opens (default sort and its direction; day/evening/system); Walls and wood (wall colour for day and for evening with preset pairs; wood for day and evening: oak, walnut, maple, ash, pine, cherry, teak, mahogany, ebony, used by planks and bookcase alike; bookcase and its parts); Books (bindings on/off, book size, tallest spine, reading ribbon colour).
- Library: import (CSV, JSON, a Bookshelf archive), export (CSV, JSON, full archive with covers), backups (how often, how many to keep, back up now, restore). Backups live in the data folder.
- Account: signed in as …; the password is changed in the container's settings.

**Interaction rules (both platforms, one system)**
- Layers (the open book, cards, sheets, the drawer): while one is open the page beneath doesn't scroll (CSS only, nothing pinned or measured); backdrops ignore swipes; each layer keeps its own scrolling.
- Typing on a touch screen: nothing focuses a field for the reader; tapping a field's caption doesn't select it; tapping anything that isn't a field puts the keyboard away; the field in use comes up to the top of its layer (the book's page, the drawer), with room beneath it only while typing. Cards are bottom sheets that keep their own size. Fields are at least 16px on phones (no iOS zoom). Hover effects only where a pointer can hover.
- Focus returns to a spine after closing a book only for keyboard users.

**Typography:** Fraunces (display) and Instrument Sans (interface). Fraunces' optical size is capped by size so thin strokes survive on 1× screens (site name 120, title page 96, longest titles and card headings 72).

## Data model (draft for review)

`books`
| Field | Notes |
| --- | --- |
| id | stable id |
| title, subtitle-free title | the title as entered; the title page splits at ":" |
| author, translator, publisher | text |
| published | year (integer) |
| pages | integer; sets spine width |
| binding | paperback, hardcover, cloth, leather |
| isbn | bare ISBN-13 digits, unique when present |
| issn, issue | a periodical's ISSN (bare eight characters) and its issue; the pair is unique; never with an ISBN |
| slug | from the title, unique (`-2`, `-3` on clashes) |
| hidden | hidden from visitors |
| cover | file in the data folder, with a thumbnail; null for a plain cloth cover |
| spine_color, spine_text_color | worked out from the cover on the server |
| spine_color_override | the owner's own colour, or null |
| added_at, updated_at | timestamps |

No edition field: the ISBN identifies the edition (decided 2026-10-12).

`readings` — every time a book is read (decided 2026-10-12: keep as much reading data as possible; the stats page can choose what to show)
| Field | Notes |
| --- | --- |
| id | |
| book_id | the book |
| started_at | when "Reading now" was chosen |
| ended_at | when it was finished or set aside; null while reading |
| outcome | reading, finished, set aside |

Several books can be read at once (decided 2026-10-11), each with its own ribbon; a book has at most one reading in progress, so "Reading now" on a book already being read changes nothing. Re-reads are separate readings. How long a book took is `ended_at − started_at`.

**Mark as read** (decided 2026-10-12): any book can be marked as read without having been tracked, since most books on a shelf were read before Bookshelf or without "Reading now". It creates a finished reading whose dates are as precise as the owner gives: a date, only a year, or none ("I don't remember"). So the dates are stored with a precision (`day`, `year`, `unknown`), and untracked readings have no page log. Such a reading counts as read (and can carry a note) but is left out of pace and time-to-finish figures; one with a year still counts toward that year. A book is "read" when it has at least one finished reading.

**Re-reading** (decided 2026-10-12): a book can be read any number of times. "Reading now" on a book already read starts a new reading; "Mark as read" on a book already marked adds another. Earlier readings are never changed by a new one. Each reading keeps its own dates, page log and note. Any past reading can be edited (dates, outcome) or deleted, to correct mistakes. Stats can count both distinct books read and readings (so re-reads are their own figure), and a book's page can show how many times it was read with each reading and its note.

`reading_progress` — the page log of a reading: reading_id, page, at. Written whenever the owner sets their page, so pace (pages per day), days actually read and progress over time can be worked out later. The current page is the latest entry.

`notes` — the owner's thoughts on a book, one per finished reading, tracked or marked as read (a re-read keeps its own): reading_id, text, visibility (private by default, or public), created_at, updated_at. Notes exist only for books that have been read. A public note is shown to visitors on its book's page; public notes are also the material for a simple blog section planned for later (see Follow-ups).

`settings` — one row of key/value pairs for everything in the Settings drawer.
`sessions` — signed-in devices (token hash, created, last seen, expires).
`sign_in_attempts` — for the lockout.
`backups` — recorded in the data folder as files; the list is read from there.

## API (draft for review)

Public (what visitors may see; the server filters hidden books and refuses everything when the library is private):
- `GET /api/library` — books, the public settings and the current reading.
- `GET /covers/<id>` and `/covers/<id>/thumb`.

Owner (signed in):
- `POST /api/session`, `DELETE /api/session`.
- `POST /api/books`, `PATCH /api/books/<id>`, `DELETE /api/books/<id>`, batch hide/show/remove.
- `PUT /api/books/<id>/cover` (upload), `DELETE /api/books/<id>/cover`.
- `GET /api/lookup?id=` (Open Library, then Google Books, through the server; an ISSN answers `periodical: true` with the ISSN read). Books are sent with one `identifier` field (ISBN or ISSN) and an `issue`; they come back with `isbn`, `issn`, `issue` and `identifier` (as shown).
- `POST /api/readings` (start reading a book, or mark it as read with an optional date or year), `PATCH /api/readings/<id>` (set the page, finish, set aside, correct dates), `DELETE /api/readings/<id>`, `GET /api/readings` (the history, optionally for one book).
- `PUT /api/readings/<id>/note`, `DELETE /api/readings/<id>/note` (text and visibility; finished readings only).
- `GET/PATCH /api/settings`.
- `POST /api/import` (a Bookshelf archive, CSV or JSON; answers with how many books were added and skipped), `GET /api/export?format=csv|json|archive`.
- `GET /api/backups`, `POST /api/backups`, `POST /api/backups/<name>/restore`.

## Documentation (agreed 2026-10-12, after Inkubator's)

Guides for people running Bookshelf, step by step, saying what they should see, also for someone who has never used Docker:
- `README.md` — what it is, the quickest start, links.
- `docs/README.md` — choose where Bookshelf runs (a table by situation), then using it.
- One guide per home: Unraid, Synology, Raspberry Pi, Docker, without Docker.
- Using Bookshelf, remote access (HTTPS), backups, updating, troubleshooting, building from source.

## Phases

1. Data model and API: reviewed and settled in this file.
2. The server: storage, sign-in, the API, covers and spine colours, the Docker image.
3. Moving the owner's 1.x library over (covers included, page counts looked up), tested against a copy, by a one-time script outside the repository.
4. The frontend: the engine ported from the study, the Svelte surfaces around it, everything the study acted out made real.
5. Hardening: tests, a code-quality review, the guides.

## Progress

- 2026-10-12: design study finished (`prototype.html`). Architecture, storage (SQLite) and frontend split (plain-JS engine + Svelte) agreed. `v2` branch started.
- 2026-10-12, phase 2 begun. The 1.x terminal app (`cmd/bookshelf`, `internal/tui`, the `bookshelf` wrapper script) is removed; `internal/library` and `internal/siteassets` stay only until the 1.x library has been moved over. New packages, each with tests:
  - `internal/store` — SQLite (modernc.org/sqlite), one connection, `locking_mode=EXCLUSIVE` + WAL taken at open with `busy_timeout(0)`; whole library held in memory as an immutable `Snapshot`; changes take turns on a mutex. Books (unique slug and ISBN), readings (one in progress per book, re-reads, mark as read with day/year/unknown precision, page log, corrections), notes (finished readings only), settings (validated), sessions (touched at most hourly; sliding week). Tests prove a second open is refused at once, no `-shm` file exists, and a single file lock is held while 200 readers run against 40 writes.
  - `internal/isbn` — normalize (labels, punctuation, ISBN-10 → 13), checksums, hyphenation from the embedded RangeMessage.xml (all groups, 978 and 979).
  - `internal/covers` — decode with size limits, WebP detail (900px) and thumbnail (360px) written atomically under a fresh name per cover, spine colour by the study's edge median, lettering colour.
  - `internal/auth` — env user/password (SHA-256 + constant-time compare), sessions in an HTTP-only SameSite=Lax cookie, lockout per client address (5 in 15 minutes) with the address taken from X-Forwarded-For only behind `BOOKSHELF_TRUSTED_PROXIES`, a global slow-down instead of a global lockout, pruned failure records.
  - `internal/lookup` — Open Library (edition + search index; edition title kept unless the index has the same title with better capitals or without a trailing series) then Google Books (optional `BOOKSHELF_GOOGLE_API_KEY`); covers fetched only over HTTPS from Open Library, archive.org and Google Books hosts, redirects checked too.
  - `internal/server` — the API from the plan, visitor and owner views built once per change, hidden books and their covers never served to visitors, a private library refusing visitors everything, same-origin check on every change, all-or-nothing settings patches.
  - `cmd/bookshelf` — the program: flags/env, default data folder `com.aloglu.bookshelf` in the system's data folder (never 1.x's), graceful shutdown, hourly session pruning, refuses to start without a password before touching anything, embeds the built frontend from `cmd/bookshelf/webdist` when present.
- Checked by hand against a scratch folder: sign-in, live lookup, adding a book with its Open Library cover, a second instance refused, and one database lock held before and after 300 parallel requests.
- 2026-10-12, phase 2 finished (code; the Docker image is not yet built and run here, see Follow-ups):
  - `internal/archive` — the `.bookshelf` format shared by backups and full exports: a zip with `manifest.json` (kind, format 1, version, date, book count), `library.json` (books, readings with page logs and notes, settings; never sessions) and `covers/`. Entries carry real times. Reading is defensive: covers only as plain `.webp` names under `covers/`, an entry count and total-size cap, newer formats refused. CSV/JSON exports carry books only (CSV with a byte-order mark and hyphenated ISBNs); CSV import finds columns by header in any order (title required), reads `="…"` ISBNs, and maps common other names (ISBN13, Year, Number of Pages).
  - `internal/backup` — backups in `/data/backups` named `bookshelf-<UTC time>-<kind>.bookshelf` (automatic, manual, before-import, before-restore), written to a temporary file then renamed, newest N kept. Restore replaces everything in one transaction after a before-restore backup and leaves the library as it was on any failure; import merges (a book is already there with the same ISBN, or without one the same title and author), brings readings and notes with it, keeps a book to one reading in progress, and leaves the library's settings alone. Scheduled backups (day/week/never, the first once there are books) and an hourly sweep of covers no book uses older than a day.
  - Server routes: list/create/download/restore backups, restore from an uploaded archive, import (uploads go to a temporary file, never held whole in memory), export CSV/JSON/archive (the archive streamed). Problems with the owner's file come back as plain messages.
  - `cmd/bookshelf` — `bookshelf health` for the image's health check; on Linux, started as root (as in Docker) it refuses PUID/PGID 0, gives the data folder to PUID:PGID changing only what isn't theirs, and becomes that user before opening anything. Cross-builds checked for Linux amd64/arm64, macOS arm64 and Windows.
  - `Dockerfile` (static Go build on Alpine with CA certificates and time zones, `/data` volume, health check), `.dockerignore`, `docker-compose.example.yml`.
- Smoke-tested the program again: health check, the automatic first backup at startup, a manual backup, and a full export holding the library and both cover files.
- 2026-10-11, phase 3 (code done, not yet committed):
  - Periodicals: `internal/isbn` reads ISSNs (check digit, 977 barcodes) and parses the one "ISBN or ISSN" field; books have `issn` and `issue`, unique as a pair; CSV/JSON carry `issn` and `issue` columns (an ISSN in another program's ISBN column is recognized); imports match an issue by ISSN and issue.
  - Imports read from the uploaded file rather than memory (as the plan always said); imports and restores get an hour's deadline instead of the server's usual five minutes; an archive whose covers were all present no longer triggers a second backup.
  - The 1.x library moves over by a one-time script outside the repository (`~/workspaces/bookshelf-1x-move/move.py`), which only reads the 1.x archive and adds everything through 2.0's API, so 2.0 checks each book as if added by hand: it looks up each page count (2.0's lookup), adds the book (bindings, visibility, years; issues 1.x kept in the ISBN field, `977-1300-2880-70-71` or `1300-2880-76`, as ISSN and issue; invalid identifiers left off and listed), uploads its cover (2.0 makes the WebP and the spine colour) and, into an empty library, brings the 1.x settings 2.0 still has (name, sort, link style, search sources with Open Library in Wikipedia's place, stats). Books already on the shelf are skipped, so it can be run again. `internal/library` and `internal/siteassets` (1.x) are removed.
- 2026-10-11, phase 4 begun: the frontend in `web/` (Vite + Svelte 5), built into `cmd/bookshelf/webdist` and embedded in the program.
  - `web/src/engine/` is the study's shelf and open book as plain modules: spines made once and only moved (FLIP), the hover, the flights and their clip, the title page and its edit form, stepping, the phone cover zoom, and the look (walls, woods, bookcase faces, ribbon, theme). Spine colors come from the server; a book without a cover gets the stamped cloth cover drawn in the browser.
  - `web/src/ui/` is everything around them in Svelte: header (search, sort, tools, the Now reading line and its list, links elsewhere), stats page (the study's chart code), selection bar, phone menu, sign-in, the Readings and Mark-as-read cards, Add a book, and the Settings drawer. `web/src/lib/` holds the API client, the app state and actions (every change goes to the server, then the library is fetched again and the shelf and open book update in place), panels with the one set of touch-typing rules, toasts and dates.
  - The study's stylesheet is used whole; the demo panel and its reduced-motion toggle are gone (the system setting remains). Fraunces and Instrument Sans are bundled (with the Turkish letters), never fetched from Google.
  - Addresses: `/` the shelf, `/stats`, `/signin`, and each book's own address (in the owner's chosen style) opens it; opening a book shows its address.
  - Barcode scanning is real: the browser's own reader where it has one, otherwise ZXing (WebAssembly, served by Bookshelf, loaded only when scanning). The camera needs https (or localhost); the card says so otherwise.
  - Changes found on real data: spine lettering is set sideways so curly apostrophes don't stand upright; books without a page count are left out of page figures (and the tiles say so); the translation line no longer assumes English; the binding list has no "Other"; the edit form and Add a book take "ISBN or ISSN" with an Issue field for periodicals; backups can be downloaded.
  - The server now accepts social links only as https addresses on their own sites (a `javascript:` link would otherwise reach visitors).
  - `npm run dev` in `web/` serves the frontend and passes `/api` and `/covers` to a Bookshelf on port 8080 (or `BOOKSHELF_DEV_SERVER`). The Docker image builds the frontend in a Node stage.
  - Tried against a scratch copy of the owner's library (496 books): the shelf, a flight and the title page, sign-in, Settings, Reading now / Finished / Add a note, Mark as read with a year, editing (a bad ISBN refused with the server's message, a page count changing the spine), the stats page, and a phone.
- 2026-10-11: Settings → Library can restore from a .bookshelf file (with the same question as restoring a backup; what is there is backed up first). Download links look like the buttons beside them.
- 2026-10-11, the Docker image built and run here (38.8 MB): healthy within seconds; the program runs as PUID:PGID and its files belong to them; a root-owned data folder is handed to PUID:PGID; `PUID=0` and a missing password are refused with plain messages; `--user` works; the app, its assets and sign-in are served; the scratch library imported through the container shows its 496 books and covers.
- 2026-10-11, phase 5 begun: a security pass. Sign-in now counts an attempt before checking its password, in one step, so a burst of attempts from one address can't all slip past the lockout while the global slow-down holds them (a test sends 50 at once and sees exactly 5 tried; it fails on the old code). Every answer carries a Content-Security-Policy: only the app's own scripts (and the barcode reader's WebAssembly), connections to this server only, covers from here or https; checked in the browser with the shelf, a flight, the wood textures, Add a book and the barcode reader. Reviewed and sound: the frontend never puts the owner's text into markup, cover downloads only from the lookup's https hosts (redirects too), archive names and sizes on import, backup names, JSON bodies capped at 1 MB.
- 2026-10-11, the guides: `README.md` and `docs/` (choosing a home, Docker, Unraid with `/mnt/cache/appdata/bookshelf`, Synology, Raspberry Pi, without Docker, using Bookshelf, putting the shelf online with Cloudflare Tunnel, a reverse proxy or Tailscale and `BOOKSHELF_TRUSTED_PROXIES`, backups and importing (Goodreads exports included), updating, troubleshooting with the program's own messages, building from source). Every link and anchor checked; the documented compose file run as written. `docker-compose.example.yml` binds to this computer only unless `BOOKSHELF_BIND_ADDRESS` says otherwise.
- 2026-10-11, release and CI replace 1.x's: CI builds the frontend, checks gofmt, vet and tests, serves the frontend from the program, and smoke-tests the Docker image (`docker/smoke-test.sh`: refusals, health, sign-in, a book with a cover, a backup, a restart keeping everything, the data owned by PUID:PGID, a clean stop). A `v*` tag (matching `web/package.json`, now 2.0.0) publishes images for amd64 and arm64 to `ghcr.io/aloglu/bookshelf` (`:2.0.0`, `:2`, `:latest`), programs for Linux, macOS and Windows on amd64 and arm64, and a draft release with checksums. The packaging step was run here. 1.x's `install.sh`, its browser smoke script and its ignore entries are gone.
- 2026-10-11: the origin check trusts Sec-Fetch-Site first (pages can't forge it, and it holds behind any proxy, whatever the proxy does to Host); browsers without it are checked by Origin against Host as before.
- 2026-10-11: `bookshelf set-password` (decided with the owner as the cheapest way to give the plain program a password): asks twice without echo (or reads a line from a pipe), at least 8 characters, and keeps a salted PBKDF2-SHA256 hash (600,000 rounds) in `password` in the data folder, readable by its owner only. `BOOKSHELF_ADMIN_PASSWORD` still wins when set, so Docker setups are unchanged; with neither, the program refuses to start before touching anything. The plain programs are a free extra, not a focus: Docker is the main way to run Bookshelf.
- 2026-10-11, the icon (chosen by the owner from six concepts and two rounds of changes, artifact https://claude.ai/artifact/RSPeSdoGw3jxnRSFfgyAdb): "Come forward, reading". On a near-black tile (#17110d), a row of spines on an oak plank (terracotta, green, navy, gold), the middle one raised: cream, lettered as on the shelf (the title down its length, the surname small at the foot), with the red reading ribbon (#a52a3b) hanging from its foot over the plank, the only red in it. `web/public/icon.svg` is the master; `favicon.svg` (and `favicon-32.png`) leave out the lettering, which would only smudge at tab sizes; `apple-touch-icon.png` is square (iOS rounds it); `icon-192/512.png` and a maskable 512 (books inside the safe zone) are in `manifest.json` (standalone, so the home-screen Bookshelf opens like an app). The Unraid guide gives `icon-512.png` as the Icon URL.
- 2026-10-12, CI and the release workflow ran on GitHub for the first time (the `v2` push, and the release run by hand without a tag): every check, the smoke test on amd64 and arm64, and all six programs passed; publishing was skipped, as it should be without a tag.
- 2026-10-12, knowing about updates (as Inkubator does): `internal/updates` asks GitHub's "latest release" (drafts and pre-releases are never it) at start and every 12 hours, only the version, with Bookshelf's version as its user agent; development builds are never checked. Settings → Account → Updates shows the version and, when a newer one is out, What's new (the release page) and How to update (the guide), with a dot on the Settings icon, the phone's menu button and the Account tab. "Check for updates" is on by default and turning it on looks at once; it is owner-only, like `/api/updates`. The updating guide covers every setup, not one in particular.
- 2026-10-12, **Bookshelf 2.0.0 released** (https://github.com/aloglu/bookshelf/releases/tag/v2.0.0): `v2` fast-forwarded into `main`, tag `v2.0.0`, images on `ghcr.io/aloglu/bookshelf` (`2.0.0`, `2`, `latest`; public, inherited from the repository) and programs for Linux, macOS and Windows. Releases are titled by their tag (`v2.0.0`), the owner's convention; the notes are written for the release page, which is what Settings' What's new opens. The camera over https is untested (the owner will report if it fails).
- 2026-10-12, after the owner's first days with 2.0.0 at https://books.alog.lu:
  - The phone's search opens on a tap again (the touch rule that keeps a field's caption from opening the keyboard caught the search box, whose caption is its magnifying glass) and stands first: search, Add book, menu.
  - Sorting either way: choosing the active sort again turns it round (A to Z / Z to A, oldest / newest first), shown by a small arrow on the active button; every sort button keeps room for it, so the header never shifts. Books without a year come last both ways. The owner's default sort is still ascending.
  - Sign-in opens with an empty user name.
  - Covers on slow connections: pointing at a book (or a finger on it) starts its covers loading; the open book shows the small cover if the full one hasn't arrived and swaps it in when it has, so a first opening never shows a blank cover. The stats page's small images use the thumbnails.
  - A footer under the shelves: "Made by alog.lu · Code available on GitHub", linking to alog.lu and the repository. No version in it (it would tell anyone which version runs).
  - The icon beside the library's name in the header (Settings → Site → Show the icon beside the name, on by default), and the owner's own icon: any picture, cropped to its middle square and written as PNGs at 32, 180, 192 and 512 under a new name each time. The server writes the chosen icon (and the library's name) into the page and into a generated `manifest.json`, since tabs and home screens read them before any script runs; Use Bookshelf's goes back and deletes the files.
  - A test that judged covers by a fixed clock (2026-10-12 09:00) started failing once that time had passed; it now starts from the real time.
  - Released as **2.0.1**, a bug-fix release. The per-book notes page (notes and highlights as entries of their own, the readings beside them) is being designed in the study and will come as 2.1.0.
- 2026-10-12, for **2.0.2** (from the owner's phone):
  - Day or evening chosen with the header's button (or the phone menu) no longer flips back after a change such as setting the page: every reload re-applied the library's setting. The setting is now applied when the library first loads and when it changes.
  - Choosing books on a phone: the bar is one row across the foot of the screen (its buttons used to wrap into a box); while asking to remove, the question takes the full width.
  - Settings → How it opens keeps the sort's direction too (`sortDesc`): choosing the active sort again turns it round, as on the shelf, and visitors start that way.
  - The footer can be hidden and rewritten (`footer`, `footerText`, starting as the original credit), and the library's name can hold Markdown, so it can link elsewhere (the icon still leads home). Both take inline Markdown only: links (web, email, this site), bold, italic, code, and new lines in the footer; everything else is escaped. The page title and the web app manifest get the name as words (`store.PlainText`); a name of formatting alone is refused. The name may be 200 characters, room for a link.
  - Settings regrouped, each section with a heading and short descriptions. Site: Header (name, icon, icon beside the name, Elsewhere links), Footer, Visitors, Book pages (address style, Search on sources). Look: When it opens (sort and its direction, day or evening; moved from Site), Walls and wood (walls, wood, bookcase), Books (bindings, size, tallest spine, ribbon). Library and Account unchanged.

## Follow-ups

- **Lookup data quality:** Open Library's "first published" year is sometimes wrong (Pale Fire shows 1945; it is 1962). The owner corrects it on the confirm card; consider preferring the edition's own year when the two disagree badly.
- **A Privacy tab** (considered 2026-10-11, not taken): visitor-facing privacy is two settings plus per-book and per-note switches, grouped under "What visitors see" in Site. Give it its own tab if the notes blog brings more.
- **Notes blog (later, decided 2026-10-12):** a very simple section where the owner publishes notes, chosen per book; notes can also stay private to the owner. Not part of 2.0's first release unless decided otherwise.
- **Read dates from imports (possibility, not committed):** a CSV import (for example a Goodreads export, which carries read dates) could mark books as read in one go.
- **Stats from reading history (later):** books finished per year/month, time to finish, pace, re-reads. The data is kept from day one so these can be added at any time.
- **Google Books without an API key** (found moving the 1.x library, 2026-10-11): after a few dozen lookups it answers 429 Too Many Requests, so books Open Library doesn't know (many Turkish editions) find nothing, and the error then reads as if the sources were down. The guides should recommend `BOOKSHELF_GOOGLE_API_KEY` (free), and a 429 should read as "busy, try again in a minute" rather than unreachable.
- **Phase 4, still to do:** testing on the owner's devices (the owner tried it on this computer and an iPhone over the home network on 2026-10-11 and found no issues; still to try: the camera over https, other browsers); reduced motion.
- **Cover size:** the pure-Go WebP encoder makes files about twice libwebp's at the same quality; most covers are ~100 KB, a few dense ones near 1 MB at 900px. Revisit (a lower quality for large covers, or another encoder) if the covers folder grows uncomfortable.
- **The two 1.x books without a valid ISBN** (On Palestine: 12 digits; Yılanı Öldürseler: wrong check digit) come in without one; the owner corrects them by hand after the move.
- **Test setups beside the real install:** document running a second Bookshelf with its own data folder and port, optionally started from a copy of the real data.
