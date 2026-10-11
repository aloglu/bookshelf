# Running Bookshelf on a Raspberry Pi

A Raspberry Pi makes a good always-on home for Bookshelf: it uses little power,
your shelf is there whenever visitors come by, and you can reach it from your
phone at any time.

## What you need

- A Raspberry Pi 3, 4 or 5 (a Pi 4 or 5 with 2 GB of memory or more is
  comfortable).
- **Raspberry Pi OS (64-bit)**. Bookshelf runs on 64-bit ARM (`arm64`); the
  older 32-bit system won't run it. If you are setting up a new card, choose
  "Raspberry Pi OS (64-bit)" in Raspberry Pi Imager. To check an existing
  system, run `uname -m`: it must say `aarch64`.
- The Pi connected to your home network, and a way to type commands on it:
  directly with a keyboard, or over SSH from another computer
  (`ssh your-user@raspberrypi.local`).

## Step 1: Install Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker "$USER"
```

Log out and back in (or restart the Pi), then check with `docker compose version`.

## Step 2: Set up Bookshelf

```bash
mkdir ~/bookshelf
cd ~/bookshelf
```

Create `compose.yml`:

```bash
nano compose.yml
```

Paste this, save with **Ctrl+O**, **Enter**, then leave with **Ctrl+X**:

```yaml
services:
  bookshelf:
    image: ghcr.io/aloglu/bookshelf:latest
    container_name: bookshelf
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      BOOKSHELF_ADMIN_USER: ${BOOKSHELF_ADMIN_USER:-admin}
      BOOKSHELF_ADMIN_PASSWORD: "${BOOKSHELF_ADMIN_PASSWORD:?Set BOOKSHELF_ADMIN_PASSWORD}"
      PUID: ${PUID:-1000}
      PGID: ${PGID:-1000}
    volumes:
      - ./bookshelf-data:/data
```

This opens Bookshelf to your whole home network (the `"8080:8080"` line), which
is what you want on a Pi: you'll use it from other devices.

Now the password. Create `.env` the same way (`nano .env`):

```dotenv
BOOKSHELF_ADMIN_PASSWORD=choose-a-long-password-of-your-own
```

and make it readable only by you:

```bash
chmod 600 .env
```

`PUID` and `PGID` default to `1000`, the first user on Raspberry Pi OS. If
`id` shows other numbers for your user, add `PUID=...` and `PGID=...` lines to
`.env`.

## Step 3: Start it

```bash
docker compose up -d
```

The first start downloads Bookshelf. Then, on any device on your home network,
open `http://raspberrypi.local:8080` (or the Pi's address, which
`hostname -I` shows on the Pi). You'll see an empty shelf; click the **Sign
in** icon at the top right and sign in as `admin` with your password.

## Everyday use

Bookshelf starts by itself whenever the Pi starts. The commands in
[Docker → Everyday commands](docker.md#everyday-commands) work here too.

A Pi's memory card is a fragile place for anything precious. Bookshelf backs up
your library every day inside `bookshelf-data/backups`; copy one of those, or a
full export, to another computer now and then. See [Backups](backups.md).

## Next

- [Using Bookshelf](using-bookshelf.md)
- [Putting your shelf online](remote-access.md): a web address for visitors.
- [Updating](updating.md)
