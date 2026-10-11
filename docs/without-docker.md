# Running Bookshelf without Docker

Bookshelf is also a single program you can download and run on Windows, macOS
or Linux. Nothing else needs to be installed. This is the quickest way to try
it on your own computer.

Bookshelf runs while its window (the terminal) is open, and you use it in your
browser. When you close the window, Bookshelf stops; your library stays safe in
its data folder.

## Step 1: Download

Go to the [latest release](https://github.com/aloglu/bookshelf/releases/latest)
and download the file for your computer:

| Computer | File |
| --- | --- |
| Windows | `bookshelf-…-windows-amd64.zip` |
| Windows on ARM | `bookshelf-…-windows-arm64.zip` |
| Mac with Apple Silicon (M1 and later) | `bookshelf-…-macos-arm64.tar.gz` |
| Mac with an Intel processor | `bookshelf-…-macos-amd64.tar.gz` |
| Linux on a regular PC | `bookshelf-…-linux-amd64.tar.gz` |
| Linux on ARM (Raspberry Pi and others, 64-bit) | `bookshelf-…-linux-arm64.tar.gz` |

Not sure which Mac you have? Apple menu → **About This Mac**: "Chip: Apple M…"
means Apple Silicon; "Processor: Intel" means Intel.

Unpack the file: on Windows, right-click → **Extract All**; on macOS and
Linux, double-click it or run `tar xzf` on it. You get a folder with the
program, `bookshelf` (`bookshelf.exe` on Windows).

## Step 2: Open a terminal in that folder

- **Windows:** open the folder in File Explorer, click the address bar, type
  `powershell` and press **Enter**.
- **macOS:** open the **Terminal** app, type `cd ` (with a space), drag the
  folder onto the Terminal window, and press **Enter**.
- **Linux:** right-click in the folder and choose **Open in Terminal**, or use
  `cd` to get there.

## Step 3: Set your password (once)

Run:

| System | Command |
| --- | --- |
| Windows | `.\bookshelf.exe set-password` |
| macOS, Linux | `./bookshelf set-password` |

Type a password (at least 8 characters) and press **Enter**, then type it
again. Nothing appears while you type; that's normal. Your user name is
`admin`. Bookshelf keeps the password in its data folder, as a one-way hash
that can't be turned back into the password.

**macOS says the program "cannot be opened" or "is damaged":** macOS is wary
of programs downloaded from the internet that aren't from the App Store. Run
this once in the same Terminal window, then try again:

```bash
xattr -d com.apple.quarantine bookshelf
```

**Windows shows "Windows protected your PC":** click **More info**, then **Run
anyway**.

## Step 4: Start Bookshelf

| System | Command |
| --- | --- |
| Windows | `.\bookshelf.exe` |
| macOS, Linux | `./bookshelf` |

You should see a line like this:

```text
level=INFO msg="Bookshelf is running" version=2.0.0 address=http://localhost:8080 data=/home/you/.local/share/com.aloglu.bookshelf user=admin
```

Open **http://localhost:8080**. You'll see an empty shelf. Click the **Sign
in** icon at the top right and sign in with the user name `admin` and your
password. Keep the terminal window open while you use Bookshelf; press
**Ctrl+C** in it to stop.

On Windows, the firewall may ask whether Bookshelf may use the network. Allow
it on **private networks** if you want to open Bookshelf from your phone at
home.

To change the password, run `set-password` again and restart Bookshelf.

## Where your library is kept

Unless you choose a folder, Bookshelf keeps everything in:

| System | Folder |
| --- | --- |
| Windows | `%APPDATA%\com.aloglu.bookshelf` (usually `C:\Users\you\AppData\Roaming\com.aloglu.bookshelf`) |
| macOS | `~/Library/Application Support/com.aloglu.bookshelf` |
| Linux | `~/.local/share/com.aloglu.bookshelf` |

To use another folder, add `--data-dir` to every command, for example
`./bookshelf --data-dir ~/Documents/Bookshelf` (and the same with
`set-password`).

## Options

| Option | Default | What it does |
| --- | --- | --- |
| `--data-dir <folder>` | the folder above | Where your library is kept |
| `--port <number>` | `8080` | The port in the address (`http://localhost:8080`) |
| `--host <address>` | every address | Who may connect: by default other devices at home can reach it; `127.0.0.1` allows only this computer |

The same can be set as `BOOKSHELF_DATA_DIR`, `PORT` and `BOOKSHELF_HOST`.
`./bookshelf --help` lists everything, including `BOOKSHELF_ADMIN_USER` for a
user name other than `admin`. A password in `BOOKSHELF_ADMIN_PASSWORD` is used
instead of the one set with `set-password`, if you prefer that.

## Using it from your phone

While Bookshelf runs, other devices on your home Wi-Fi can open it at your
computer's address, for example `http://192.168.1.20:8080`. To find the
address: on Windows run `ipconfig`; on macOS open System Settings → Wi-Fi →
Details; on Linux run `hostname -I`. Your computer has to be on and Bookshelf
running, which is why a server or NAS is a better long-term home (see the
[guides](README.md)).

## Starting automatically on Linux

On Linux, Bookshelf can run in the background and start when you log in. Save
this as `~/.config/systemd/user/bookshelf.service`, with the path to where you
put the program:

```ini
[Unit]
Description=Bookshelf

[Service]
ExecStart=/home/you/bookshelf/bookshelf
Restart=on-failure

[Install]
WantedBy=default.target
```

and run:

```bash
systemctl --user daemon-reload
systemctl --user enable --now bookshelf
```

To keep it running after you log out, also run `loginctl enable-linger`.

## Next

- [Using Bookshelf](using-bookshelf.md)
- [Backups](backups.md)
- [Updating](updating.md): download the new version and replace the program
  file; your data folder stays as it is.
