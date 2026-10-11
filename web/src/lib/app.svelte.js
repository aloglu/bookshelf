// The app's state, and every change the owner makes. The server is the source of truth: after a change
// the library is fetched again (it is built once per change on the server) and the shelf and the open
// book are brought up to date in place.
import { api, when } from "./api.js";
import { toast } from "./toast.js";
import { shelfBooks, plural } from "../engine/model.js";
import { applyLook } from "../engine/look.js";
import { picked, relayout, refreshSlot, view, lift, markChosen } from "../engine/shelf.js";
import { current, isOpen, refill } from "../engine/reader.js";

// Plain data the engine reads (never proxied), replaced whole on every load.
let library = $state.raw(null);
let books = $state.raw([]);
let readings = $state.raw([]);
export const data = {
  get library() { return library; },
  get books() { return books; },
  get readings() { return readings; },
};
// What the interface around the shelf shows.
export const ui = $state({
  ready: false, gated: false, name: "Bookshelf", owner: false, user: "",
  view: "shelf", query: "", sort: "author", selecting: false, chosen: 0,
});

// The settings as they are now: the owner's full set, or what visitors are given.
export const settings = () => library?.settings;

export async function load() {
  let lib;
  try {
    lib = await api("GET", "/api/library");
  } catch (err) {
    if (err.status === 403 && err.data?.private) {
      Object.assign(ui, { ready: true, gated: true, owner: false, name: err.data.name || "Bookshelf" });
      document.title = ui.name;
      return;
    }
    throw err;
  }
  const rs = lib.owner ? await api("GET", "/api/readings") : [];
  const first = !library;
  library = lib; readings = rs;
  books = shelfBooks(lib, rs, lib.settings);
  if (first) ui.sort = lib.settings.sort;
  Object.assign(ui, { ready: true, gated: false, owner: lib.owner, user: lib.user || "", name: lib.settings.name });
  document.title = ui.name;
  applyLook(lib.settings);
  Object.assign(view, { books, owner: lib.owner, settings: lib.settings, query: ui.query, sort: ui.sort });
  if (!lib.owner && ui.selecting) setSelecting(false);
}
// After a change: fetch the library again, re-lay the shelves (moving only what moved), and show the
// open book as it is now.
export async function reload({ animate = false, fade = false } = {}) {
  await load();
  relayout(animate);
  books.forEach(refreshSlot);
  if (isOpen() && current) {
    const b = books.find((x) => x.key === current.key);
    if (b) refill(b, fade);
  }
}

export function setQuery(q) { ui.query = q; view.query = q; }
export function setSort(s) { ui.sort = s; view.sort = s; relayout(); }

/* ---------- Signing in ---------- */
export async function signIn(user, password) {
  await api("POST", "/api/session", { user, password });
  await reload({ animate: true });
}
export async function signOut() {
  await api("DELETE", "/api/session");
  await reload({ animate: true });
  toast("Signed out. The shelf is as visitors see it.");
}
// The owner's session ended elsewhere (expired, signed out on another device): show what visitors see.
async function guard(run) {
  try {
    return await run();
  } catch (err) {
    if (err.status === 401) { await reload(); toast("You’re signed out. Sign in again to make changes."); }
    throw err;
  }
}

/* ---------- Books ---------- */
export const saveBook = (b, input) => guard(async () => { await api("PATCH", `/api/books/${b.id}`, input); await reload(); });
export const replaceCover = (b, file) => guard(async () => { await api("PUT", `/api/books/${b.id}/cover`, file); await reload(); });
export const setSpine = (b, color) => guard(async () => { await api("PUT", `/api/books/${b.id}/spine`, { color }); await reload(); });
export async function setHidden(list, hidden) {
  await guard(() => api("POST", "/api/books/batch", { ids: list.map((b) => b.id), action: hidden ? "hide" : "show" }));
  await reload();
  const n = list.length;
  toast(n === 1 && list[0] === current
    ? (hidden ? "Hidden from the website. It stays in your library." : "Shown on the website")
    : `${n === 1 ? "1 book" : n + " books"} ${hidden ? "hidden from" : "shown on"} the website`);
}
// Removed spines lift off the shelf, the rest close up.
export async function removeBooks(list) {
  if (!list.length) return;
  await guard(() => api("POST", "/api/books/batch", { ids: list.map((b) => b.id), action: "remove" }));
  const done = await lift(list);
  await reload({ animate: true });
  done?.();
  toast(list.length === 1 ? `“${list[0].title}” removed` : `${list.length} books removed`);
}

/* ---------- Readings ---------- */
export async function startReading(b) {
  await guard(() => api("POST", "/api/readings", { bookId: b.id }));
  await reload();
  toast(`Now reading “${b.title}”`);
}
export async function finishReading(b, openNote) {
  const r = await guard(() => api("PATCH", `/api/readings/${b.readingId}`, { end: "finished" }));
  await reload();
  toast(`Finished “${b.title}”`, { label: "Add a note", run: () => openNote(b, r.id) });
}
export async function setAside(b) {
  await guard(() => api("PATCH", `/api/readings/${b.readingId}`, { end: "set_aside" }));
  await reload();
  toast(`Set aside “${b.title}”`);
}
export async function setPage(b, page) {
  await guard(() => api("PATCH", `/api/readings/${b.readingId}`, { page }));
  await reload();
}
export async function markRead(b, finished, note) {
  const r = await guard(() => api("POST", "/api/readings", { bookId: b.id, read: finished }));
  if (note.text) await api("PUT", `/api/readings/${r.id}/note`, note);
  await reload();
  toast(`Marked “${b.title}” as read`);
}
export async function correctReading(r, started, ended, outcome) {
  await guard(() => api("PATCH", `/api/readings/${r.id}`, { started, ended, outcome }));
  await reload();
  toast("Reading corrected");
}
export async function deleteReading(r) {
  await guard(() => api("DELETE", `/api/readings/${r.id}`));
  await reload();
  toast("Reading deleted");
}
export async function saveNote(r, text, isPublic) {
  if (text) await guard(() => api("PUT", `/api/readings/${r.id}/note`, { text, public: isPublic }));
  else if (r.note) await guard(() => api("DELETE", `/api/readings/${r.id}/note`));
  await reload();
  toast(!text ? "Note removed" : isPublic ? "Note saved; visitors can read it" : "Note saved");
}
// A reading's dates in the form the server takes them.
export const whenOf = (w) => w.precision === "day" ? when("day", w.time.slice(0, 10)) : w.precision === "year" ? when("year", w.time.slice(0, 4)) : when("unknown");

/* ---------- Choosing several books ---------- */
export function setSelecting(on) {
  ui.selecting = on; view.selecting = on;
  document.documentElement.toggleAttribute("data-selecting", on);
  if (!on) { picked.forEach((b) => markChosen(b, false)); picked.clear(); }
  ui.chosen = picked.size;
}
export function toggleChosen(b) {
  if (picked.has(b)) picked.delete(b); else picked.add(b);
  markChosen(b, picked.has(b));
  ui.chosen = picked.size;
}

/* ---------- Settings ---------- */
// Changes save at once; the server checks them, and the shelf follows.
export async function patchSettings(patch) {
  const st = await guard(() => api("PATCH", "/api/settings", patch));
  library = { ...library, settings: st };
  view.settings = st;
  ui.name = st.name; document.title = st.name;
  applyLook(st);
  return st;
}
export const countText = (n) => plural(n, "book");
