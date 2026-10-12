# Backups

Your whole library lives in Bookshelf's **data folder**: the books, their
covers, your readings and notes, the settings, and the backups themselves.
Bookshelf makes backups by itself, and you can make one whenever you like.

A backup is a single `.bookshelf` file (a zip) with the library and every
cover. It works on any Bookshelf 2 setup: Docker, Unraid, Synology or the plain
program.

## Automatic backups

Sign in and open **Settings → Your library → Backups**. You choose:

- **Back up:** every day (the default), every week, or only before imports and
  restores.
- **Keep the last:** how many backups to keep (14 by default). Older ones are
  deleted.

Bookshelf also makes a backup just before every import and every restore,
whatever you choose, so either can be undone.

Backups are saved in the data folder, in `backups`, with names like
`bookshelf-2026-10-11-030000-automatic.bookshelf`. They protect you from
mistakes inside Bookshelf (such as removing the wrong books), but not from
losing the disk they are on. For that, also keep a copy elsewhere: **Download**
one now and then, or let your NAS backup tool or cloud sync include the data
folder.

## Making a backup yourself

**Back up now** makes one at once. Each backup in the list has **Download**, to
keep it somewhere safe such as another disk or cloud storage.

## Restoring

**Restore** beside a backup in the list, or **Restore from a file** for a
`.bookshelf` file you kept elsewhere.

Restoring **replaces your whole library** with the one in the backup. Bookshelf
asks first, checks the file before using it, and backs up your current library
just before replacing it, so a restore can be undone by restoring that one.

## Taking your books out

**Settings → Your library → Take them out** downloads:

- **CSV**, for a spreadsheet: one row per book, with its ISBN hyphenated.
- **JSON**, for other programs.
- **Full archive with covers**: a `.bookshelf` file, the same as a backup.

CSV and JSON hold the books only; readings, notes and covers travel in the full
archive.

## Bringing books in

**Settings → Your library → Bring books in** takes a file (choose it, or drop it on
the box):

- a **`.bookshelf` archive**, from a backup or another Bookshelf: the books with
  their covers, readings and notes;
- a **CSV** file, from Bookshelf or another program: columns are found by their
  header, in any order. Only a title column is needed; author, translator,
  publisher, published (or year), pages (or number of pages), binding, ISBN (or
  ISBN13), ISSN, issue and hidden are used if present. A
  [Goodreads export](https://www.goodreads.com/review/import) works as it is;
- a **JSON** list of books, as Bookshelf exports it.

Importing **adds** books and never replaces any: a book already on your shelf
(the same ISBN, the same issue of a periodical, or without either the same
title and author) is skipped. Bookshelf backs up first, then says how many
books were added and how many were already there. Files can be up to 2 GB.

## Moving to another computer or server

1. On the old setup: **Back up now**, then **Download** that backup.
2. Set Bookshelf up on the new one (see the [guides](README.md)).
3. On the new one: **Restore from a file** with that backup.

## What is in the data folder

| Item | What it is |
| --- | --- |
| `bookshelf.db` (and `bookshelf.db-wal`) | Your library and settings |
| `covers/` | Every cover, and a smaller copy of each for the shelf |
| `backups/` | The backups |
| `password` | Only if you used `bookshelf set-password`: your password, as a one-way hash |

Where the data folder is:

| Setup | Data folder |
| --- | --- |
| Docker | the folder you gave as `/data` (`bookshelf-data` in the [Docker guide](docker.md)) |
| Unraid | `/mnt/cache/appdata/bookshelf` |
| Synology | `docker/bookshelf/data` |
| Without Docker | see [Where your library is kept](without-docker.md#where-your-library-is-kept) |

Don't edit or copy `bookshelf.db` while Bookshelf runs; to go back to an earlier
state, restore a backup instead. Only one Bookshelf can use a data folder at a
time.
