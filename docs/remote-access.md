# Putting your shelf online

At home, Bookshelf is reached by an address like `http://192.168.1.20:8080`.
That works only inside your home network, and it is plain HTTP: fine on your
own Wi-Fi, but not something to open to the internet.

**Never forward Bookshelf's port on your router.** Your password would travel
unencrypted, and anyone on the internet could try to guess it.

Below are safe ways to give your shelf an `https://` address. HTTPS also lets
you scan barcodes with your phone's camera, which browsers allow only on secure
addresses.

## Cloudflare Tunnel: a public address without opening your router

To let anyone see your shelf at an address such as `https://books.example.com`,
a Cloudflare Tunnel publishes Bookshelf without opening any port at home. You
need a domain name managed by Cloudflare (the free plan is enough).

1. In the Cloudflare dashboard, go to **Zero Trust → Networks → Tunnels** and
   create a tunnel. Follow its instructions to install `cloudflared` on the
   machine running Bookshelf (Unraid and Synology have apps or containers for
   it).
2. Add a **public hostname** (for example `books.example.com`) pointing to
   `http://localhost:8080` (or `http://bookshelf:8080` if `cloudflared` runs in
   a container beside Bookshelf).
3. Tell Bookshelf to trust the tunnel (see
   [Trusting your proxy](#trusting-your-proxy) below), and restart it.
4. Open `https://books.example.com`.

## Your own reverse proxy

If you already run a web server such as Caddy, nginx or Nginx Proxy Manager
with a domain name, put Bookshelf behind it. With
[Caddy](https://caddyserver.com), which gets HTTPS certificates by itself, this
is the whole configuration:

```caddyfile
books.example.com {
  reverse_proxy localhost:8080
}
```

Use the address of the machine running Bookshelf instead of `localhost` if
Caddy runs elsewhere. Then [trust the proxy](#trusting-your-proxy).

## Tailscale: just for you

If the shelf is only for you, [Tailscale](https://tailscale.com) connects your
own devices in a private, encrypted network, and Bookshelf stays invisible to
everyone else. It is free for personal use.

1. Install Tailscale on the computer or server running Bookshelf and on your
   phone, and sign in to the same account on both. (Unraid and Synology have
   Tailscale apps; on a Raspberry Pi follow
   [Tailscale's Linux instructions](https://tailscale.com/download/linux).)
2. On the computer running Bookshelf, give it an HTTPS address:

   ```bash
   tailscale serve --bg 8080
   ```

   (If you changed Bookshelf's port, use that number.) Tailscale prints an
   address like `https://your-server.your-tailnet.ts.net`.
3. [Trust the proxy](#trusting-your-proxy): Tailscale's comes from the same
   computer.
4. Open that address on your phone. It works at home and away, as long as
   Tailscale is on.

## Trusting your proxy

Behind a tunnel or proxy, every visitor seems to come from the proxy's own
address. Tell Bookshelf which addresses are your proxy with
`BOOKSHELF_TRUSTED_PROXIES`, and it will:

- limit wrong passwords per visitor, so a stranger guessing passwords can't
  lock you out;
- believe the proxy when it says the page was reached over HTTPS (proxies say
  so with an `X-Forwarded-Proto` header), and then keep your sign-in cookie to
  HTTPS only.

What to put in it:

| Your setup | `BOOKSHELF_TRUSTED_PROXIES` |
| --- | --- |
| Bookshelf without Docker, proxy on the same computer | `127.0.0.1, ::1` |
| Bookshelf in Docker, proxy on the same computer or in another container | `172.16.0.0/12` (Docker's own networks) |
| Proxy on another machine | that machine's address, for example `192.168.1.5` |

In Docker, add it as a variable (see
[Docker → Other settings](docker.md#other-settings)); without Docker, set it
before starting Bookshelf, like the password. Only list addresses that are
really your proxy: Bookshelf believes what they say about visitors.

## Whichever way you choose

- Use a long password of your own.
- Decide what visitors may see in **Settings → Visitors**: a
  private library shows them only a sign-in page.
- Keep backups (see [Backups](backups.md)).
