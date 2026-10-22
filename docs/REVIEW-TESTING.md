# Testing the review build

This candidate is on [`v2.1.1-review`](https://github.com/aloglu/bookshelf/tree/v2.1.1-review).
It is for checking the reported problems before a release. It does not move
`main`, `latest`, `2`, or any released image tag.

## What this candidate changes

- The page keeps an explicit vertical overflow setting when the book or an
  Add-book card releases its scroll lock. This addresses the reproduced
  Firefox jump on closing.
- Put back retains the row where its closing flight landed after stepping
  through books. A focused test reproduced a second jump when history restored
  the original opening row; this is an additional finding, not a reproduction
  of the owner's Chrome closing effect. Browser Back/Forward retain their own
  saved-position behavior.
- Adding returns to the shelf, clears an active search, lays out and scrolls
  to the new book, waits for its destination to settle, lands it, then opens
  its details. Adding from Statistics, Notes, and Settings uses this sequence.
- The scanner asks for ideal 1920×1080 rear-camera video, accepting what the
  device can provide. It tries the bundled reader if a native reader fails
  or keeps missing, reports repeated decoder errors, and releases the camera
  when scanning succeeds, fails, or is cancelled. Old scans cannot take over
  a newly opened scan. Accepted book/periodical codes must pass their checksum.

HTTPS, camera permission, sign-in, server access controls, storage format,
fonts, and text handling retain their existing rules. The decoder dependencies
have not changed. The guide still covers a full-frame scan; there is no new
crop, zoom, torch control, or OCR of the printed digits.

**Still to verify:** the owner's Chrome closing/settling effect, the reported
uppercase-i appearance in Firefox, and real small-barcode scanning on iPhone
13 Safari. The Turkish-letter check below is a reproduction/regression check;
this candidate does not claim to fix an unconfirmed character defect. The
phone-width automated test runs desktop Firefox, not iOS Safari.

## Deploy a separate instance

CI builds and smoke-tests amd64 and arm64 images after the frontend, scanner,
Go, and browser tests pass. Both architectures are published as:

- `ghcr.io/aloglu/bookshelf:review-fixes` — the most recently published candidate.
- `ghcr.io/aloglu/bookshelf:review-<full-commit-sha>` — the candidate for a particular commit.

Use the commit tag or image digest supplied with the test handoff to keep
results tied to one build. Settings → About shows `review-<commit-sha>`.
The commit is also in the image's `org.opencontainers.image.revision` label.

For Compose, download `docker-compose.review.yml` from this branch into a
separate directory on the server. Beside it, create `.review.env`:

```dotenv
BOOKSHELF_REVIEW_IMAGE=ghcr.io/aloglu/bookshelf:review-fixes
BOOKSHELF_REVIEW_PASSWORD=replace-with-a-password-for-this-test-instance
BOOKSHELF_REVIEW_PORT=18081
BOOKSHELF_REVIEW_BIND=127.0.0.1
```

Replace the image value with the supplied commit tag or digest, then run:

```sh
docker compose --env-file .review.env -f docker-compose.review.yml pull
docker compose --env-file .review.env -f docker-compose.review.yml up -d
docker compose --env-file .review.env -f docker-compose.review.yml ps
```

The container is `bookshelf-review`. Its named data volume belongs to the
`bookshelf-review` Compose project and is separate from your live library.
Sign in as `admin` with the test password. Start empty or restore a downloaded
Bookshelf backup into this test instance through Settings → Your library.
Do not point the test container at the live container's data folder.

For Unraid's Docker interface, the equivalent settings are: a new container
named `bookshelf-review`, the supplied review image, container port `8080`
mapped to an unused host port such as `18081`, and a new `/data` mapping such
as `/mnt/cache/appdata/bookshelf-review`. Set the admin password and the same
appropriate PUID/PGID used for your other Bookshelf container.

Give this instance its own test HTTPS hostname in your existing reverse proxy
or tunnel, routing to the new instance/port. The Compose default binds the
host port to loopback, suitable for a proxy on that host. A proxy in another
container needs a shared Docker network or a reachable host address; its
`localhost` refers to the proxy container itself. If needed, set
`BOOKSHELF_REVIEW_BIND` to the server's reachable LAN address. Set
`BOOKSHELF_REVIEW_TRUSTED_PROXIES` to the proxy address/network actually seen by
the test container, following [the proxy guide](remote-access.md#trusting-your-proxy).
Keep the phone-facing connection on trusted HTTPS. No camera restriction or
certificate bypass is needed.

To stop the candidate while retaining its test data:

```sh
docker compose --env-file .review.env -f docker-compose.review.yml down
```

## Before testing

- [ ] Record the image tag/digest, Settings → About version, browser versions,
  OS versions, and the exact iOS/Safari version on the iPhone 13.
- [ ] Confirm you are at the test hostname and can sign in. Use test books or
  a restored backup in the separate test library.
- [ ] For scrolling, use a library large enough to have many shelf rows
  (roughly 300–500 books). Also try one addition in an empty test library.
- [ ] Keep the existing release available at its original hostname for
  comparison. Record outcomes separately for Firefox, Chrome, and Safari.

## Firefox and Chrome: closing and adding

- [ ] Scroll several rows down, open a book, and use **Put back**. Repeat with
  **Escape** and a click outside the book. Expect no flash of the page's top,
  no sideways header shift, and the book to return to its visible spine.
- [ ] While a book or Add-book card is open, try scrolling the background.
  Expect the background to stay locked while the overlay's own content can scroll.
- [ ] Open a book, step to books on another row with **Next/Previous**, then
  put it back. Include **Previous** from the first book to wrap to the last,
  and a run of more than 50 steps. Expect its landing row to remain visible
  after the animation.
- [ ] In Chrome, separately compare the reported lock-in/settling effect
  throughout the closing animation. A correct final scroll position alone
  does not establish that this visual symptom is resolved.
- [ ] Add a book whose sorted position is near the beginning, middle, and end
  of the shelf. Expect the shelf to reach its position before the flight,
  the destination to stay still during the flight, and its details to open
  automatically with the new book's address in the URL.
- [ ] Repeat adding from **Statistics**, **All notes**, and **Settings**.
  Also add with a search active at phone width, where the Add button remains
  available. Expect the shelf and newly added book to appear; a former search
  must not hide it. Desktop search intentionally covers the other tools.
- [ ] Repeat under Author, Title, and Year sorting, in both directions.
  Test both a freshly loaded page and a shelf you already scrolled through.
- [ ] After adding, use Put back, browser Forward, then browser Back.
  Expect coherent book/shelf addresses and no second jump after landing.
- [ ] Repeat an addition with your OS/browser's reduced-motion preference on.
  Expect the same final book/details state without the usual flight.

## Firefox: uppercase i

- [ ] In Title, Author, Translator, and Publisher, enter
  `İstanbul İzmir Iğdır ışık — İ I i ı Ç ç Ğ ğ Ö ö Ş ş Ü ü` with your usual
  keyboard. Check both appearance and the actual text while typing.
- [ ] Save, reload, open **More → Edit details**, and check the same values.
  Repeat in the confirmation card after a catalogue lookup if that was where
  the original symptom occurred.
- [ ] If it still looks wrong, note the exact field, character, browser zoom,
  OS/display scaling, and whether copying the value elsewhere preserves it.
  Include a screenshot or short recording of the affected field. This helps
  distinguish altered text from a glyph/rendering problem.

## iPhone 13 Safari: barcode scanning

- [ ] Open the test **HTTPS** hostname. Use the same physical books that failed
  before. Keep light, distance, and framing comparable between deployments.
- [ ] Begin with a larger barcode that already worked. Confirm the captured
  ISBN matches its printed number and the camera indicator stops after success.
- [ ] Test at least ten small book barcodes (including the previously failing
  ones) and five larger controls, five attempts per book, in portrait and
  landscape. Record the barcode's approximate printed width and height.
- [ ] Time from a stable barcode in view to the number being read. Record camera
  startup separately and catalogue lookup separately. A correct number with
  “not found” or “already on your shelf” is a successful scan, not a scanner miss.
- [ ] Include 978 and 979 book codes, a separate 977 periodical if available,
  a book with a price supplement, and a scene with another barcode nearby.
- [ ] Check preview sharpness, responsiveness, and correct identifier selection.
  Repeat the previously failing books under a second ordinary indoor light.
- [ ] Cancel scanning, reopen it immediately, and scan another book. Also close
  the Add-book card during camera startup and during scanning. Expect the camera
  to stop and the next scan to work without reloading the page.
- [ ] Deny camera permission once. Expect a useful message and manual entry to
  remain available. Restore permission in Safari before the next scan test.
- [ ] If an error appears, record its wording. If access to Safari's Web Inspector
  is available, the console's `Bookshelf scanner` messages show actual stream
  dimensions and backend changes; these logs contain no images or ISBNs.

**Proposed acceptance target:** at least 95% of small-barcode attempts read the
correct identifier within 3 seconds after stable framing, at least 4/5 successes
for each previously failing book in each orientation, no wrong accepted codes,
no regression for the large controls, and no stuck camera or frozen preview.
These are targets for this deployment, not results already established on iOS.

## Report results

For each failure, record the image version, device/browser, numbered action,
expected/actual behavior, and a short recording where useful. For scanning:

| Book / expected ISBN | Barcode size | Orientation / light | Release successes / 5 | Candidate successes / 5 | Read times / wrong values / preview notes |
| --- | --- | --- | --- | --- | --- |
| | | | | | |

Keep Chrome's closing symptom and Firefox's jump as separate outcomes. A pass
on desktop Firefox at phone width does not replace a test on the actual iPhone.

## Automated verification

From a source checkout, with Node and Go installed:

```sh
cd web
npm ci
npm test
npm run build
npx playwright install chromium firefox
npm run test:browser
```

Browser tests create their own temporary library and server on port 18323,
then stop it. They cover frame-by-frame closing and arrival geometry, page
navigation, history, reduced motion, Unicode preservation, the real bundled
WASM reader with a synthetic video source, and scan cancellation/reopening.
The scanner unit tests inject startup and decoder failures. The Go suite,
`go vet ./...`, and the Docker smoke test run in CI as well. None of these
synthetic tests establish real iPhone camera performance.
