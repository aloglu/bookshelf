#!/usr/bin/env node
import { createInterface } from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import fs from "node:fs/promises";
import { existsSync } from "node:fs";
import { spawnSync } from "node:child_process";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = path.resolve(SCRIPT_DIR, "..");
const BOOKS_JSON = path.join(PROJECT_ROOT, "data", "books.json");
const BOOKS_JS = path.join(PROJECT_ROOT, "data", "books.js");
const COVERS_DIR = path.join(PROJECT_ROOT, "data", "covers");
const MANUAL_COVERS_DIR = path.join(PROJECT_ROOT, "data", "manual-covers");
const OPEN_LIBRARY_URL = "https://covers.openlibrary.org/b/isbn/{isbn}-L.jpg?default=false";
const DEFAULT_INSTALL_DIR = path.join(process.env.HOME || "", ".local", "share", "bookshelf");
const DEFAULT_BIN_PATH = path.join(process.env.HOME || "", ".local", "bin", "bookshelf");

const BOOK_FIELDS = [
  "title",
  "author",
  "isbn",
  "translator",
  "publisher",
  "binding",
  "published",
];

const OPTIONAL_FIELDS = BOOK_FIELDS.filter((field) => field !== "title");

function usage() {
  console.log(`Usage:
  bookshelf
  bookshelf build [--fetch-covers] [--recompute-colors]
  bookshelf add [--title VALUE] [--author VALUE] [--isbn VALUE] [--fetch-covers]
  bookshelf update --id-or-isbn VALUE [--title VALUE] [--author VALUE] [...]
  bookshelf remove --id-or-isbn VALUE
  bookshelf uninstall
  bookshelf validate`);
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith("--")) {
      args._.push(arg);
      continue;
    }

    const eq = arg.indexOf("=");
    if (eq !== -1) {
      args[arg.slice(2, eq)] = arg.slice(eq + 1);
      continue;
    }

    const key = arg.slice(2);
    const next = argv[i + 1];
    if (next && !next.startsWith("--")) {
      args[key] = next;
      i += 1;
    } else {
      args[key] = true;
    }
  }
  return args;
}

async function ensureDirs() {
  await fs.mkdir(COVERS_DIR, { recursive: true });
  await fs.mkdir(MANUAL_COVERS_DIR, { recursive: true });
}

async function readJsonFile(filePath) {
  let raw = await fs.readFile(filePath, "utf8");
  if (raw.charCodeAt(0) === 0xfeff) {
    raw = raw.slice(1);
  }
  return JSON.parse(raw);
}

async function loadBooks() {
  const books = await readJsonFile(BOOKS_JSON);
  if (!Array.isArray(books)) {
    throw new Error("data/books.json must contain a JSON array.");
  }
  return books;
}

async function saveBooks(books) {
  await fs.writeFile(BOOKS_JSON, `${JSON.stringify(books, null, 4)}\n`, "utf8");
}

async function saveBooksJs(books) {
  await fs.writeFile(
    BOOKS_JS,
    `window.booksData = ${JSON.stringify(books, null, 4)};\n`,
    "utf8",
  );
}

function normalizeBlank(value) {
  if (value === undefined || value === null) return null;
  const trimmed = String(value).trim();
  return trimmed.length ? trimmed : null;
}

function cleanIsbn(value) {
  const normalized = normalizeBlank(value);
  if (!normalized) return "";
  return normalized.replace(/[^0-9Xx]/g, "").toUpperCase();
}

function slugify(seed) {
  const slug = String(seed || "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || Math.random().toString(36).slice(2, 14);
}

function parseYear(value) {
  const normalized = normalizeBlank(value);
  if (!normalized) return null;
  const match = normalized.match(/\d{4}/);
  return match ? Number.parseInt(match[0], 10) : null;
}

function normalizeBook(inputBook) {
  const title = normalizeBlank(inputBook.title);
  const author = normalizeBlank(inputBook.author);
  const isbn = normalizeBlank(inputBook.isbn);
  const isbnClean = cleanIsbn(isbn);
  const id = normalizeBlank(inputBook.id) || (isbnClean || slugify(`${title || ""}-${author || ""}`));

  return {
    id,
    title,
    author,
    isbn,
    translator: normalizeBlank(inputBook.translator),
    publisher: normalizeBlank(inputBook.publisher),
    binding: normalizeBlank(inputBook.binding),
    published: parseYear(inputBook.published),
    cover: normalizeBlank(inputBook.cover),
    spineColor: normalizeBlank(inputBook.spineColor),
    spineTextColor: normalizeBlank(inputBook.spineTextColor),
  };
}

function validateBooks(books) {
  const errors = [];
  const ids = new Map();
  const isbns = new Map();

  books.forEach((book, index) => {
    const label = `Book ${index + 1}`;
    if (!normalizeBlank(book.id)) errors.push(`${label}: missing id`);
    if (!normalizeBlank(book.title)) errors.push(`${label}: missing title`);

    const id = normalizeBlank(book.id);
    if (id) {
      if (ids.has(id)) errors.push(`${label}: duplicate id "${id}"`);
      ids.set(id, index);
    }

    const isbn = cleanIsbn(book.isbn);
    if (isbn) {
      if (isbns.has(isbn)) errors.push(`${label}: duplicate ISBN "${isbn}"`);
      isbns.set(isbn, index);
    }

    if (book.published !== null && book.published !== undefined) {
      const year = Number(book.published);
      if (!Number.isInteger(year) || year < 0) {
        errors.push(`${label}: published must be a year or null`);
      }
    }
  });

  return errors;
}

function findBookIndex(books, idOrIsbn) {
  const needle = normalizeBlank(idOrIsbn);
  const needleIsbn = cleanIsbn(needle);
  if (!needle) return -1;
  return books.findIndex((book) => {
    return book.id === needle || (needleIsbn && cleanIsbn(book.isbn) === needleIsbn);
  });
}

function coverFilename(book) {
  const isbn = cleanIsbn(book.isbn);
  return `${isbn || book.id}.jpg`;
}

async function firstExisting(paths) {
  for (const candidate of paths) {
    if (existsSync(candidate)) return candidate;
  }
  return null;
}

async function findManualCover(book) {
  const candidates = [cleanIsbn(book.isbn), book.id].filter(Boolean);
  const extensions = [".jpg", ".jpeg", ".png", ".webp", ".bmp"];
  const paths = [];
  for (const candidate of candidates) {
    for (const ext of extensions) {
      paths.push(path.join(MANUAL_COVERS_DIR, `${candidate}${ext}`));
    }
  }
  return firstExisting(paths);
}

function hasConvert() {
  return spawnSync("convert", ["--version"], { stdio: "ignore" }).status === 0;
}

function convertImage(source, dest) {
  const result = spawnSync("convert", [source, dest], { stdio: "ignore" });
  return result.status === 0 && existsSync(dest);
}

function extractSpineColors(imagePath) {
  const result = spawnSync(
    "convert",
    [imagePath, "-resize", "1x1", "-format", "%[hex:u.p{0,0}]", "info:"],
    { encoding: "utf8" },
  );
  if (result.status !== 0) return null;
  const hex = result.stdout.trim().slice(0, 6);
  if (!/^[0-9A-Fa-f]{6}$/.test(hex)) return null;

  const r = Number.parseInt(hex.slice(0, 2), 16);
  const g = Number.parseInt(hex.slice(2, 4), 16);
  const b = Number.parseInt(hex.slice(4, 6), 16);
  const luminance = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
  return {
    background: `#${hex.toUpperCase()}`,
    text: luminance > 0.55 ? "#1c1c22" : "#fdfdfd",
  };
}

async function fetchCover(isbn, destPath) {
  const url = OPEN_LIBRARY_URL.replace("{isbn}", encodeURIComponent(isbn));
  const response = await fetch(url, {
    headers: { "User-Agent": "BookshelfCLI/1.0" },
  });
  if (!response.ok) return false;
  const buffer = Buffer.from(await response.arrayBuffer());
  if (buffer.length < 1000) return false;
  await fs.writeFile(destPath, buffer);
  return true;
}

async function buildLibrary(options = {}) {
  await ensureDirs();
  const books = (await loadBooks()).map(normalizeBook);
  const errors = validateBooks(books);
  if (errors.length) {
    throw new Error(`Validation failed:\n${errors.map((error) => `- ${error}`).join("\n")}`);
  }

  const canConvert = hasConvert();
  const stats = { books: books.length, manuals: 0, downloads: 0, colored: 0, skipped: 0 };

  for (const book of books) {
    const filename = coverFilename(book);
    const destPath = path.join(COVERS_DIR, filename);
    const manualCover = await findManualCover(book);

    if (manualCover) {
      const manualExt = path.extname(manualCover).toLowerCase();
      if (manualExt === ".jpg" || manualExt === ".jpeg") {
        await fs.copyFile(manualCover, destPath);
        stats.manuals += 1;
      } else if (canConvert && convertImage(manualCover, destPath)) {
        stats.manuals += 1;
      }
    }

    if (!existsSync(destPath) && options.fetchCovers && cleanIsbn(book.isbn)) {
      process.stdout.write(`Fetching cover: ${book.title}... `);
      try {
        if (await fetchCover(cleanIsbn(book.isbn), destPath)) {
          stats.downloads += 1;
          console.log("done");
        } else {
          console.log("not found");
        }
      } catch {
        console.log("failed");
      }
    }

    if (existsSync(destPath)) {
      book.cover = `data/covers/${filename}`;
      if (canConvert && (options.recomputeColors || !book.spineColor || !book.spineTextColor)) {
        const palette = extractSpineColors(destPath);
        if (palette) {
          book.spineColor = palette.background;
          book.spineTextColor = palette.text;
          stats.colored += 1;
        }
      }
    } else {
      book.cover = null;
      book.spineColor = null;
      book.spineTextColor = null;
      stats.skipped += 1;
    }
  }

  await saveBooks(books);
  await saveBooksJs(books);

  if (!canConvert) {
    console.warn("Warning: ImageMagick convert was not found. Spine color extraction was skipped.");
  }

  return stats;
}

async function validateCommand() {
  const books = (await loadBooks()).map(normalizeBook);
  const errors = validateBooks(books);
  if (errors.length) {
    console.error("Library validation failed:");
    errors.forEach((error) => console.error(`- ${error}`));
    process.exitCode = 1;
    return;
  }
  console.log(`Library is valid. Books: ${books.length}`);
}

async function addBook(values, options = {}) {
  const books = (await loadBooks()).map(normalizeBook);
  const book = normalizeBook(values);
  if (!book.title) {
    throw new Error("Title is required.");
  }
  if (findBookIndex(books, book.id) !== -1) {
    throw new Error(`A book with id "${book.id}" already exists.`);
  }
  const isbn = cleanIsbn(book.isbn);
  if (isbn && books.some((existing) => cleanIsbn(existing.isbn) === isbn)) {
    throw new Error(`A book with ISBN "${isbn}" already exists.`);
  }
  books.push(book);
  await saveBooks(books);
  return buildLibrary(options);
}

async function updateBook(idOrIsbn, updates, options = {}) {
  const books = (await loadBooks()).map(normalizeBook);
  const index = findBookIndex(books, idOrIsbn);
  if (index === -1) {
    throw new Error(`No book found for "${idOrIsbn}".`);
  }

  const next = normalizeBook({ ...books[index], ...updates });
  const nextIsbn = cleanIsbn(next.isbn);
  books.forEach((book, bookIndex) => {
    if (bookIndex === index) return;
    if (book.id === next.id) {
      throw new Error(`A book with id "${next.id}" already exists.`);
    }
    if (nextIsbn && cleanIsbn(book.isbn) === nextIsbn) {
      throw new Error(`A book with ISBN "${nextIsbn}" already exists.`);
    }
  });

  books[index] = next;
  await saveBooks(books);
  return buildLibrary(options);
}

async function removeBook(idOrIsbn, options = {}) {
  const books = (await loadBooks()).map(normalizeBook);
  const index = findBookIndex(books, idOrIsbn);
  if (index === -1) {
    throw new Error(`No book found for "${idOrIsbn}".`);
  }
  const [removed] = books.splice(index, 1);
  await saveBooks(books);
  const stats = await buildLibrary(options);
  return { removed, stats };
}

function valuesFromArgs(args) {
  const values = {};
  for (const field of BOOK_FIELDS) {
    if (args[field] !== undefined) {
      values[field] = args[field];
    }
  }
  return values;
}

function hasAnyValue(values) {
  return Object.values(values).some((value) => value !== undefined);
}

function printStats(stats) {
  console.log(`Done. Books: ${stats.books}. Manual covers: ${stats.manuals}. Downloaded: ${stats.downloads}. Colors: ${stats.colored}. Missing covers: ${stats.skipped}.`);
}

async function promptBookFields(rl, existing = {}) {
  const values = {};
  for (const field of BOOK_FIELDS) {
    const current = existing[field];
    const label = field === "published" ? "Published year" : field[0].toUpperCase() + field.slice(1);
    const required = field === "title" && !existing.title ? " required" : "";
    const suffix = current !== undefined && current !== null ? ` [${current}]` : "";
    const answer = await rl.question(`${label}${required}${suffix}: `);
    if (answer.trim()) {
      values[field] = answer.trim();
    } else if (current !== undefined) {
      values[field] = current;
    } else {
      values[field] = null;
    }
  }
  return values;
}

async function promptSearch(rl, books) {
  const query = (await rl.question("Search by title, author, ISBN, or id: ")).trim().toLowerCase();
  if (!query) return null;
  const matches = books.filter((book) => {
    return [book.id, book.title, book.author, book.isbn]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query));
  });

  if (!matches.length) {
    console.log("No matching books found.");
    return null;
  }

  matches.slice(0, 20).forEach((book, index) => {
    const author = book.author ? ` by ${book.author}` : "";
    const isbn = book.isbn ? ` (${book.isbn})` : "";
    console.log(`${index + 1}. ${book.title}${author}${isbn}`);
  });

  const answer = await rl.question("Choose a book number: ");
  const selected = Number.parseInt(answer, 10);
  if (!Number.isInteger(selected) || selected < 1 || selected > Math.min(matches.length, 20)) {
    console.log("Invalid selection.");
    return null;
  }
  return matches[selected - 1];
}

async function interactiveBuild(rl) {
  const fetchAnswer = await rl.question("Fetch missing covers from Open Library? [y/N]: ");
  const colorAnswer = await rl.question("Recompute all spine colors? [y/N]: ");
  const stats = await buildLibrary({
    fetchCovers: /^y(es)?$/i.test(fetchAnswer.trim()),
    recomputeColors: /^y(es)?$/i.test(colorAnswer.trim()),
  });
  printStats(stats);
}

async function interactiveAdd(rl) {
  const values = await promptBookFields(rl);
  const fetchAnswer = await rl.question("Fetch cover now? [y/N]: ");
  const stats = await addBook(values, { fetchCovers: /^y(es)?$/i.test(fetchAnswer.trim()) });
  printStats(stats);
}

async function interactiveUpdate(rl) {
  const books = (await loadBooks()).map(normalizeBook);
  const selected = await promptSearch(rl, books);
  if (!selected) return;
  console.log("Press Enter to keep the existing value.");
  const values = await promptBookFields(rl, selected);
  const fetchAnswer = await rl.question("Fetch missing cover if needed? [y/N]: ");
  const stats = await updateBook(selected.id, values, {
    fetchCovers: /^y(es)?$/i.test(fetchAnswer.trim()),
  });
  printStats(stats);
}

async function interactiveRemove(rl) {
  const books = (await loadBooks()).map(normalizeBook);
  const selected = await promptSearch(rl, books);
  if (!selected) return;
  const answer = await rl.question(`Remove "${selected.title}"? [y/N]: `);
  if (!/^y(es)?$/i.test(answer.trim())) {
    console.log("Cancelled.");
    return;
  }
  const { removed, stats } = await removeBook(selected.id);
  console.log(`Removed "${removed.title}".`);
  printStats(stats);
}

async function interactiveMain() {
  const rl = createInterface({ input, output });
  try {
    console.log("Bookshelf manager");
    console.log("1. Build / refresh library");
    console.log("2. Add a new book");
    console.log("3. Modify an existing book");
    console.log("4. Remove a book");
    console.log("5. Validate library");
    console.log("Q. Quit");

    const choice = (await rl.question("Choose an option: ")).trim().toLowerCase();
    if (choice === "1") await interactiveBuild(rl);
    else if (choice === "2") await interactiveAdd(rl);
    else if (choice === "3") await interactiveUpdate(rl);
    else if (choice === "4") await interactiveRemove(rl);
    else if (choice === "5") await validateCommand();
    else if (choice === "q" || choice === "quit") return;
    else console.log("Invalid option.");
  } finally {
    rl.close();
  }
}

async function maybePromptAdd(args, options) {
  const values = valuesFromArgs(args);
  if (args.title || !process.stdin.isTTY) {
    const stats = await addBook(values, options);
    printStats(stats);
    return;
  }

  const rl = createInterface({ input, output });
  try {
    const prompted = await promptBookFields(rl);
    const stats = await addBook(prompted, options);
    printStats(stats);
  } finally {
    rl.close();
  }
}

async function maybePromptUpdate(args, options) {
  const values = valuesFromArgs(args);
  if (args["id-or-isbn"] && (hasAnyValue(values) || !process.stdin.isTTY)) {
    const stats = await updateBook(args["id-or-isbn"], values, options);
    printStats(stats);
    return;
  }

  const rl = createInterface({ input, output });
  try {
    await interactiveUpdate(rl);
  } finally {
    rl.close();
  }
}

async function maybePromptRemove(args, options) {
  if (args["id-or-isbn"] || !process.stdin.isTTY) {
    const { removed, stats } = await removeBook(args["id-or-isbn"], options);
    console.log(`Removed "${removed.title}".`);
    printStats(stats);
    return;
  }

  const rl = createInterface({ input, output });
  try {
    await interactiveRemove(rl);
  } finally {
    rl.close();
  }
}

async function uninstallCommand(args) {
  const installDir = process.env.BOOKSHELF_INSTALL_DIR || DEFAULT_INSTALL_DIR;
  const binPath = process.env.BOOKSHELF_BIN_PATH || DEFAULT_BIN_PATH;
  const force = Boolean(args.force);
  const currentRoot = path.resolve(PROJECT_ROOT);
  const targetRoot = path.resolve(installDir);

  if (currentRoot !== targetRoot && !force) {
    console.error(`Refusing to uninstall from this checkout: ${currentRoot}`);
    console.error(`Run the installed command, or use: ${path.join(targetRoot, "bookshelf")} uninstall`);
    console.error("Use --force only if you intentionally want to remove the configured install path.");
    process.exitCode = 1;
    return;
  }

  if (existsSync(binPath)) {
    await fs.rm(binPath, { force: true });
    console.log(`Removed ${binPath}`);
  }

  if (existsSync(installDir)) {
    await fs.rm(installDir, { recursive: true, force: true });
    console.log(`Removed ${installDir}`);
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const command = args._[0];
  const options = {
    fetchCovers: Boolean(args["fetch-covers"]),
    recomputeColors: Boolean(args["recompute-colors"]),
  };

  if (!command) {
    await interactiveMain();
    return;
  }

  if (command === "help" || command === "--help" || command === "-h") {
    usage();
  } else if (command === "build") {
    printStats(await buildLibrary(options));
  } else if (command === "validate") {
    await validateCommand();
  } else if (command === "add") {
    await maybePromptAdd(args, options);
  } else if (command === "update") {
    await maybePromptUpdate(args, options);
  } else if (command === "remove" || command === "delete") {
    await maybePromptRemove(args, options);
  } else if (command === "uninstall") {
    await uninstallCommand(args);
  } else {
    usage();
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
