# Bookshelf

Your books on a shelf in the browser. Every spine takes its cover's color and
stands as thick as the book is long; click one and it flies out of the shelf
and opens to its title page. Visitors browse your shelf; you sign in to add
books by ISBN or barcode, keep track of what you're reading, write notes, and
change how it all looks.

Bookshelf runs on a computer of your own: a home server or NAS (Unraid,
Synology), a Raspberry Pi, a rented server (a VPS), any machine with Docker, or
as a single program on your own computer. Nothing is stored anywhere else.

## Quick start

With Docker:

```bash
mkdir bookshelf && cd bookshelf
curl -fsSLO https://raw.githubusercontent.com/aloglu/bookshelf/main/docker-compose.example.yml
mv docker-compose.example.yml compose.yml
echo "BOOKSHELF_ADMIN_PASSWORD=choose-a-long-password" > .env
docker compose up -d
```

Open **http://localhost:8080**, click **Sign in** and sign in as `admin` with
your password. Then **Add book**.

Step-by-step guides for every kind of setup, including running it without
Docker, are in **[docs](docs/README.md)**.

## What it does

- **The shelf:** spines colored from their covers, as thick as their page
  count and as tall as their titles; hardcover, cloth and leather bindings that
  look like themselves; your choice of wall colors, wood and bookcase, by day and
  by evening.
- **The open book:** cover and title page, Previous and Next, a link to every
  book's own address, and searching it on Open Library, Google Books or
  Goodreads.
- **Adding books:** look an ISBN up on Open Library and Google Books, or scan
  its barcode with your phone's camera, with the cover fetched for you.
  Periodicals with an ISSN and an issue number, and books with neither, are
  entered by hand.
- **Reading:** several books at once, each with its own ribbon on the shelf;
  your page and pace; finishing, setting aside, re-reading, marking older
  reads; a note on each finished reading, private or public.
- **Statistics:** what you're reading, then the library in numbers.
- **Privacy:** hide any book from visitors, make the whole library private, or
  keep the statistics to yourself. The server enforces it; hidden books never
  leave it.
- **Your data:** automatic daily backups, restore in one step, export as CSV,
  JSON or a full archive with covers, and import from those or a Goodreads
  export.

## Documentation

- [Guides](docs/README.md): installing, using, putting your shelf online,
  backups, updating, troubleshooting.
- [Building from source](docs/build-from-source.md).

## License

Released under the [MIT License](https://github.com/aloglu/bookshelf/blob/4805ecdf62790aa468563182e9868b85852d807b/LICENSE).
