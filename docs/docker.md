# Running Bookshelf with Docker

This guide sets Bookshelf up with Docker on any computer or server: Windows,
macOS or Linux, on a regular PC (amd64) or an ARM machine such as a Raspberry
Pi or an Apple Silicon Mac. If you use Unraid or Synology, their own guides
([Unraid](unraid.md), [Synology](synology.md)) are easier.

## What you will end up with

- Bookshelf running in the background, starting again by itself after a
  restart.
- A folder called `bookshelf-data` that holds your whole library: the books,
  their covers, what you've read and your notes, the settings, and the
  automatic backups. As long as you keep this folder, you keep your library.
- Bookshelf open in your browser at `http://localhost:8080`.

## Step 1: Install Docker

- **Windows or macOS:** install [Docker Desktop](https://www.docker.com/products/docker-desktop/)
  and start it. Wait until it says Docker is running.
- **Linux:** follow Docker's guide for your distribution
  ([docs.docker.com/engine/install](https://docs.docker.com/engine/install/)).
  On Debian, Ubuntu and Raspberry Pi OS, the short way is:

  ```bash
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker "$USER"
  ```

  Then log out and back in, so your account may use Docker without `sudo`.

Check that it works. Open a terminal (on Windows: PowerShell; on macOS: the
Terminal app) and run:

```bash
docker compose version
```

You should see a version number. If you see "command not found", Docker is
not installed or not running yet.

## Step 2: Make a folder for Bookshelf

Make a folder that will hold Bookshelf's setup and data, for example
`bookshelf` in your home folder, and open a terminal in it:

```bash
mkdir bookshelf
cd bookshelf
```

## Step 3: Describe the setup

Create a file called `compose.yml` in that folder with this content (or copy
[`docker-compose.example.yml`](../docker-compose.example.yml) from this
repository):

```yaml
services:
  bookshelf:
    image: ghcr.io/aloglu/bookshelf:latest
    container_name: bookshelf
    restart: unless-stopped
    ports:
      - "${BOOKSHELF_BIND_ADDRESS:-127.0.0.1}:${BOOKSHELF_HOST_PORT:-8080}:8080"
    environment:
      BOOKSHELF_ADMIN_USER: ${BOOKSHELF_ADMIN_USER:-admin}
      BOOKSHELF_ADMIN_PASSWORD: "${BOOKSHELF_ADMIN_PASSWORD:?Set BOOKSHELF_ADMIN_PASSWORD before starting Bookshelf}"
      PUID: ${PUID:-1000}
      PGID: ${PGID:-1000}
    volumes:
      - ./bookshelf-data:/data
```

Then create a second file called `.env` (the name starts with a dot) next to
it. This is where your password lives:

```dotenv
BOOKSHELF_ADMIN_USER=admin
BOOKSHELF_ADMIN_PASSWORD=choose-a-long-password-of-your-own
```

Use a password you don't use anywhere else. On Linux and macOS, make the file
readable only by you:

```bash
chmod 600 .env
```

**What these lines mean**

- `image` is the Bookshelf program Docker downloads.
- `ports` decides who can open Bookshelf. `127.0.0.1` means only this
  computer. See [Opening it from other devices](#opening-it-from-other-devices)
  to change it.
- `PUID` and `PGID` are the user that owns your data folder. On most Linux
  systems your own user is `1000`; run `id` to check. Bookshelf never runs as
  the administrator (root).
- `./bookshelf-data:/data` is your library. Docker creates the folder the
  first time.

## Step 4: Start Bookshelf

In the same folder, run:

```bash
docker compose up -d
```

The first time, Docker downloads Bookshelf, which takes a moment. When it
finishes, open **http://localhost:8080** in your browser. You'll see an empty
shelf. Click the **Sign in** icon at the top right and sign in with the user
name and password from your `.env` file; then **Add book** puts the first one
on the shelf. [Using Bookshelf](using-bookshelf.md) shows the rest.

To see what Bookshelf is doing, run `docker compose logs bookshelf`. Docker
also checks on it every 30 seconds: `docker compose ps` shows it as
`(healthy)` when all is well.

## Everyday commands

Run these in the folder with `compose.yml`:

| To | Run |
| --- | --- |
| Stop Bookshelf | `docker compose stop` |
| Start it again | `docker compose start` |
| See what it's doing | `docker compose logs -f bookshelf` (Ctrl+C to stop watching) |
| Update to a new version | see [Updating](updating.md) |

Your library stays in `bookshelf-data` whatever you do with the container.

## Opening it from other devices

As set up above, only this computer can open Bookshelf. To open it from your
phone or other computers on your home network, add this line to `.env`:

```dotenv
BOOKSHELF_BIND_ADDRESS=0.0.0.0
```

Run `docker compose up -d` again, then open `http://THIS-COMPUTER:8080` on the
other device, with this computer's address or name (for example
`http://192.168.1.20:8080`). To find the address: on Windows run `ipconfig`,
on macOS look in **System Settings → Network**, on Linux run `hostname -I`.

To give your shelf a web address that visitors can open from anywhere, see
[Putting your shelf online](remote-access.md).

If port 8080 is already used by something else, set `BOOKSHELF_HOST_PORT=8090`
(or any free port) in `.env` and open that port instead.

## Other settings

Add any of these to `.env`, and to the `environment` list in `compose.yml`, as
`NAME: ${NAME}`:

| Setting | What it does |
| --- | --- |
| `BOOKSHELF_TRUSTED_PROXIES` | When Bookshelf sits behind a reverse proxy, the proxy's address or network (for example `172.18.0.0/16`), so sign-in limits apply to each visitor and not to the proxy. See [Putting your shelf online](remote-access.md). |
| `BOOKSHELF_GOOGLE_API_KEY` | An optional Google Books key. Without one, Google allows only a few lookups a day; Open Library is always asked first. |
| `TZ` | Your time zone (for example `Europe/Istanbul`), for the times in the logs. |
