// The book as the shelf and the open book use it: the server's book, plus what drawing it needs
// (thickness and height in shelf units, a cover image, the spine's colors, a search haystack).

export const root = document.documentElement;
export const $ = (id) => document.getElementById(id);
export const EASE = "cubic-bezier(.22, 1, .36, 1)";
export const curve = "cubic-bezier(.32, .72, 0, 1)";
export const motionOK = () => !matchMedia("(prefers-reduced-motion: reduce)").matches;
export const isPhone = () => matchMedia("(max-width: 720px)").matches;
export const touch = () => matchMedia("(pointer: coarse)").matches;
// An animation's end, or a backstop in case it never reports finishing.
export const settled = (anim, ms) => new Promise((resolve) => {
  let done = false; const go = () => { if (!done) { done = true; resolve(); } };
  anim.finished.then(go, go); setTimeout(go, ms + 80);
});
// The dimming behind every layer (the open book, the cards, a cover held up): one color (--scrim), and one way
// of coming and going, in slowly and out a little slower, whatever the layer does meanwhile.
export const DIM = { in: 500, out: 560 };
export const dimIn = (el, delay = 0) => el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: motionOK() ? DIM.in : 0, delay, easing: "ease-out", fill: "backwards" });
export const dimOut = (el, delay = 0) => el.animate([{ opacity: 1 }, { opacity: 0 }], { duration: motionOK() ? DIM.out : 0, delay, easing: "ease-in-out", fill: "forwards" });
export const fmt = (n) => (n ?? 0).toLocaleString("en-US");
export const plural = (n, unit) => `${fmt(n)} ${unit}${n === 1 ? "" : "s"}`;

export const hash = (s) => { let h = 2166136261; for (const c of s) h = Math.imul(h ^ c.charCodeAt(0), 16777619); return h >>> 0; };
export const fold = (s) => String(s ?? "").toLowerCase().normalize("NFD").replace(/\p{M}/gu, "");
export const lastName = (a) => (a || "").trim().split(/\s+/).pop();
const article = /^(the|a|an) /i;

/* Spine height. Real dimensions are rarely in the catalogs, so a spine is as tall as its title needs,
   between a floor and a ceiling, with a few units of irregularity so no two neighbours line up exactly.
   Hardcover, cloth and leather books stand a little taller, as their boards do on a real shelf. */
const BINDING_HEIGHT = { paperback: 1, hardcover: 1.07, cloth: 1.07, leather: 1.05 };
export const bindingOf = (b) => (b.binding in BINDING_HEIGHT ? b.binding : "paperback");
export const BINDING_NAMES = { paperback: "Paperback", hardcover: "Hardcover", cloth: "Cloth", leather: "Leather" };
// `limit` is Look → Tallest spine: every height is scaled by the same share, so the tallest book comes
// down to the limit and the rest keep their proportions to it.
export function spineHeight(b, bindings = true, limit = 1) {
  const fit = Math.max(222, Math.min(280, 196 + b.title.length * 3.1));
  return Math.round((fit + (b.seed % 7) - 3) * (bindings ? BINDING_HEIGHT[bindingOf(b)] : 1) * limit);
}
// Spine thickness follows the page count: roughly 0.085 px per page plus the boards.
const thickness = (pages) => Math.max(18, Math.min(80, Math.round((pages || 250) * 0.085 + 7)));

const sorters = {
  author: (a, b) => lastName(a.author).localeCompare(lastName(b.author)) || a.title.localeCompare(b.title),
  title: (a, b) => a.title.replace(article, "").localeCompare(b.title.replace(article, "")),
  year: (a, b) => (a.published || 0) - (b.published || 0) || a.title.localeCompare(b.title),
};
// Sorts books by author, title or year, either way. Books without a year go last both ways.
export function sortBooks(list, by, desc = false) {
  const compare = sorters[by], dir = desc ? -1 : 1;
  return list.sort((a, b) => (by === "year" && !a.published !== !b.published ? (a.published ? -1 : 1) : dir * compare(a, b)));
}
// What each order is called, for the sort buttons' labels.
export const SORT_WAYS = { author: ["A to Z", "Z to A"], title: ["A to Z", "Z to A"], year: ["oldest first", "newest first"] };

/* Without a cover, a book gets a plain cloth binding with its title stamped on, in the cloth color the
   server gave its spine. Drawn once per title and color. */
const plain = new Map();
function plainCover(b) {
  const key = `${b.spine.color}|${b.title}|${b.author}`;
  if (plain.has(key)) return plain.get(key);
  const c = document.createElement("canvas"); c.width = 330; c.height = 500;
  const g = c.getContext("2d");
  g.fillStyle = b.spine.color; g.fillRect(0, 0, 330, 500);
  g.strokeStyle = "rgba(240, 222, 180, .55)"; g.lineWidth = 2; g.strokeRect(22, 22, 286, 456);
  g.fillStyle = "#efdcae"; g.textAlign = "center";
  g.font = "600 34px 'Source Serif 4 Variable', Georgia, serif";
  const lines = [""];
  for (const w of b.title.split(/\s+/)) { const t = (lines[lines.length - 1] + " " + w).trim(); if (g.measureText(t).width > 250 && lines[lines.length - 1]) lines.push(w); else lines[lines.length - 1] = t; }
  lines.forEach((l, i) => g.fillText(l, 165, 190 + i * 42));
  if (b.author) { g.font = "500 16px 'Instrument Sans Variable', sans-serif"; g.fillText(b.author.toUpperCase(), 165, 210 + lines.length * 42); }
  const url = c.toDataURL("image/jpeg", .9);
  plain.set(key, url);
  return url;
}

// The same cloth with nothing stamped on it: the face a book wears beside its notes, where its title is
// already written next to it.
const bare = new Map();
function bareCover(b) {
  const key = b.spine.color;
  if (bare.has(key)) return bare.get(key);
  const c = document.createElement("canvas"); c.width = 330; c.height = 500;
  const g = c.getContext("2d");
  g.fillStyle = b.spine.color; g.fillRect(0, 0, 330, 500);
  const url = c.toDataURL("image/jpeg", .9);
  bare.set(key, url);
  return url;
}

// Covers are decoded up front so nothing pops in late.
const decoded = new Map();
export function cover(url) {
  let entry = decoded.get(url);
  if (!entry) {
    const im = new Image(); im.decoding = "async"; im.src = url;
    entry = { im, loaded: false };
    entry.ready = im.decode().then(() => { entry.loaded = true; }, () => {});
    decoded.set(url, entry);
  }
  return entry;
}
// A book about to be opened (pointed at, touched): its covers start loading now, not on the click.
export function prefetch(b) { if (b) { cover(b.thumb); cover(b.cover); } }
// The cover worth showing right now: the full one once it has loaded, otherwise the small one.
export const bestCover = (b) => (cover(b.cover).loaded || !b.thumb ? b.cover : b.thumb);
export function coverImg(url) {
  const im = new Image(); im.alt = ""; im.decoding = "sync"; im.src = url;
  return im;
}

/* The library's books as the shelf draws them. A book keeps its object (and so its spine) across
   reloads; only what the server says changes. */
const kept = new Map();
export function shelfBooks(view, readings, settings) {
  const now = new Map(view.reading.map((r) => [r.bookId, r]));
  const history = new Map();
  for (const r of (readings || []).filter((r) => r.outcome !== "reading")) (history.get(r.bookId) || history.set(r.bookId, []).get(r.bookId)).push(r);
  const seen = new Set();
  const list = view.books.map((src) => {
    const b = kept.get(src.id) || { key: src.id, seed: hash(src.id) };
    // The server leaves out what is empty (hidden, readCount, entries when zero), so what the last load
    // gave and this one doesn't is gone.
    for (const k of b.given || []) if (!(k in src)) delete b[k];
    Object.assign(b, src);
    b.given = Object.keys(src);
    b.pages = src.pages || 0;
    b.t = thickness(src.pages);
    b.h = spineHeight(b, settings.bindings, settings.maxHeight / 100);
    b.cover = src.cover ? src.cover.url : plainCover(b);
    b.thumb = src.cover ? src.cover.thumb : b.cover;
    b.face = src.cover ? src.cover.thumb : bareCover(b);   // beside notes: no title stamped on a plain cover
    const r = now.get(src.id);
    b.reading = !!r;
    b.readingId = r?.id || null;
    b.readingSince = r ? Date.parse(r.started.time) : null;
    b.page = r?.page || 0;
    b.history = history.get(src.id) || [];
    b.haystack = fold([b.title, b.author, b.translator, b.publisher, b.published, b.isbn, b.issn, b.identifier, b.issue].join(" "));
    kept.set(src.id, b);
    seen.add(src.id);
    return b;
  });
  for (const id of kept.keys()) if (!seen.has(id)) kept.delete(id);
  return list;
}
export const tone = (b) => ({ c: b.spine.color, fg: b.spine.text });
// Look → bindings and Tallest spine change every height at once.
export function reheight(list, settings) { list.forEach((b) => { b.h = spineHeight(b, settings.bindings, settings.maxHeight / 100); }); }
