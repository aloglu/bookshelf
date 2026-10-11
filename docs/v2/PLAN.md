# Bookshelf 2.0 — plan and decisions

This folder holds the agreed direction for the 2.0 rework. Read this file first when picking the work back up.

- `prototype.html` — the clickable design study (desktop and mobile, day and evening). Open it in a browser. It is a design reference, not app code: its data is built in and every server action is acted out.
- Live copy: https://claude.ai/artifact/4oRGjkGhDJNBr5hSA8Qm9t

## Ground rules

- 2.0 is a clean break. Bookshelf 1.x was a terminal app that built a static website; 2.0 is a self-hosted web app (a server in Docker, or a plain program) that serves the shelf and its admin. Nothing of 1.x carries over except a one-time import of the owner's 1.x library.
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

- **One lock, taken once.** The database is opened at startup with `locking_mode=EXCLUSIVE` and that lock is held for the life of the program; no request ever waits on a file lock. A second Bookshelf (or the 1.x import) on the same folder is refused at once with "another Bookshelf is already using …".
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

**Stats page** (`#stats`, the Stats icon; off for visitors if the owner turns it off)
- Now reading first, under its own heading: cover, title, progress (share and page on the left, the owner's page field on the same line at the right, the bar beneath), and four facts (days since starting, pages to go, share of the library that is shorter, share that is older).
- The library in numbers: five uniform tiles (books, pages, authors, average length, years written), then books by decade (only decades that have books, labels fitted to the room), by page band, by binding, in translation, authors with two or more books, top publishers, and the oldest/newest/longest/shortest books.
- One accent hue, hairline grids, a Table view for every chart, hover readouts that only repeat what the table holds.

**Settings** (a drawer, tabs Site / Look / Library / Account). Every change saves at once and a quiet "Settings saved" pill says so.
- Site: name; private library (visitors see only a sign-in page); default sort; day/evening/system; stats page on/off; social links (Goodreads, The StoryGraph, Instagram; no others); each book's address (`/9780679723424`, `/978-0-679-72342-4` or `/pale-fire`); "Search on" sources (Open Library, Google Books, Goodreads).
- Look: wall colour for day and for evening, each its own colour (with preset pairs); wood for day and for evening (oak, walnut, maple, ash, pine, cherry, teak, mahogany, ebony), used by planks and bookcase alike; bookcase parts; reading ribbon colour (presets or any colour); bindings on/off; book size; tallest spine (scales every height by the same share, never caps).
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

At most one reading is in progress at a time (the shelf's single ribbon). Re-reads are separate readings. How long a book took is `ended_at − started_at`.

**Mark as read** (decided 2026-10-12): any book can be marked as read without having been tracked, since most books on a shelf were read before Bookshelf or without "Reading now". It creates a finished reading whose dates are as precise as the owner gives: a date, only a year, or none ("I don't remember"). So the dates are stored with a precision (`day`, `year`, `unknown`), and untracked readings have no page log. Such a reading counts as read (and can carry a note) but is left out of pace and time-to-finish figures; one with a year still counts toward that year. A book is "read" when it has at least one finished reading.

**Re-reading** (decided 2026-10-12): a book can be read any number of times. "Reading now" on a book already read starts a new reading; "Mark as read" on a book already marked adds another. Earlier readings are never changed by a new one. Each reading keeps its own dates, page log and note. Any past reading can be edited (dates, outcome) or deleted, to correct mistakes. Stats can count both distinct books read and readings (so re-reads are their own figure), and a book's page can show how many times it was read with each reading and its note.

`reading_progress` — the page log of a reading: reading_id, page, at. Written whenever the owner sets their page, so pace (pages per day), days actually read and progress over time can be worked out later. The current page is the latest entry.

`notes` — the owner's thoughts on a book, one per finished reading, tracked or marked as read (a re-read keeps its own): reading_id, text, visibility (private by default, or public), created_at, updated_at. Notes exist only for books that have been read. Public notes are the material for a simple blog section planned for later (see Follow-ups); until it exists, nothing is shown to visitors.

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
- `GET /api/lookup?isbn=` (Open Library, then Google Books, through the server).
- `POST /api/readings` (start reading a book, or mark it as read with an optional date or year), `PATCH /api/readings/<id>` (set the page, finish, set aside, correct dates), `DELETE /api/readings/<id>`, `GET /api/readings` (the history, optionally for one book).
- `PUT /api/readings/<id>/note`, `DELETE /api/readings/<id>/note` (text and visibility; finished readings only).
- `GET/PATCH /api/settings`.
- `POST /api/import`, `GET /api/export?format=csv|json|archive`.
- `GET /api/backups`, `POST /api/backups`, `POST /api/backups/<name>/restore`.

## Documentation (agreed 2026-10-12, after Inkubator's)

Guides for people running Bookshelf, step by step, saying what they should see, also for someone who has never used Docker:
- `README.md` — what it is, the quickest start, links.
- `docs/README.md` — choose where Bookshelf runs (a table by situation), then using it.
- One guide per home: Unraid, Synology, Raspberry Pi, Docker, without Docker.
- Using Bookshelf, remote access (HTTPS), backups, updating, moving from 1.x, troubleshooting, building from source.

## Phases

1. Data model and API: reviewed and settled in this file.
2. The server: storage, sign-in, the API, covers and spine colours, the Docker image.
3. Moving the owner's 1.x library over (one-time import, covers included), tested against a copy.
4. The frontend: the engine ported from the study, the Svelte surfaces around it, everything the study acted out made real.
5. Hardening: tests, a code-quality review, the guides.

## Progress

- 2026-10-12: design study finished (`prototype.html`). Architecture, storage (SQLite) and frontend split (plain-JS engine + Svelte) agreed. `v2` branch started.
- 2026-10-12, phase 2 begun. The 1.x terminal app (`cmd/bookshelf`, `internal/tui`, the `bookshelf` wrapper script) is removed; `internal/library` and `internal/siteassets` stay only until the 1.x import exists. New packages, each with tests:
  - `internal/store` — SQLite (modernc.org/sqlite), one connection, `locking_mode=EXCLUSIVE` + WAL taken at open with `busy_timeout(0)`; whole library held in memory as an immutable `Snapshot`; changes take turns on a mutex. Books (unique slug and ISBN), readings (one in progress, re-reads, mark as read with day/year/unknown precision, page log, corrections), notes (finished readings only), settings (validated), sessions (touched at most hourly; sliding week). Tests prove a second open is refused at once, no `-shm` file exists, and a single file lock is held while 200 readers run against 40 writes.
  - `internal/isbn` — normalize (labels, punctuation, ISBN-10 → 13), checksums, hyphenation from the embedded RangeMessage.xml (all groups, 978 and 979).
  - `internal/covers` — decode with size limits, WebP detail (900px) and thumbnail (360px) written atomically under a fresh name per cover, spine colour by the study's edge median, lettering colour.
  - `internal/auth` — env user/password (SHA-256 + constant-time compare), sessions in an HTTP-only SameSite=Lax cookie, lockout per client address (5 in 15 minutes) with the address taken from X-Forwarded-For only behind `BOOKSHELF_TRUSTED_PROXIES`, a global slow-down instead of a global lockout, pruned failure records.
  - `internal/lookup` — Open Library (edition + search index; edition title kept unless the index has the same title with better capitals or without a trailing series) then Google Books (optional `BOOKSHELF_GOOGLE_API_KEY`); covers fetched only over HTTPS from Open Library, archive.org and Google Books hosts, redirects checked too.
  - `internal/server` — the API from the plan, visitor and owner views built once per change, hidden books and their covers never served to visitors, a private library refusing visitors everything, same-origin check on every change, all-or-nothing settings patches.
  - `cmd/bookshelf` — the program: flags/env, default data folder `com.aloglu.bookshelf` in the system's data folder (never 1.x's), graceful shutdown, hourly session pruning, refuses to start without a password before touching anything, embeds the built frontend from `cmd/bookshelf/webdist` when present.
- Checked by hand against a scratch folder: sign-in, live lookup, adding a book with its Open Library cover, a second instance refused, and one database lock held before and after 300 parallel requests.

## Follow-ups

- **Phase 2, still to do:** backups (scheduled, before imports and restores, keep N, zip with real timestamps), restore, import (CSV, JSON, archive), export (CSV, JSON, archive with covers), the orphaned-cover cleanup, and the Docker image (non-root entrypoint with PUID/PGID, healthcheck).
- **Lookup data quality:** Open Library's "first published" year is sometimes wrong (Pale Fire shows 1945; it is 1962). The owner corrects it on the confirm card; consider preferring the edition's own year when the two disagree badly.
- **Reading in the interface:** the study has "Reading now" / "Finished reading" and the page field. 2.0 also needs "Set aside", "Mark as read" (with its optional date or year), a way to write the note after finishing (on the book's page, for books that have been read), and somewhere to see, correct and delete past readings of a book ("Read 3 times", each reading with its dates and note). Design these before the frontend phase.
- **Notes blog (later, decided 2026-10-12):** a very simple section where the owner publishes notes, chosen per book; notes can also stay private to the owner. Not part of 2.0's first release unless decided otherwise.
- **Read dates from imports (possibility, not committed):** a CSV import (for example a Goodreads export, which carries read dates) could mark books as read in one go.
- **Stats from reading history (later):** books finished per year/month, time to finish, pace, re-reads. The data is kept from day one so these can be added at any time.
- **Sign-in lockout behind a proxy** (from Inkubator's follow-ups): counting failures per connecting address means that behind Cloudflare Tunnel or any reverse proxy every visitor shares one address, so a bot's wrong passwords lock the owner out too. Use a trusted forwarded-address setting and a global limit, and prune the failure records.
- **Orphaned uploads:** a cover uploaded in a form that is then cancelled must not stay in the data folder; clean up unreferenced covers older than a day (on startup and daily).
- **Archive and backup timestamps:** zip entries carry real modification times.
- **Test setups beside the real install:** document running a second Bookshelf with its own data folder and port, optionally started from a copy of the real data.
- **Docker image:** never runs as root; the entrypoint gives `/data` to `PUID:PGID` and drops privileges; `PUID=0` refused; `--user` honoured.
