# Building from source

For people who want to change Bookshelf or build it themselves.

## What it is made of

| Part | Folder | Language |
| --- | --- | --- |
| The program: storage, sign-in, the API, covers, lookups, backups | `cmd/bookshelf`, `internal/` | Go |
| The shelf and the open book (spines, flights, the title page) | `web/src/engine` | JavaScript |
| Everything around them (header, stats, settings, cards) | `web/src/ui`, `web/src/lib` | Svelte 5, Vite |

The library is kept in SQLite through a pure-Go driver, so no C compiler is
needed and every platform's program cross-builds from any machine. The program
has the web interface built into it: it embeds `cmd/bookshelf/webdist`, which
the web build fills. `docs/v2/PLAN.md` holds the design decisions and the
architecture.

## Requirements

- Go (the version in `go.mod`).
- Node.js 24 or later.

## Build the program

```bash
cd web
npm ci
npm run build
cd ..
go build ./cmd/bookshelf
```

The program is `./bookshelf`. Build the web interface first; otherwise the
program starts but says its web interface isn't built in.

For another system, set `GOOS` and `GOARCH`, for example
`GOOS=windows GOARCH=amd64 go build ./cmd/bookshelf`. The release builds Linux,
macOS and Windows, each for amd64 and arm64.

## Run it while working on it

```bash
BOOKSHELF_ADMIN_PASSWORD=dev go run ./cmd/bookshelf --data-dir /tmp/bookshelf-dev
```

For work on the web interface, leave that running and, in another terminal:

```bash
cd web
npm run dev
```

Open the address Vite prints. Changes to the web interface show at once; the
API and covers are passed through to the program on port 8080
(`BOOKSHELF_DEV_SERVER` points elsewhere).

Use a data folder of its own for development, never your real library.

## Tests

```bash
go vet ./...
go test ./...
```

The scanner and browser regressions run from `web/`:

```bash
npm ci
npm test
npm run build
npx playwright install chromium firefox
npm run test:browser
```

The browser suite builds a server and uses a new temporary library on port
18323. It stops the server and removes that test library afterwards. The
[review checklist](REVIEW-TESTING.md) covers separate Docker deployment and
the real-device checks that these automated tests cannot establish.

## Build the Docker image

```bash
docker build -t bookshelf:local .
docker/smoke-test.sh bookshelf:local
```

The smoke test starts the image, signs in, adds a book with a cover, makes a
backup, restarts it and checks that everything is kept and that the data
belongs to `PUID:PGID`. It leaves nothing behind.

## Releases

Pushing a tag such as `v2.0.0` runs `.github/workflows/release.yml`: it checks
that the tag matches `web/package.json`, runs the tests, builds and smoke-tests
the Docker images for amd64 and arm64, publishes them to
`ghcr.io/aloglu/bookshelf`, builds the programs for every system, and creates a
draft release to review before publishing.
