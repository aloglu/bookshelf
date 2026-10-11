# Running Bookshelf on Unraid

Unraid runs Bookshelf as a Docker container that you add through its web
interface. No terminal is needed.

## Add the container

1. Make sure the array is started and Docker is enabled (**Settings → Docker**).
2. Open the **Docker** tab and click **Add Container**.
3. Fill in the top of the form:

   | Field | Value |
   | --- | --- |
   | Name | `Bookshelf` |
   | Repository | `ghcr.io/aloglu/bookshelf:latest` |
   | Network Type | `Bridge` |
   | Privileged | Off |
   | Icon URL | `https://raw.githubusercontent.com/aloglu/bookshelf/main/web/public/icon-512.png` |
   | WebUI | `http://[IP]:[PORT:8080]/` |

   **Icon URL** and **WebUI** appear after switching to **Advanced View** (top
   right). The icon is what the Docker tab and the dashboard show for
   Bookshelf; WebUI adds an "Open WebUI" entry to the container's menu.

4. Click **Add another Path, Port, Variable, Label or Device** and add a
   **Port**:

   | Field | Value |
   | --- | --- |
   | Name | `Web` |
   | Container Port | `8080` |
   | Host Port | `8080` (or any free port) |
   | Connection Type | `TCP` |

   If 8080 is already used by another container, change only the **Host
   Port**, for example to `8090`.

5. Add a **Path** for your library:

   | Field | Value |
   | --- | --- |
   | Name | `Data` |
   | Container Path | `/data` |
   | Host Path | `/mnt/cache/appdata/bookshelf` |
   | Access Mode | `Read/Write` |

   Everything Bookshelf keeps (your books, covers, readings, notes, settings
   and automatic backups) lives in this folder, so it survives updates.

   `/mnt/cache/appdata/...` puts the folder straight on your cache drive.
   Bookshelf also works under `/mnt/user/appdata/bookshelf`, but the direct
   path is faster and skips Unraid's share layer. If your appdata share isn't
   on a cache pool, use `/mnt/user/appdata/bookshelf`.

6. Add four **Variables**:

   | Name | Key | Value |
   | --- | --- | --- |
   | Password | `BOOKSHELF_ADMIN_PASSWORD` | A long password of your own |
   | User name | `BOOKSHELF_ADMIN_USER` | `admin`, or a name you like |
   | User ID | `PUID` | `99` |
   | Group ID | `PGID` | `100` |

   `99` and `100` are Unraid's usual `nobody` user and `users` group, so the
   files stay accessible from your shares. Bookshelf never runs as root.

7. Click **Apply**. Unraid downloads Bookshelf and starts it. Turn on
   **Autostart** for the container so it starts with the array.

## Open Bookshelf

Click the Bookshelf icon on the Docker tab and choose **WebUI**, or open
`http://YOUR-SERVER:8080` in your browser (with your server's address or name,
for example `http://tower.local:8080`). You'll see an empty shelf. Click the
**Sign in** icon at the top right and sign in with the user name and password
you set.

The Docker tab shows the container as **healthy** once it is ready; Bookshelf
checks itself every 30 seconds.

From your phone, open the same address while on your home Wi-Fi.
[Using Bookshelf](using-bookshelf.md) shows the rest.

## Updating

When a new version is out, the Docker tab shows **update ready** next to
Bookshelf. Click it, then **Apply update**. Your library stays in the data
folder. See [Updating](updating.md) for more.

## Good to know

- **Only one Bookshelf per data folder.** If you add a second container (or a
  test copy) pointing at the same folder, it stops at once with "another
  Bookshelf is already using this data folder". Give each copy its own folder.
- **Backups** are made every day in `/mnt/cache/appdata/bookshelf/backups`.
  Unraid's own appdata backup plugin can copy that folder elsewhere too. See
  [Backups](backups.md).
- **Other settings** (a reverse proxy's address, a Google Books key) are more
  **Variables**; see [Docker → Other settings](docker.md#other-settings).
