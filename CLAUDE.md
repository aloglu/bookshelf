# Bookshelf

Bookshelf is becoming a self-hosted web app (a Go server in Docker, or a plain program) that shows the owner's books on a shelf and manages them in the browser. 2.0 is being built on the `v2` branch; `main` still holds 1.x, a terminal app that builds a static site.

**Before doing anything, read `docs/v2/PLAN.md`:** it holds every settled design decision, the architecture, the data model, progress and follow-ups. `docs/v2/prototype.html` is the design study the app follows.

Working rules:
- 2.0 is a clean break: no backward compatibility with 1.x and no code for it. The owner's 1.x library is moved over once by a script outside the repository. No dead code.
- One Go server; SQLite through a pure-Go driver; the shelf and the open book as a plain-JavaScript engine, everything around them in Svelte + Vite.
- Never write to the owner's real 1.x library (`~/.local/share/bookshelf`); test against copies in a scratch folder.
- Update the Progress and Follow-ups sections of `docs/v2/PLAN.md` as work lands.
- Keep the guides in `docs/` true to the app: when behaviour or setup changes, change the guide in the same piece of work.

Building: `cd web && npm ci && npm run build` puts the frontend in `cmd/bookshelf/webdist`, then `go build ./cmd/bookshelf` embeds it. For frontend work, run Bookshelf on port 8080 and `npm run dev` in `web/`. `go test ./...` runs the server's tests.
