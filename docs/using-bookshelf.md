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
- You can read a book again: its earlier readings stay, and the line above the
  title says **Read twice** (or how many times).

## Notes and highlights

**Notes** on a book's page opens its notes page (`/<the book's address>/notes`).
The book closes onto its cover there; click the cover to open it again.

- A **note** is your own words, a line or a page. A **highlight** is a passage
  you liked, copied as printed, with its page if you like and a thought on it.
  A book can have as many of each as you want.
- Each one is **Only you** (the default) or **Public**, and belongs to one of
  the book's readings (the one under way, or the last) unless you choose
  another or none.
- Show all of them, or only notes or highlights, newest first or **By page**.
  **Edit** changes one where it stands; **Delete** can be undone.
- Beside them are the book's **readings**: the one under way with its page,
  and each finished or set-aside one with its dates, how long it took and how
  many pages a day. Correct or delete any of them there. Deleting a reading
  keeps what you wrote during it. On a phone, the readings fold into one line
  above the notes.

Visitors see a book's notes page once something on it is public, and only the
public notes and highlights, never your readings.

## Settings

The **Settings** icon (the sliders) opens Settings. Every change saves at once.

- **Site:** the library's name, and its icon: Bookshelf's, or **Use your own**
  with any picture (cropped to a square at its middle), for the browser tab, a
  phone's home screen and, with **Show the icon beside the name**, the header.
  **What visitors see:** a private library shows
  visitors only a sign-in page; the statistics page can be yours alone. **How it
  opens:** the sort and day or evening visitors start with. **Elsewhere:** your
  Goodreads, The StoryGraph and Instagram, shown under the name. **Links:** each
  book's address (`/9780679723424`, `/978-0-679-72342-4` or `/pale-fire`), and
  where **Search on…** looks.
- **Look:** the wall colors by day and by evening, the wood of the shelves, a
  bookcase around them, the reading ribbon's color, whether the spines show
  their bindings (hardcover, cloth, leather), the size of the books, and a limit
  on the tallest spine.
- **Library:** bring books in from a file, take them out as CSV, JSON or a full
  archive, and backups. See [Backups](backups.md).
- **Account:** the version you're running and whether a newer one is out (see
  [Updating](updating.md)), who is signed in, and **Sign out**.
