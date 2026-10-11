# Troubleshooting

First, look at what Bookshelf says. It explains most problems in plain words.

- **Docker Compose:** `docker compose logs bookshelf` in the folder with
  `compose.yml`.
- **Unraid / Synology:** the container's **Logs**.
- **Without Docker:** the terminal window Bookshelf runs in.

## Bookshelf doesn't start

**"set BOOKSHELF_ADMIN_PASSWORD to the password you want to sign in with, or run bookshelf set-password once"**
Bookshelf won't start without a password. Docker: add
`BOOKSHELF_ADMIN_PASSWORD` (in `.env`, or as a variable on Unraid/Synology).
Without Docker: run `bookshelf set-password` once (see
[Without Docker, step 3](without-docker.md#step-3-set-your-password-once)); if
you start Bookshelf with `--data-dir`, give `set-password` the same folder.

**"…is another program using port 8080?" / "port is already allocated"**
Another program uses port 8080. Docker: set another host port
(`BOOKSHELF_HOST_PORT=8090` in `.env`). Without Docker: start with
`--port 8090`. Then use that port in the address.

**"PUID and PGID must not be 0" / "PUID must be a number"**
Set `PUID` and `PGID` to a regular user (run `id` to see yours; on Unraid `99`
and `100`). `0` is root and is refused on purpose.

**"give the data folder to …" / "open the library in …: permission denied"**
Bookshelf can't use the data folder. With Docker, check `PUID` and `PGID`:
Bookshelf gives the folder to that user when it starts, so the folder must be
on a disk where that's possible (not a read-only or network share that refuses
ownership changes). Without Docker, make sure your user may write to the folder,
or choose another with `--data-dir`.

**"another Bookshelf is already using this data folder"**
Only one Bookshelf can use a data folder at a time. Stop the other one (an
older container, a second copy of the program, or a test pointed at the same
folder) and start again. To try something out beside your real library, give
the test its own data folder.

**"this library was written by a newer Bookshelf"**
The data folder was used by a newer version. Update Bookshelf (see
[Updating](updating.md)), or restore a backup made with this version.

**"BOOKSHELF_TRUSTED_PROXIES: … is not an address or network"**
List addresses (`192.168.1.5`) or networks (`172.16.0.0/12`), separated by
commas. See [Trusting your proxy](remote-access.md#trusting-your-proxy).

**"the password file in the data folder is damaged"**
Run `bookshelf set-password` again.

**macOS: "cannot be opened" / "is damaged"**
See [Without Docker, step 3](without-docker.md#step-3-set-your-password-once).

## It starts, but…

**The page doesn't open from my phone.**
Check that the phone is on the same Wi-Fi, and use the computer's address, not
`localhost`. With Docker, `BOOKSHELF_BIND_ADDRESS=0.0.0.0` must be set (see
[Opening it from other devices](docker.md#opening-it-from-other-devices)). A
firewall on the computer may block the port, and a VPN on the computer may
block devices at home unless it allows "local network" access.

**"That user name and password don't match."**
Use `BOOKSHELF_ADMIN_USER` (`admin` unless you changed it) and your password:
`BOOKSHELF_ADMIN_PASSWORD`, or the one set with `set-password`. To change it,
change the setting (or run `set-password` again) and restart Bookshelf.

**"Too many wrong passwords. Try again in … minutes."**
After five wrong passwords from one place, signing in from there waits 15
minutes. If this happens to you behind a tunnel or proxy, where every visitor
seems to come from the proxy, set
[`BOOKSHELF_TRUSTED_PROXIES`](remote-access.md#trusting-your-proxy).

**"This change didn't come from Bookshelf's own page."**
Bookshelf refuses changes that come from other sites. If you see this while
using Bookshelf itself, your browser is very old, or a proxy changes the
address in a way Bookshelf can't follow: update the browser, and see
[Trusting your proxy](remote-access.md#trusting-your-proxy).

**"Scan barcode" says the camera only works over https.**
Browsers let a page use the camera only on a secure (`https://`) address or on
the computer Bookshelf runs on. Type the ISBN instead, or give Bookshelf an
https address: see [Putting your shelf online](remote-access.md).

**Looking up an ISBN finds nothing, or "The book sources couldn't be reached".**
Bookshelf asks Open Library first, then Google Books. Some books (many
non-English editions among them) are in neither; enter those by hand. Google
also allows only a few lookups a day without a key: if lookups often fail, set
an optional `BOOKSHELF_GOOGLE_API_KEY` (free from the
[Google Cloud console](https://console.cloud.google.com/apis/library/books.googleapis.com)).
The computer running Bookshelf needs internet access for lookups.

**The shelf looks empty to visitors, but I see my books.**
Your books are hidden from the website, or the library is private. Check
**Settings → Site → What visitors see**, and **Show on website** on each book's
page (or **Select** several, then **Show**).

## Still stuck?

Open an [issue](https://github.com/aloglu/bookshelf/issues) with what you did,
what you expected, and what Bookshelf's log says. Leave out your password.
