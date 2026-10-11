# Updating Bookshelf

Updating never touches your library: it replaces only the program. Still, it's
a good habit to **Back up now** first (Settings → Library → Backups).

New versions are listed on the
[releases page](https://github.com/aloglu/bookshelf/releases). Then follow the
steps for your setup below.

## Docker Compose (also Raspberry Pi)

In the folder with `compose.yml`:

```bash
docker compose pull
docker compose up -d
```

The first command downloads the new version, the second restarts Bookshelf
with it. Afterwards you can remove the old download with `docker image prune`.

If your `compose.yml` names a version (such as
`ghcr.io/aloglu/bookshelf:2.0.0`), change the number first. `:latest` always
follows the newest release; `:2` follows the newest 2.x.

## Docker without Compose

A container started with `docker run` keeps the version it started with.
Download the new version, remove the old container (your data folder stays),
and start it again with the same command you used the first time:

```bash
docker pull ghcr.io/aloglu/bookshelf:latest
docker stop bookshelf
docker rm bookshelf
docker run -d --name bookshelf ...   # the same command as before
```

## Unraid

On the Docker tab, click **update ready** next to Bookshelf, or **Check for
Updates** and then **Apply Update**. Your library in the data folder is kept.

## Synology

Over SSH, in the project folder:

```bash
cd /volume1/docker/bookshelf
sudo docker compose pull && sudo docker compose up -d
```

## Without Docker

1. Stop Bookshelf (**Ctrl+C** in its window, or
   `systemctl --user stop bookshelf` if it runs in the background).
2. Download the new version from the
   [releases page](https://github.com/aloglu/bookshelf/releases/latest) and
   replace the `bookshelf` program (`bookshelf.exe` on Windows) with the new
   one.
3. Start it again the same way as before.

Your library stays where it was.

## After updating

Reload Bookshelf in your browser. A new version may update the library's file
the first time it starts; that is why it can't then be opened by an older
version. To go back to an older version, restore a backup made with it.
