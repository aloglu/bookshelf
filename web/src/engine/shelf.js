// The shelves. Each spine is created once and only ever moved: sorting, searching and resizing re-pack
// the same elements, and FLIP animates the ones on screen. Nothing here is rendered by a framework.
import { EASE, bindingOf, fold, hash, isPhone, lastName, motionOK, prefetch, sortBooks, tone } from "./model.js";

// What the shelf shows, set by the app: the books, who is looking, the query and sort, the settings.
export const view = { books: [], owner: false, query: "", sort: "author", desc: false, selecting: false, settings: null, onOpen: null, onChoose: null };

let shelvesEl = null;
export const slots = new Map();
const shelves = [];
export let scale = 1;
const EYE = `<svg class="eye" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M3 3l18 18M10.6 6.1A9.8 9.8 0 0 1 12 6c5 0 8.5 4.2 9.5 6-.4.8-1.3 2.1-2.6 3.3M6.3 7.4C4.4 8.7 3 10.7 2.5 12c1 1.8 4.5 6 9.5 6 1.7 0 3.2-.5 4.5-1.2"/></svg>`;

export function visibleBooks() {
  const q = fold(view.query.trim());
  return sortBooks(view.books.filter((b) => (view.owner || !b.hidden) && (!q || b.haystack.includes(q))), view.sort, view.desc);
}

function slotFor(b) {
  let el = slots.get(b.key);
  if (!el) {
    el = document.createElement("button");
    el.type = "button"; el.className = "slot"; el.dataset.key = b.key;
    el.innerHTML = `<div class="spine"><span class="st"></span><span class="sa"></span>${EYE}</div>`;
    slots.set(b.key, el);
  }
  el.book = b;
  paint(el, b);
  el.dataset.b = bindingOf(b);
  const t = Math.round(b.t * scale), h = Math.round(b.h * scale);
  el.style.setProperty("--t", t + "px");
  el.style.setProperty("--h", h + "px");
  return el;
}
// The spine's colors, lettering and marks, from the book as it is now.
function paint(el, b) {
  const { c, fg } = tone(b);
  el.style.setProperty("--c", c);
  el.style.setProperty("--fg", fg);
  const st = el.querySelector(".st"), sa = el.querySelector(".sa"), surname = lastName(b.author);
  if (st.textContent !== b.title) st.textContent = b.title;
  if (sa.textContent !== surname) sa.textContent = surname;
  el.setAttribute("aria-label", `${b.title} by ${b.author}`);
  el.classList.toggle("reading", !!b.reading);
  el.classList.toggle("is-hidden", !!b.hidden);
}
export function refreshSlot(b) {
  const el = slots.get(b.key);
  if (el) paint(el, b);
}
function shelf(i) {
  if (!shelves[i]) {
    const s = document.createElement("section");
    s.className = "shelf";
    s.innerHTML = `<div class="books"></div><div class="plank"></div>`;
    s.style.setProperty("--wx", -(hash("shelf" + i) % 900) + "px");
    shelves[i] = s;
  }
  return shelves[i];
}
const bookWidth = (b) => Math.round(b.t * scale) + 2;

// Shelves fill edge to edge; the leftover sliver on a full shelf is spread between the books.
// Only a shelf that is not full (usually the last) is centered.
function pack() {
  scale = (isPhone() ? .78 : 1.15) * view.settings.bookSize / 100;
  // Every shelf is spaced for the tallest book in the library, so the planks stay evenly apart.
  const tallest = view.books.reduce((m, b) => Math.max(m, b.h), 0) || 266;
  shelvesEl.style.setProperty("--row-h", Math.round(tallest * scale + 4) + "px");
  const list = visibleBooks();
  const cs = getComputedStyle(shelvesEl), avail = shelvesEl.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) - 12;
  const rows = [[]]; let used = 0;
  for (const b of list) {
    const w = bookWidth(b);
    if (rows[rows.length - 1].length && used + w > avail) { rows.push([]); used = 0; }
    rows[rows.length - 1].push(b); used += w;
  }
  // Shelves stay in the page between layouts; only the books move.
  const needed = list.length ? rows.length : 1;
  for (let i = 0; i < needed; i++) {
    const s = shelf(i);
    if (shelvesEl.children[i] !== s) shelvesEl.insertBefore(s, shelvesEl.children[i] || null);
  }
  while (shelvesEl.children.length > needed) shelvesEl.lastChild.remove();
  if (!list.length) {
    const p = document.createElement("p"); p.className = "empty";
    p.textContent = view.query ? `Nothing on these shelves matches “${view.query}”.` : view.books.length ? "Every book here is hidden from visitors." : "These shelves are empty.";
    shelf(0).firstChild.replaceChildren(p);
    shelf(0).firstChild.classList.remove("full");
  } else rows.forEach((row, i) => {
    const books = shelf(i).firstChild;
    books.replaceChildren(...row.map(slotFor));
    books.classList.toggle("full", i < rows.length - 1);
  });
  // Spines of books that are gone are let go.
  const shown = new Set(view.books.map((b) => b.key));
  for (const key of slots.keys()) if (!shown.has(key)) slots.delete(key);
}

function slotsOnScreen() {
  const out = [];
  for (const s of shelvesEl.children) {
    const r = s.getBoundingClientRect();
    if (r.bottom < -150 || r.top > innerHeight + 150) continue;
    out.push(...s.firstChild.children);
  }
  return out;
}
// The shelf out of sight (another page is showing) isn't laid out (at no width, every book would stand on
// a shelf of its own); it is laid out as it comes back.
let stale = false;
// Sort and search move books with FLIP: measure, re-pack, then animate only what is on screen.
export function relayout(animate = true) {
  if (!shelvesEl || !view.settings) return;
  if (!shelvesEl.clientWidth) { stale = true; return; }
  stale = false;
  setPeek(null);
  const before = new Map();
  if (animate && motionOK()) for (const el of slotsOnScreen()) before.set(el, el.getBoundingClientRect());
  for (const el of before.keys()) el.getAnimations().forEach((a) => a.cancel());
  pack();
  if (!before.size) return;
  slotsOnScreen().forEach((el, i) => {
    const b = before.get(el), a = el.getBoundingClientRect();
    const delay = Math.min(i * 5, 160);
    if (b) {
      const dx = b.left - a.left, dy = b.top - a.top;
      if (Math.abs(dx) + Math.abs(dy) < 1) return;
      el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "none" }], { duration: 540, delay, easing: EASE, fill: "backwards" });
    } else {
      el.animate([{ opacity: 0, transform: "translateY(16px)" }, { opacity: 1, transform: "none" }], { duration: 400, delay: 80 + delay, easing: EASE, fill: "backwards" });
    }
  });
}

// Removed spines lift off the shelf before the books close up.
export async function lift(books) {
  const els = books.map((b) => slots.get(b.key)).filter((el) => el?.isConnected);
  if (!motionOK() || !els.length) return;
  const anims = els.map((el, i) => el.animate([{ opacity: 1, transform: "none" }, { opacity: 0, transform: "translateY(-34px)" }], { duration: 300, delay: Math.min(i * 30, 240), easing: "cubic-bezier(.5, 0, .75, 0)", fill: "forwards" }));
  await Promise.all(anims.map((a) => a.finished.catch(() => {})));
  return () => els.forEach((el) => el.getAnimations().forEach((a) => a.cancel()));
}

/* ---------- Hover: the book comes forward ---------- */
let peeked = null, leaveTimer = 0;
export function setPeek(el) {
  if (el === peeked) return;
  peeked?.classList.remove("peek");
  peeked = el;
  if (el && !el.classList.contains("out")) el.classList.add("peek");
  prefetch(el?.book);   // pointed at: it may well be opened next
}

/* ---------- Choosing several books ---------- */
export const picked = new Set();
export function markChosen(b, on) {
  const el = slots.get(b.key);
  el?.classList.toggle("sel", on);
  if (on) el?.setAttribute("aria-pressed", "true"); else el?.removeAttribute("aria-pressed");
}

export function mountShelves(el) {
  shelvesEl = el;
  el.addEventListener("pointerover", (e) => {
    if (e.pointerType === "touch" || view.selecting) return;
    const s = e.target.closest(".slot");
    if (!s) return;
    clearTimeout(leaveTimer);
    setPeek(s);   // at once; a hover only moves one spine, so even a fast sweep is cheap
  });
  el.addEventListener("pointerout", (e) => {
    if (e.pointerType === "touch") return;
    const s = e.target.closest(".slot");
    if (!s || s.contains(e.relatedTarget)) return;
    if (e.relatedTarget?.closest?.(".slot")) return;
    clearTimeout(leaveTimer);
    leaveTimer = setTimeout(() => setPeek(null), 60);   // bridges the 2px gaps between books
  });
  el.addEventListener("pointerdown", (e) => prefetch(e.target.closest(".slot")?.book));   // a finger on a book, before the tap ends
  el.addEventListener("focusin", (e) => { const s = e.target.closest(".slot"); if (s && s.matches(":focus-visible")) setPeek(s); });
  el.addEventListener("focusout", () => setPeek(null));
  el.addEventListener("click", (e) => {
    const s = e.target.closest(".slot"); if (!s) return;
    if (view.selecting) return view.onChoose?.(s.book);
    view.onOpen?.(s.book, s);
  });
  let lastWidth = 0;
  new ResizeObserver(() => {
    const w = el.clientWidth;
    if (!w || (w === lastWidth && !stale)) return;   // hidden, or nothing changed
    const resized = w !== lastWidth;
    lastWidth = w;
    relayout(false);
    if (resized) onResize.forEach((f) => f());
  }).observe(el);
}
export const onResize = [];
