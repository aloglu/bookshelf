# Running Bookshelf on a Synology NAS

On a Synology NAS, Bookshelf runs in **Container Manager**, Synology's Docker
app. These steps are for DSM 7.2 or later. Bookshelf runs on both Intel/AMD and
ARM (arm64) Synology models; older 32-bit ARM models aren't supported.

## Step 1: Install Container Manager

Open **Package Center**, search for **Container Manager**, and install it.

## Step 2: Make a folder

Open **File Station**. In the `docker` shared folder (Container Manager creates
it), make a folder called `bookshelf`, and inside it another folder called
`data`.

## Step 3: Find your user and group numbers

Bookshelf runs as a regular user who owns the `data` folder. The simplest way
to find your user's numbers:

1. Open **Control Panel → Terminal & SNMP** and turn on **SSH** for a moment.
2. Connect with an SSH program (on Windows: PowerShell; on macOS: Terminal):
   `ssh your-user@your-nas`, and run `id`.
3. Note the numbers after `uid=` and `gid=` (often `1026` and `100`), then
   turn SSH off again if you don't use it.

## Step 4: Create the project

1. Open **Container Manager → Project → Create**.
2. **Project name:** `bookshelf`. **Path:** the `docker/bookshelf` folder.
3. **Source:** *Create docker-compose.yml*, and paste this, with your own
   password and the numbers from step 3:

   ```yaml
   services:
     bookshelf:
       image: ghcr.io/aloglu/bookshelf:latest
       container_name: bookshelf
       restart: unless-stopped
       ports:
         - "8080:8080"
       environment:
         BOOKSHELF_ADMIN_USER: admin
         BOOKSHELF_ADMIN_PASSWORD: "choose-a-long-password-of-your-own"
         PUID: 1026
         PGID: 100
       volumes:
         - ./data:/data
   ```

4. Click **Next** until **Done**. Container Manager downloads Bookshelf and
   starts it.

If port 8080 is already in use on the NAS, change the first number, for
example `"8090:8080"`.

## Step 5: Open Bookshelf

Open `http://YOUR-NAS:8080` in your browser (your NAS's address, for example
`http://192.168.1.10:8080`, or the name you use for it). You'll see an empty
shelf. Click the **Sign in** icon at the top right and sign in with `admin`
and the password from the project.

Synology's firewall, if you turned it on, must allow port 8080 from your home
network (**Control Panel → Security → Firewall**).

## Updating

The most reliable way is two commands over SSH (see step 3 for turning SSH on):

```bash
cd /volume1/docker/bookshelf
sudo docker compose pull && sudo docker compose up -d
```

The first downloads the new version, the second restarts Bookshelf with it.
Your library in `docker/bookshelf/data` is kept. (Container Manager can also do
this from its **Project** and **Image** screens, but the menus differ between
DSM versions.) See [Updating](updating.md).

## Next

- [Using Bookshelf](using-bookshelf.md): adding books, from your phone too.
- [Backups](backups.md): Hyper Backup can include the `docker/bookshelf`
  folder as well.
- [Putting your shelf online](remote-access.md): a web address for visitors.
