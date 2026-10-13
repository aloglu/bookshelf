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
  publisher, year or ISBN as you type. Esc clears it.
- **Author, Title, Year** sort the shelf. Choose the same one again to turn it
  round (Z to A, newest first); the small arrow shows which way it runs. Books
  without a year come last either way. The moon switches between day and
  evening for this visit.
- **Statistics** (the bar chart) shows the library in numbers: the books being
  read, then when they were written, how long they run, how they're bound and
  more. Each chart has a **Table** view.
- **Copy link** on a book's page copies its own address. Opening that address
  opens the book. **Search on…** looks the book up on Open Library, Google
  Books or Goodreads, as you choose in Settings.

On a phone, the tools that don't fit sit behind the **menu** button at the top
right. To keep Bookshelf on the home screen like an app: on an iPhone, open it
in Safari, tap **Share**, then **Add to Home Screen**; on Android, open it in
Chrome, tap the **⋮** menu, then **Add to Home screen** (or **Install app**).

## Signing in

Click the **Sign in** icon at the top right (or open `/signin`) and use the user
name (`admin`, unless `BOOKSHELF_ADMIN_USER` says otherwise) and your password:
`BOOKSHELF_ADMIN_PASSWORD`, or the one set with `bookshelf set-password`. You stay signed in for a week on that
device, and each week you use it extends that. The **Sign out** icon signs you
out. After five wrong passwords from one place, signing in from there waits 15
minutes.

To change the password, change `BOOKSHELF_ADMIN_PASSWORD` where Bookshelf runs
(or run `bookshelf set-password` again) and restart it.

## Adding books

Click **Add book**.

1. Type the **ISBN** (the number above the barcode; ISBN-10s work too) and
   click **Look up**. Bookshelf asks Open Library, then Google Books.
2. A card shows what was found, with the cover. Every field can be changed;
   **Replace cover** takes a picture of your own. Click **Add to the shelf** and
   the book flies into its place.

- **Scan barcode** uses the camera instead of typing. Browsers only allow the
  camera on a secure (https) address, or on the computer Bookshelf runs on; see
  [Putting your shelf online](remote-access.md).
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

On a book's page:

- **Edit** turns the title page into a form. **Save** (or Enter) keeps the
  changes; **Cancel** (or Esc) drops them.
- On the inside of the cover: **Replace cover**, **Spine color** to choose the
  spine's color by hand, and **From cover** to go back to the cover's own.
- **Hide from website** keeps the book in your library but out of visitors'
  sight. It shows faded on your shelf.
- **Remove** takes it off the shelf after asking.

To hide, show or remove several at once, click **Select**, click the books,
then choose. **Done** (or Esc) ends choosing.

## Reading

- **Reading now** on a book's page marks it as being read. A ribbon hangs from
  its spine, and the header says what you're reading. You can read several
  books at once; each gets its own ribbon.
- Keep your place on the **Statistics** page: each book you're reading has a
  card with an **On page** field, how far along you are, and how long you've
  been at it.
- **Finished reading** or **Set aside** ends a reading. Finishing offers **Add a
  note**.
- **Mark as read** is for books you read before Bookshelf, or without "Reading
  now": say when you finished, as a day, a year, or "I don't remember".
- You can read a book again: its earlier readings stay. The line above the
  title then says **Read twice** (or how many times); click it to see every
  reading with its dates and notes, and to correct or delete one.

**Notes** belong to a finished reading. They're yours alone unless you turn on
**Public note**; a public note shows on the book's page for visitors ("A
note"), with the day you finished it and nothing else about your reading.

## Settings

The **Settings** icon (the sliders) opens Settings, with its sections down the
side; on a phone, the list of sections comes first and **‹** takes you back to
it. Every change shows at once and saves itself.

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
  statistics page can be yours alone. **When they arrive:** the theme
  (**Automatic** follows their device) and the sort they start with (choose it
  again for Z to A or newest first); a visitor who switches between day and
  evening keeps their choice while they're there. **A book's page:** its
  address (ISBN, hyphenated or title, with an example beside it), and the
  **Search links** on it.
- **Your library:** bring books in from a file, take them out as CSV, JSON or a
  full archive, and backups. See [Backups](backups.md).
- **About:** who made Bookshelf and where its code is, the version you're
  running and whether a newer one is out (see [Updating](updating.md)), who is
  signed in, and **Sign out**.
