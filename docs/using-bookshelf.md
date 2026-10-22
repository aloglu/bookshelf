# Using Bookshelf

## The shelf

Open Bookshelf's address in a browser and you see your shelf: every book stands
with its spine out, in its cover's color, as thick as it is long. This is what
visitors see too.

- **Point at a book** and it comes forward. **Click it** and it flies out of
  the shelf and opens to its title page: the cover on the left, the details on
  the right. **Previous** and **Next** (or the arrow keys, or a swipe on a
  phone) step along the shelf; **Put back** (or Esc) returns it to its place.
- **Search** (the magnifying glass) finds books by title, author, translator,
  publisher, year or ISBN as you type; accents don't matter ("populer kultur"
  finds *Popüler Kültür*, "kitalar" finds *Kıtalar*). On a wide screen it opens across the
  other buttons, which come back when you clear or close it. Esc clears it.
- **Author, Title, Year** sort the shelf. Choose the same one again to turn it
  round (Z to A, newest first); the small arrow shows which way it runs. Books
  without a year come last either way. The moon switches between day and
  evening for this visit.
- **Statistics** (the bar chart) shows the library in numbers: the books being
  read, then when they were written, how long they run, how they're bound and
  more. Each chart has a **Table** view.
- Under a book's details: **Notes** (showing how many it has), **Link**, which
  copies the book's own address (opening that address opens the book), and
  **Search**, which looks the book up on Open Library, Google Books or
  Goodreads, as you choose in Settings (with more than one, it lists them).
- Your browser's **Back** and **Forward** move through Bookshelf as through any
  site: each page, each Settings section, each book you take out and each one
  you step to with Previous and Next is a step. **Put back** returns the book to
  the shelf in one go.

On a phone, the tools that don't fit sit behind the **menu** button at the top
right. To keep Bookshelf on the home screen like an app: on an iPhone, open it
in Safari, tap **Share**, then **Add to Home Screen**; on Android, open it in
Chrome, tap the **⋮** menu, then **Add to Home screen** (or **Install app**).

## Signing in

Click the **Sign in** icon at the top right (or open `/signin`). On the sign-in
page, use the user name (`admin`, unless `BOOKSHELF_ADMIN_USER` says otherwise) and your password:
`BOOKSHELF_ADMIN_PASSWORD`, or the one set with `bookshelf set-password`. You stay signed in for a week on that
device, and each week you use it extends that. The **Sign out** icon signs you
out. After five wrong passwords from one place, signing in from there waits 15
minutes.

To change the password, change `BOOKSHELF_ADMIN_PASSWORD` where Bookshelf runs
(or run `bookshelf set-password` again) and restart it.

## Adding books

Click **+** (Add book) in the header.

1. Type the **ISBN** (the number above the barcode; ISBN-10s work too) and
   click **Look up**. Bookshelf asks Open Library, then Google Books.
2. A card shows what was found, with the cover. Every field can be changed;
   **Replace cover** takes a picture of your own. Click **Add to the shelf** and
   the shelf scrolls to its place, the book flies in, and its details open.
   Adding from Statistics, All notes, or Settings returns to the shelf first;
   an active search is cleared so the new book is visible.

- **Scan barcode** uses the camera instead of typing. Browsers only allow the
  camera on a secure (https) address, or on the computer Bookshelf runs on; see
  [Putting your shelf online](remote-access.md). Hold the whole barcode steady
  in the preview. If the reader reports an error, try scanning again or type
  the number. Closing the card or choosing **Type it instead** stops the camera.
- **No ISBN, or nothing found?** Click **Enter the details by hand**. Only the
  title is required. A book without a cover gets a plain cloth cover with its
  title stamped on.
- **Periodicals** with book-like issues carry an **ISSN** instead of an ISBN.
  Type or scan it the same way (the 977 barcode works); an **Issue** field
  appears for the issue number, as printed (`70–71`). Each issue is its own
  book.
- Bookshelf won't add the same ISBN (or the same issue) twice; it offers to
  **Show it** instead.

## Changing a book

On a book's page, **More** holds what changes the book itself:

- **Edit details** turns the title page into a form. **Save** (or Enter) keeps the
  changes; **Cancel** (or Esc) drops them.
- While editing, on the inside of the cover: **Replace cover**, **Spine color**
  to choose the spine's color by hand, and **From cover** to go back to the
  cover's own.
- **Hide from website** keeps the book in your library but out of visitors'
  sight. It shows faded on your shelf.
- **Remove** takes it off the shelf after asking.

To hide, show or remove several at once, click the checkbox button (Select books) in the header, click the books,
then choose. **Done** (or Esc) ends choosing.

## Reading

- On a book's page, a line under the details says how its reading stands
  ("Reading since Oct 2 · page 54 of 130", "Read twice · last Mar 3, 2025",
  "Not read yet"), and **Reading** holds what you can do about it.
- **Reading now** marks it as being read. A ribbon hangs from
  its spine, and the header says what you're reading. You can read several
  books at once; each gets its own ribbon.
- Keep your place on the **Statistics** page: each book you're reading has a
  card with an **On page** field, how far along you are, and how long you've
  been at it.
- **Finished reading** or **Set aside** ends a reading. Finishing offers **Add a
  note**.
- **Mark as read** is for books you read before Bookshelf, or without "Reading
  now": say when you finished, as a day, a year, or "I don't remember".
- You can read a book again: its earlier readings stay, and the line about its
  reading says **Read twice** (or how many times).

## Notes

**Notes** on a book's page (its icon is how many it has) opens its notes page
(`/<the book's address>/notes`). The book closes onto its cover there; click the
cover or the title to open it again.

- A note is a **passage** from the book, copied as printed, your own words, or
  both. Write in the window at the top; **+ Passage** adds the passage (what you
  wrote so far stays above it, and Tab takes you below it for more), and
  **+ Page** adds where it is: tap the small **Page** button inside it to switch
  to **Ch.** (a chapter) or **Loc.** (a Kindle location). **Only me** or
  **Public** says who can read it; **Save** keeps it. The **×** throws a draft
  away (**Undo** brings it back).
- A book can have as many notes as you like. Each belongs to the reading under
  way when you wrote it; with more than one reading, a note says which ("2nd
  reading"), and **⋯** can show one reading or group the notes by reading.
- **Newest** or **By page** orders them. **Edit** opens a note in the same
  window: **Done** saves, **×** throws your changes away, **Delete** deletes
  it (and **Undo** brings it back), and its date opens the note's own page.
- **⋯** also says how many visitors can read, hides this book's notes from
  visitors or shows them again, copies the page's link, and downloads the notes
  as **Markdown** (for Obsidian, Notion or any text editor).
- Each note has a page of its own: click its **date**. **Copy link** under a
  public note copies that page's address, to share one note on its own.
- Under the book is a line about your **readings**: open it for the one under
  way with its page, and each finished or set-aside one with its dates, how
  long it took and how many pages a day. Correct or delete any of them there.
  Deleting a reading keeps what you wrote during it.

**All notes** (`/notes`) lists every book's notes, newest first. The pen beside
Statistics (or **All notes** in a phone's menu) opens it. **Edit** under a note
changes it right there, as on its book's page. It is always there
for you; turn it on in Settings → Visitors to let visitors open it too, and
then **As visitors see it** shows exactly what they would.

Visitors see a book's notes page once something on it is public, and only the
public notes, never your readings or which reading a note came from. They see
All notes only while it is on.

## Settings

The **Settings** icon (the sliders) opens Settings: a page with its sections
down the side, each at its own address (`/settings/visitors`, say). On a phone,
`/settings` lists the sections and each opens as its own page; **‹ Settings**
returns to the list. Click the icon again, the library's name or Esc to return
to the shelf. Every change shows at once and saves itself.

- **Shelf:** a preview of your shelf, drawn from your own books, with **Day**
  and **Evening** above it: the paint, wood and ribbon you choose are for the
  one picked there. The preview stays in sight as you scroll to the settings
  below it.
  - **Paint, wood and ribbon:** a paint for the wall, or any color of your own
    from the rainbow at the end of the row; the wood of the shelves; and the
    color of the ribbon on the books you're reading.
  - **Bookcase:** which parts to build: **Shelves** (the boards the books
    stand on), **Sides**, a **Top** and a **Back panel**, each on its own, all
    in the shelves' wood. With sides and no shelves, a base board closes the
    bottom; with none of them, the books stand in rows on their own.
  - **Books:** **Binding details** (hardcovers show their boards, cloth its
    weave, leather its bands and gilt), the size of the books, and the tallest
    a spine may stand.
- **Header and footer:** the library's name, its icon (Bookshelf's, or **Use
  your own** with any picture, cropped to a square) and whether it stands
  beside the name; your Goodreads, The StoryGraph and Instagram, shown under
  it; and a line of your own under the shelves (empty and hidden until you
  write one and turn on **Show the footer**). The name and the footer take
  Markdown:
  `[Our books](https://example.com)` makes the name a link somewhere else (the
  icon still leads home; the browser tab shows just the words), and the footer
  can have links, bold, italic and a new line for a new line.
- **Visitors:** a private library shows visitors only a sign-in page, and the
  statistics page can be yours alone. **What you're reading** shows them the
  Now reading line, the ribbons and those books on the statistics page, and
  **How far along you are** adds your page and when you started. **When they arrive:** the theme
  (**Automatic** follows their device) and the sort they start with (choose it
  again for Z to A or newest first); a visitor who switches between day and
  evening keeps their choice while they're there. **A book's page:** its
  address (ISBN, hyphenated or title, with an example beside it), and the
  **Search links** on it. **Notes:** whether visitors can read your public
  notes at all, whether they get the **All notes** page, and whether a new note
  starts as **Only me** or **Public**. **Hidden books** lists the books whose
  notes visitors can't read; find a book by title or author to add one.
- **Your library:** bring books in from a file, take them out as CSV, JSON or a
  full archive, and backups. See [Backups](backups.md). **Export notes** takes
  every book's notes, or one book's, as Markdown, with a preview first, and
  with or without the notes only you can see. **Import notes** reads a
  Kindle's `My Clippings.txt` (in its documents folder): it shows what it found,
  which books on your shelf it matched by title, and what it skips, before
  anything is added, and you choose whether the notes arrive as **Only me** or
  **Public**. A note you made beside a highlight becomes part of it, and
  bookmarks are left out. Importing the same file again adds only what's new.
  Kindles set to English are read for now.
- **About:** who made Bookshelf and where its code is, the version you're
  running and whether a newer one is out (see [Updating](updating.md)), who is
  signed in, and **Sign out**.
