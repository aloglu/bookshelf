// What notes pages share: places in a book, their order, dates, and the icons.
import { fmt } from "../engine/model.js";

export const ICON = {
  pen: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  lock: '<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
  eye: '<path d="M2.5 12C3.5 10.2 7 6 12 6s8.5 4.2 9.5 6c-1 1.8-4.5 6-9.5 6s-8.5-4.2-9.5-6Z"/><circle cx="12" cy="12" r="3"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h9"/>',
  down: '<path d="M12 4v11M7 11l5 5 5-5M5 20h14"/>',
  link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  chevron: '<path d="m6 9 6 6 6-6"/>',
  chev: '<path d="m9 6 6 6-6 6"/>',
  swap: '<path d="m8 9 4-4 4 4M8 15l4 4 4-4"/>',
  quote: '<path d="M5 5v14" stroke-width="2.4"/><path d="M10 8h9M10 12h9M10 16h5"/>',
  check: '<path d="m5 12 5 5 9-10"/>',
  trash: '<path d="M4 7h16M10 7V4h4v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>',
  more: '<circle cx="5" cy="12" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="19" cy="12" r="1.4"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  place: '<path d="M7 4h10v16l-5-3.5L7 20Z"/>',
  date: '<rect x="4" y="5" width="16" height="15" rx="2"/><path d="M4 10h16M8 3v4M16 3v4"/>',
  reading: '<path d="M12 6.5C10.5 5 8 4.5 4 4.5v13c4 0 6.5.5 8 2 1.5-1.5 4-2 8-2v-13c-4 0-6.5.5-8 2Z"/><path d="M12 6.5v13"/>',
};

// A place in a book is a page, a chapter or a location (what Kindle gives), always a number; a location may
// be a range. The small kind button in the editor goes round them.
export const KINDS = { page: "Page", chapter: "Ch.", loc: "Loc." };
export const LONG = { page: "Page", chapter: "Chapter", loc: "Location" };
export const NEXT = { page: "chapter", chapter: "loc", loc: "page" };
export const whereText = (w) => !w ? "" : w.kind === "page" ? `Page ${fmt(w.n)}` : w.kind === "loc" ? `Location ${fmt(w.n)}${w.to ? `–${fmt(w.to)}` : ""}` : `Chapter ${fmt(w.n)}`;
// What the editor's number field holds, and what it makes of what was typed (null when it isn't a number).
export const whereField = (w) => !w ? "" : w.kind === "loc" && w.to ? `${w.n}–${w.to}` : String(w.n);
export function whereFrom(kind, raw) {
  const m = String(raw).trim().match(/^(\d+)(?:\s*[–-]\s*(\d+))?$/);
  if (!m || +m[1] < 1) return null;
  return kind === "loc" && m[2] ? { kind, n: +m[1], to: +m[2] } : { kind, n: +m[1] };
}
// Only digits go into the field (and a dash, for a range of locations).
export const typedWhere = (kind, v) => v.replace(kind === "loc" ? /[^\d–-]/g : /\D/g, "");
// In order by place: pages and locations by number; chapters and notes without a place follow, newest first.
const numbered = (w) => (w && (w.kind === "page" || w.kind === "loc") ? w.n : Infinity);
export const newest = (x, y) => Date.parse(y.createdAt) - Date.parse(x.createdAt);
export const byPlace = (x, y) => numbered(x.where) - numbered(y.where) || newest(x, y);

const NOW = () => new Date();
export const noteDate = (iso) => { const d = new Date(iso); return d.toLocaleDateString("en-US", { month: "short", day: "numeric", ...(d.getFullYear() !== NOW().getFullYear() && { year: "numeric" }) }); };
export const monthOf = (iso) => new Date(iso).toLocaleDateString("en-US", { month: "long", year: "numeric" });
