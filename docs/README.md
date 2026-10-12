# Bookshelf guides

Bookshelf puts your books on a shelf in your web browser: every spine in its
cover's color, as thick as the book. Visitors can browse it; you sign in to add
books, keep track of what you're reading and change how it looks. It runs on a
computer you own, and nothing is stored anywhere else.

If you have never set up anything like this before, that's fine: every guide
here goes step by step and says what you should see.

## 1. Choose where Bookshelf runs

Bookshelf is a small program that keeps running while people use it. Pick the
line that sounds most like you:

| Your situation | Follow |
| --- | --- |
| I have a home server or NAS running **Unraid** | [Unraid](unraid.md) |
| I have a **Synology** NAS | [Synology](synology.md) |
| I have a **Raspberry Pi** (or want to use one) | [Raspberry Pi](raspberry-pi.md) |
| I already use **Docker**, or want to on my computer or server | [Docker](docker.md) |
| I rent a server (a **VPS** from Hetzner, DigitalOcean or the like) | [Docker](docker.md), then [Putting your shelf online](remote-access.md#your-own-reverse-proxy) |
| I just want to try it on **my own computer**, without installing anything else | [Without Docker](without-docker.md) |

Not sure? A computer or NAS that stays on is the best home for Bookshelf: your
shelf is then always there for visitors, and for you from your phone. If you
only want to try it, start with [Without Docker](without-docker.md) on your own
computer; you can move your library to a server later with a backup.

**What is Docker?** Docker runs programs in sealed boxes called *containers*.
You tell Docker which program you want ("the Bookshelf image") and which folder
it may keep its files in, and Docker takes care of the rest. Many home servers
and NAS systems have it built in. The [Docker guide](docker.md) explains it
from the start.

## 2. Use it

- [Using Bookshelf](using-bookshelf.md): signing in, adding books, covers,
  what you're reading, notes, and what visitors see.
- [Putting your shelf online](remote-access.md): a web address for visitors,
  and for you away from home, safely with HTTPS.
- [Backups](backups.md): automatic backups, exporting, importing and restoring.
- [Updating](updating.md): getting a new version.
- [Troubleshooting](troubleshooting.md): when something doesn't work.

For developers: [Building from source](build-from-source.md).
