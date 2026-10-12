// The open book: the flight out of the shelf (or up from a cover on the stats page or a notes page), the title page,
// stepping between books, putting it back. Its markup is made here and only ever filled in, never
// re-rendered, so nothing redraws text on a layer while it flies.
import { $, EASE, BINDING_NAMES, bestCover, bindingOf, cover, coverImg, isPhone, lastName, motionOK, root, settled, tone, touch } from "./model.js";
import { onResize, scale, setPeek, slots, view, visibleBooks } from "./shelf.js";

const I = (d) => `<svg class="i" viewBox="0 0 24 24" aria-hidden="true">${d}</svg>`;
const ICONS = {
  read: I(`<path d="M7 4h10v16l-5-4-5 4z"/>`), finished: I(`<path d="M7 4h10v16l-5-4-5 4z"/><path d="m9.5 9.5 2 2 3.5-3.5"/>`),
  aside: I(`<path d="M9.5 6v12M14.5 6v12"/>`), mark: I(`<path d="m5 12.5 4.5 4.5L19 7.5"/>`),
  show: I(`<path d="M2.5 12C3.5 10.2 7 6 12 6s8.5 4.2 9.5 6c-1 1.8-4.5 6-9.5 6s-8.5-4.2-9.5-6Z"/><circle cx="12" cy="12" r="3"/>`),
  hide: I(`<path d="M3 3l18 18M10.6 6.1A9.8 9.8 0 0 1 12 6c5 0 8.5 4.2 9.5 6-.4.8-1.3 2.1-2.6 3.3M6.3 7.4C4.4 8.7 3 10.7 2.5 12c1 1.8 4.5 6 9.5 6 1.7 0 3.2-.5 4.5-1.2"/>`),
  search: I(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.6-3.6"/>`),
  notes: I(`<path d="M6 3.5h8.5L18 7v13.5H6zM9 11h6M9 14.5h6M9 18h3.5"/>`),
};
const MARKUP = `
<div class="scrim" id="scrim"></div>
<div class="stage">
  <div class="carry" id="carry">
    <div class="tome" id="tome">
      <i class="t-back"></i>
      <i class="t-spine" id="t-spine" aria-hidden="true"><span class="t-face" id="t-face"><span class="st" id="t-st"></span><span class="sa" id="t-sa"></span></span><i class="t-glow" id="t-glow"></i></i>
      <i class="t-edge"></i><i class="t-top"></i><i class="t-bot"></i>
      <div class="t-page" id="page">
        <div class="leaf" id="leaf">
          <div class="thumbwrap"><img class="thumb" id="thumb" alt=""><label class="thumb-replace">Replace cover<input type="file" accept="image/*" id="cover-file-2" hidden></label></div>
          <div class="view">
          <div class="head" id="d-kicker"></div>
          <p class="chip">Hidden from the website</p>
          <div class="tp">
            <h2 id="d-title"></h2>
            <p class="sub" id="d-sub" hidden></p>
            <i class="orn" aria-hidden="true"></i>
            <p class="by" id="d-author"></p>
          </div>
          <dl>
            <div><dt>Publisher</dt><dd id="d-publisher"></dd></div>
            <div><dt>Published</dt><dd id="d-published"></dd></div>
            <div><dt>Pages</dt><dd id="d-pages"></dd></div>
            <div><dt id="d-id-t">ISBN</dt><dd id="d-isbn"></dd></div>
            <div id="d-issue-row" hidden><dt>Issue</dt><dd id="d-issue"></dd></div>
          </dl>
          <div class="actbar">
          <div class="actions">
            <button type="button" class="act" id="copy">${I(`<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>`)}Copy link</button>
            <a class="act" id="search-one" target="_blank" rel="noopener" hidden></a>
            <button type="button" class="act" id="search-many" hidden>${ICONS.search}Search on…</button>
            <button type="button" class="act" id="notes-btn">${ICONS.notes}Notes</button>
            <button type="button" class="act owner-only" id="edit">${I(`<path d="M4 20h4L19 9l-4-4L4 16v4ZM14 6l4 4"/>`)}Edit</button>
            <button type="button" class="act owner-only" id="reading"></button>
            <button type="button" class="act owner-only" id="reading2"></button>
            <button type="button" class="act owner-only" id="hide"></button>
            <button type="button" class="act owner-only warn" id="remove">${I(`<path d="M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12"/>`)}Remove</button>
          </div>
          <div class="sources" id="sources"><span>Search on</span><span id="source-links"></span><button type="button" class="act" id="sources-x" aria-label="Back">${I(`<path d="M6 6l12 12M18 6 6 18"/>`)}</button></div>
          <div class="confirm" id="confirm">
            <span>Remove this book?</span>
            <button type="button" class="act solid" id="remove-yes" style="background:#a3321f">Remove</button>
            <button type="button" class="act" id="remove-no">Keep it</button>
          </div>
          </div>
          </div>
          <form class="pform" id="pform" novalidate>
            <div class="head">Editing</div>
            <textarea class="pinp pt" name="title" rows="1" placeholder="Title" aria-label="Title"></textarea>
            <textarea class="pinp ps" name="subtitle" rows="1" placeholder="Subtitle, if any" aria-label="Subtitle"></textarea>
            <i class="orn" aria-hidden="true"></i>
            <input class="pinp pa" name="author" placeholder="Author" autocomplete="off" aria-label="Author">
            <div class="prows">
              <label class="prow"><span>Translator</span><input class="pinp" name="translator" autocomplete="off"></label>
              <label class="prow"><span>Publisher</span><input class="pinp" name="publisher" autocomplete="off"></label>
              <label class="prow"><span>Published</span><input class="pinp" name="published" inputmode="numeric" autocomplete="off"></label>
              <label class="prow"><span>Pages</span><input class="pinp" name="pages" inputmode="numeric" autocomplete="off"></label>
              <label class="prow"><span>Binding</span><select class="pinp" name="binding"><option value="paperback">Paperback</option><option value="hardcover">Hardcover</option><option value="cloth">Cloth</option><option value="leather">Leather</option></select></label>
              <label class="prow"><span>ISBN or ISSN</span><input class="pinp" name="identifier" autocomplete="off" spellcheck="false"></label>
              <label class="prow" id="p-issue"><span>Issue</span><input class="pinp" name="issue" autocomplete="off"></label>
            </div>
            <p class="perr" id="perr" role="alert"></p>
            <label class="pcheck"><input type="checkbox" name="visible">Show on the website</label>
            <div class="actions">
              <button type="submit" class="act solid">Save</button>
              <button type="button" class="act" id="pcancel">Cancel</button>
            </div>
          </form>
          <p class="folio" id="folio"></p>
        </div>
      </div>
      <div class="hinge"><div class="lid" id="lid">
        <div class="lid-out" id="lid-out"></div>
        <div class="lid-in"><figure class="plate" id="plate"></figure>
          <div class="ctools">
            <label class="ctool">${I(`<path d="M4 16l4-4 3 3 5-6 4 5M4 5h16v14H4z"/>`)}Replace cover<input type="file" accept="image/*" id="cover-file"></label>
            <label class="ctool"><i class="dot"></i>Spine color<input type="color" id="spine-color"></label>
            <button type="button" class="ctool" id="spine-auto">From cover</button>
          </div>
        </div>
      </div></div>
    </div>
  </div>
</div>
<div class="reader-bar" id="bar">
  <button type="button" class="btn icon" data-step="-1" aria-label="Previous book">${I(`<path d="m15 6-6 6 6 6"/>`)}</button>
  <button type="button" class="btn" id="close">${I(`<path d="M6 6l12 12M18 6 6 18"/>`)}Put back</button>
  <button type="button" class="btn icon" data-step="1" aria-label="Next book">${I(`<path d="m9 6 6 6-6 6"/>`)}</button>
</div>`;

// What the app does for the open book. Each returns once the server has answered.
export const hooks = {
  save: async () => {}, startReading: async () => {}, finishReading: async () => {}, setAside: async () => {},
  markRead: () => {}, openNotes: () => {}, setHidden: async () => {}, remove: async () => {},
  replaceCover: async () => {}, setSpine: async () => {}, toast: () => {}, opened: () => {}, closed: () => {},
};

let reader, scrim, carry, tome, lid, leaf, bar, page, stage, pform;
export let current = null;
let outSlot = null, busy = false, returnFocus = null, size = null;
const lerp = (a, b, t) => a + (b - a) * t;
// One transform shape for every pose so the browser can interpolate between them.
const pose = (x, y, z, s, back, rx, ry) => `translate3d(${x}px, ${y}px, ${z}px) scale3d(${s}, ${s}, ${s}) translateZ(${back}px) rotateX(${rx}deg) rotateY(${ry}deg)`;
export const isOpen = () => reader && !reader.hidden;
export const isBusy = () => busy;

// Desktop shows the whole spread; a phone centers the first page and lets the open board run off the left edge.
function measure() {
  const vw = innerWidth, room = innerHeight - 120;
  if (isPhone()) {
    const W = Math.min(vw - 40, 400);
    size = { W, H: Math.min(Math.round(W * 1.5), room), shift: 0 };
  } else {
    let H = Math.min(660, room), W = Math.round(H * .68);
    if (2 * W > vw - 64) { W = Math.floor((vw - 64) / 2); H = Math.round(W / .68); }
    size = { W, H, shift: Math.round(W / 2) };
  }
  carry.style.setProperty("--W", size.W + "px");
  carry.style.setProperty("--H", size.H + "px");
  carry.style.left = Math.round((innerWidth - size.W) / 2) + "px";
  carry.style.top = Math.round((innerHeight - size.H) / 2 - 26) + "px";
}
const depth = (b) => Math.max(12, Math.round(b.t * size.H / b.h));
// At rest the first page sits exactly at depth zero and on whole pixels, so it is painted like flat
// paper: any fraction there shows as faint seams between paint tiles when the page repaints.
const restBack = (b) => 1 - depth(b) / 2;
// The open board lies 1.5px above the page; once the book is at rest it is drawn at depth zero too, so the
// cover plate and anything on the endpaper are as sharp as the page (see .reader.flat).
const flat = (on) => reader.classList.toggle("flat", on);
const rest = (b) => pose(0, 0, 0, 1, restBack(b), 0, 0);
const smooth = (a, b, t) => { const x = Math.min(1, Math.max(0, (t - a) / (b - a))); return x * x * (3 - 2 * x); };
// Where the book would be if it stood on its shelf, seen spine on, in the open book's coordinates.
function shelfPose(b) {
  const el = slots.get(b.key);
  if (!el?.isConnected) return null;
  const r = el.getBoundingClientRect(), row = el.parentElement.getBoundingClientRect(), shelf = el.closest(".shelf").getBoundingClientRect();
  return {
    x: r.left + r.width / 2 - (carry.offsetLeft + size.W / 2), y: r.top + r.height / 2 - (carry.offsetTop + size.H / 2), s: r.height / size.H,
    r, top: row.top - 2, bottom: shelf.bottom + 2,
  };
}
/* The whole flight is one sampled path, so nothing comes to rest on the way. The book first comes
   straight out until it is clear of its neighbours, then travels and turns to face you.
   While any of it is still inside the shelf:
   - the eye moves to straight in front of it, so its sides, top and bottom stay edge-on and can never
     sweep across the books beside it or the plank below it;
   - the stage is clipped to its own gap in the row, widened only by how far its front has come out,
     and to the plank's top edge, so whatever is still inside stays hidden as a real shelf hides it.
   Once it is clear the eye drifts back to the middle of the window and the clip falls away. */
function flightPath(p, b, z0) {
  const W = size.W, P = 1800;
  const clear = W * p.s + 10, cx = p.r.left + p.r.width / 2, cy = p.r.top + p.r.height / 2;
  const ex0 = cx, ey0 = cy;
  const vw = innerWidth + 10, vh = innerHeight + 10, t0 = p.top, t1 = p.bottom;
  const frames = [], clips = [];
  for (let i = 0, n = 40; i <= n; i++) {
    const t = i / n, out = smooth(0, .36, t), go = smooth(.22, 1, t), turn = smooth(.3, 1, t);
    const z = lerp(z0, clear, out) * (1 - go);
    const ex = lerp(lerp(ex0, cx, out), innerWidth / 2, go), ey = lerp(lerp(ey0, cy, out), innerHeight / 2, go);
    frames.push({ transform: pose(p.x * (1 - go), p.y * (1 - go) - 28 * Math.sin(Math.PI * go), z, lerp(p.s, 1, go), lerp(-W / 2, restBack(b), turn), 6 * Math.sin(Math.PI * turn), 90 * (1 - turn)) });
    const inside = go < .02 && z < clear, k = z / (P - z);
    const dx = inside ? k * (Math.abs(cx - ex) + p.r.width / 2) + 2 : 4000;
    const dy = inside ? k * (Math.abs(cy - ey) + p.r.height / 2) + 2 : 4000;
    const l = Math.max(-10, p.r.left - dx), r = Math.min(vw, p.r.right + dx), cb = Math.min(vh, p.r.bottom + dy);
    // Two shapes: everything above the row plus the book's gap down to the plank, and everything below the shelf.
    const path = `M -10 -10 L ${vw} -10 L ${vw} ${t0} L ${r} ${t0} L ${r} ${cb} L ${l} ${cb} L ${l} ${t0} L -10 ${t0} Z M -10 ${t1} L ${vw} ${t1} L ${vw} ${vh} L -10 ${vh} Z`;
    clips.push({ perspectiveOrigin: `${ex}px ${ey}px`, clipPath: `path("${path}")` });
  }
  return { frames, clips };
}
function markOut(b) {
  outSlot?.classList.remove("out");
  outSlot = b ? slots.get(b.key) : null;
  outSlot?.classList.add("out");
}

const SOURCES = {
  openlibrary: { name: "Open Library", url: (b) => b.isbn ? `https://openlibrary.org/isbn/${b.isbn}` : `https://openlibrary.org/search?q=${encodeURIComponent(b.title + " " + b.author)}` },
  google: { name: "Google Books", url: (b) => `https://www.google.com/search?tbm=bks&q=${b.isbn ? "isbn:" + b.isbn : encodeURIComponent(b.title + " " + b.author)}` },
  goodreads: { name: "Goodreads", url: (b) => `https://www.goodreads.com/search?q=${b.isbn || encodeURIComponent(b.title + " " + b.author)}` },
};
// One source: a "Search on Goodreads" link beside Copy link. Several: "Search on…" opens the row of them.
function fillLinks(b) {
  const on = Object.keys(SOURCES).filter((k) => view.settings.sources.includes(k));
  const one = $("search-one"), many = $("search-many");
  one.hidden = on.length !== 1; many.hidden = on.length < 2;
  if (on.length === 1) { one.href = SOURCES[on[0]].url(b); one.innerHTML = `${ICONS.search}Search on ${SOURCES[on[0]].name}`; }
  $("source-links").replaceChildren(...on.map((k) => Object.assign(document.createElement("a"), { className: "act", href: SOURCES[k].url(b), target: "_blank", rel: "noopener", textContent: SOURCES[k].name })));
  if (on.length < 2) reader.classList.remove("searching");
}
const timesText = (n) => n === 1 ? "Read once" : n === 2 ? "Read twice" : `Read ${n} times`;

export function fill(b) {
  current = b;
  dropZoom();
  const { c, fg } = tone(b), t = Math.round(b.t * scale), h = Math.round(b.h * scale);   // the shelf spine's size, as the shelf draws it
  tome.style.setProperty("--c", c);
  tome.style.setProperty("--fg", fg);
  tome.style.setProperty("--D", depth(b) + "px");
  tome.style.setProperty("--k", (size.H / h).toFixed(5));
  tome.dataset.b = bindingOf(b);
  const face = $("t-face");
  face.style.setProperty("--t", t + "px"); face.style.setProperty("--h", h + "px");
  $("t-st").textContent = b.title;
  $("t-sa").textContent = lastName(b.author);
  showCover(b);
  const list = visibleBooks(), i = list.findIndex((x) => x.key === b.key);
  [list[(i + 1) % list.length], list[(i - 1 + list.length) % list.length]].forEach((n) => n && cover(n.cover));   // the neighbours, ready for stepping
  // The line above the title: the binding, and whether it's being read or (for the owner) how often it was.
  const times = b.readCount || 0;
  $("d-kicker").textContent = [BINDING_NAMES[bindingOf(b)], b.reading ? (view.owner ? "Reading now" : "Currently reading") : view.owner && times ? timesText(times) : ""].filter(Boolean).join(" · ");
  // Its notes page: always the owner's to write in; for visitors, once something on it is public.
  $("notes-btn").hidden = !view.owner && !b.entries;
  $("reading").innerHTML = b.reading ? `${ICONS.finished}Finished reading` : `${ICONS.read}Reading now`;
  $("reading2").innerHTML = b.reading ? `${ICONS.aside}Set aside` : `${ICONS.mark}Mark as read`;
  // "Title: Subtitle" sets the subtitle on its own line; long titles step down in size so they fit the page.
  const [title, ...more] = b.title.split(/:\s+/), sub = more.join(": ");
  $("d-title").textContent = title;
  $("d-title").dataset.len = title.length > 48 ? "longer" : title.length > 26 ? "long" : "";
  $("d-sub").textContent = sub; $("d-sub").hidden = !sub;
  $("d-author").textContent = b.author;
  $("d-publisher").textContent = b.publisher || "Unknown";
  $("d-published").textContent = b.published || "Unknown";
  $("d-id-t").textContent = b.issn ? "ISSN" : "ISBN";
  $("d-isbn").textContent = b.identifier ? b.identifier.replace(/^ISSN /, "") : "None";
  $("d-issue-row").hidden = !b.issue; $("d-issue").textContent = b.issue;
  $("d-pages").textContent = b.pages ? b.pages.toLocaleString("en-US") : "Unknown";
  $("folio").textContent = `${(i + 1).toLocaleString("en-US")} of ${list.length.toLocaleString("en-US")}`;
  $("hide").innerHTML = b.hidden ? `${ICONS.show}Show on website` : `${ICONS.hide}Hide from website`;
  reader.classList.toggle("is-hidden", !!b.hidden);
  reader.classList.remove("editing", "asking");
  fillLinks(b);
  tome.style.transform = rest(b);
}
const stop = (...els) => els.forEach((el) => el.getAnimations().forEach((a) => a.cancel()));

// Click: the book comes out of the shelf, flies to the middle turning to show its cover, then opens.
const faded = () => getComputedStyle(root).getPropertyValue("--faded").trim();   // animations want the literal filter
let flight = null;   // the opening's animations, kept so Esc can rewind them mid-air
let soloFrom = null;   // the cover on the page (stats or notes) a book rose from, if it did
// Where the eye stands for a book over a cover: straight in front of it, so on the way up (and back down)
// only its face shows, never the edges of its pages.
const coverEye = (img) => { const r = img.getBoundingClientRect(); return `${Math.round(r.left + r.width / 2)}px ${Math.round(r.top + r.height / 2)}px`; };
const restEye = () => `${Math.round(carry.offsetLeft + size.W / 2)}px ${Math.round(carry.offsetTop + size.H / 2)}px`;   // the closed book's centre at rest
// The closed book's pose that sits exactly over a cover on the page: centred on it, scaled to its height.
function coverPose(b, img) {
  const r = img.getBoundingClientRect();
  if (!r.height) return null;
  return pose(r.left + r.width / 2 - (carry.offsetLeft + size.W / 2), r.top + r.height / 2 - (carry.offsetTop + size.H / 2), 0, r.height / size.H, restBack(b), 0, 0);
}
let lastInput = "pointer";
export async function openBook(b, from) {
  if (busy || !reader.hidden) return;
  busy = true; returnFocus = from || null;
  // A hovered spine stands a few pixels proud of the row, and the flight takes off from right there.
  const lifted = !!from?.classList?.contains("peek"), lift = lifted ? parseFloat(getComputedStyle(from).getPropertyValue("--lift")) || 0 : 0;
  setPeek(null);
  const solo = !!root.dataset.view;   // the stats page or a notes page: no shelf to fly from
  reader.classList.toggle("solo", solo);
  reader.hidden = false;
  measure(); fill(b);
  hooks.opened(b);
  const p = solo ? null : shelfPose(b);
  if (p) p.y -= lift;
  // From a cover on the stats page: the closed book starts exactly over that cover, facing you.
  soloFrom = solo ? (from?.matches?.("img") ? from : from?.querySelector?.("img")) || null : null;
  const fromCover = soloFrom && coverPose(b, soloFrom);
  if (soloFrom) soloFrom.style.visibility = "hidden";
  markOut(b);
  page.scrollTop = 0;
  lid.style.transform = "rotateY(-180deg)";
  carry.style.transform = `translateX(${size.shift}px)`;
  $("close").focus({ preventScroll: true });
  if (motionOK() && p) {
    const { frames, clips } = flightPath(p, b, 0), fly = 760;
    const open = { duration: 560, delay: fly * .8, easing: "cubic-bezier(.45, 0, .2, 1)", fill: "backwards" };
    // The spine leaves the shelf looking exactly as it did there (lit if hovered, faded if hidden), then settles.
    const swing = lid.animate([{ transform: "rotateY(0deg)" }, { transform: "rotateY(-180deg)" }], open);
    const anims = [
      scrim.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 600, easing: "ease-out", fill: "backwards" }),
      tome.animate(frames, { duration: fly, fill: "backwards" }),
      stage.animate(clips, { duration: fly, fill: "backwards" }),
      swing,
      carry.animate([{ transform: "translateX(0)" }, { transform: `translateX(${size.shift}px)` }], open),
      bar.animate([{ opacity: 0, transform: "translateY(10px)" }, { opacity: 1, transform: "none" }], { duration: 300, delay: 960, easing: EASE, fill: "backwards" }),
      $("t-glow").animate([{ opacity: lifted ? 1 : 0 }, { opacity: 0 }], { duration: fly * .3, fill: "backwards" }),
      $("t-spine").animate([{ filter: b.hidden ? faded() : "none" }, { filter: "none" }], { duration: fly * .35, fill: "backwards" }),
    ];
    flight = { anims, rewinding: false };
    await settled(swing, fly * .8 + 560 + 500);   // the backstop only matters if the animation never reports finishing
    if (flight.rewinding) return;
    flight = null;
  } else if (motionOK() && fromCover) {
    const rise = 520, open = { duration: 560, delay: rise * .75, easing: "cubic-bezier(.45, 0, .2, 1)", fill: "backwards" };
    const swing = lid.animate([{ transform: "rotateY(0deg)" }, { transform: "rotateY(-180deg)" }], open);
    const anims = [
      scrim.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 500, easing: "ease-out", fill: "backwards" }),
      tome.animate([{ transform: fromCover }, { transform: rest(b) }], { duration: rise, easing: "cubic-bezier(.32, .72, 0, 1)", fill: "backwards" }),
      stage.animate([{ perspectiveOrigin: coverEye(soloFrom) }, { perspectiveOrigin: restEye() }], { duration: rise, easing: "cubic-bezier(.32, .72, 0, 1)", fill: "backwards" }),
      swing,
      carry.animate([{ transform: "translateX(0)" }, { transform: `translateX(${size.shift}px)` }], open),
      bar.animate([{ opacity: 0, transform: "translateY(10px)" }, { opacity: 1, transform: "none" }], { duration: 300, delay: rise * .75 + 400, easing: EASE, fill: "backwards" }),
    ];
    flight = { anims, rewinding: false };
    // Settled only once the cover has finished its swing (the outside of the cover is put away after that).
    await settled(swing, rise * .75 + 560 + 500);
    if (flight.rewinding) return;   // Esc turned it round; the rewind finishes the job
    flight = null;
  }
  busy = false; flat(true);
}
// Esc or a click outside while the book is still on its way: play the opening backwards from wherever
// it has got to, so it goes straight back into its gap.
async function rewind() {
  const f = flight;
  f.rewinding = true;
  f.anims.forEach((a) => a.reverse());
  await Promise.all(f.anims.map((a) => settled(a, 1400)));
  putAway(current);
}
function putAway(b) {
  markOut(null); flat(false);
  if (soloFrom) { soloFrom.style.visibility = ""; soloFrom = null; }
  reader.hidden = true;
  stop(tome, lid, carry, bar, scrim, stage, $("t-glow"), $("t-spine"));
  flight = null; busy = false;
  const el = slots.get(b.key);
  // Back to its spine for someone on the keyboard; anyone who tapped or clicked gets no ring left behind.
  const home = returnFocus?.isConnected && (returnFocus.book?.key === b.key || !returnFocus.classList.contains("slot")) ? returnFocus : el;   // a stats cover, or its spine
  if (lastInput === "key") home?.focus({ preventScroll: true });
  else document.activeElement?.blur?.();
  hooks.closed(b);
}
// Put back: the book closes, then retraces the same path back into its gap on the shelf.
export async function closeBook() {
  if (!isOpen()) return;
  if (busy) { if (flight && !flight.rewinding) rewind(); return; }
  busy = true; flat(false);
  const b = current, el = soloFrom ? null : slots.get(b.key);
  if (el?.isConnected) {
    const r = el.getBoundingClientRect();
    if (r.bottom < 60 || r.top > innerHeight - 60) el.scrollIntoView({ block: "center", behavior: "instant" });
  }
  const p = soloFrom ? null : shelfPose(b);
  if (motionOK()) {
    stop(tome, lid, carry, bar, scrim, stage);
    const shut = { duration: 420, easing: "cubic-bezier(.5, 0, .3, 1)", fill: "forwards" }, wait = 300, fly = 720;
    bar.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 140, fill: "forwards" });
    lid.animate([{ transform: "rotateY(-180deg)" }, { transform: "rotateY(0deg)" }], shut);
    carry.animate([{ transform: `translateX(${size.shift}px)` }, { transform: "translateX(0)" }], shut);
    let back;
    const toCover = soloFrom && coverPose(b, soloFrom);
    if (toCover) {
      const down = { duration: 480, delay: wait, easing: "cubic-bezier(.5, 0, .3, 1)", fill: "forwards" };
      back = tome.animate([{ transform: rest(b) }, { transform: toCover }], down);
      stage.animate([{ perspectiveOrigin: restEye() }, { perspectiveOrigin: coverEye(soloFrom) }], down);
    } else if (p) {
      const { frames, clips } = flightPath(p, b, 0);
      back = tome.animate(frames.reverse(), { duration: fly, delay: wait, fill: "forwards" });
      stage.animate(clips.reverse(), { duration: fly, delay: wait, fill: "forwards" });
      if (b.hidden) $("t-spine").animate([{ filter: "none" }, { filter: faded() }], { duration: fly * .35, delay: wait + fly * .65, fill: "forwards" });
    } else back = tome.animate([{ opacity: 1 }, { opacity: 0, transform: rest(b) + " scale(.9)" }], { duration: 260, delay: wait, easing: "ease-in", fill: "forwards" });
    scrim.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 560, delay: wait + 100, easing: "ease-in-out", fill: "forwards" });
    await settled(back, wait + (toCover ? 480 : p ? fly : 260));
  }
  putAway(b);
}
// The book closes onto a cover on a page put beneath it (its notes page), as it goes back to the stats
// page. under() shows that page and returns the cover; it returns false when the book can't close.
export async function closeOnto(under) {
  if (!isOpen() || busy) return false;
  if (reader.classList.contains("editing")) { hooks.toast("Save or cancel your changes first"); return false; }
  const img = await under();
  if (soloFrom) soloFrom.style.visibility = "";
  reader.classList.add("solo");
  soloFrom = returnFocus = img || null;
  if (img) img.style.visibility = "hidden";
  await closeBook();
  return true;
}
// Next and previous swap the book in your hands: the cover plate changes at once, the page fades through.
function step(dir) {
  if (busy || !current || reader.classList.contains("solo")) return;
  if (reader.classList.contains("editing")) return hooks.toast("Save or cancel your changes first");
  const list = visibleBooks();
  const i = list.findIndex((b) => b.key === current.key);
  const next = list[(i + dir + list.length) % list.length];
  markOut(next);
  fill(next);
  hooks.opened(next);
  page.scrollTop = 0;
  leaf.getAnimations().forEach((a) => a.cancel());
  if (motionOK()) leaf.animate([{ opacity: .15, transform: `translateX(${dir * 14}px)` }, { opacity: 1, transform: "none" }], { duration: 300, easing: EASE });
}
// The book in hand changed on the server (an edit, a reading): show it as it is now.
export function refill(b, fade = false) {
  if (!isOpen() || current?.key !== b.key || reader.classList.contains("editing")) return;
  fill(b);
  if (fade && motionOK()) leaf.animate([{ opacity: .3 }, { opacity: 1 }], { duration: 320, easing: EASE });
}

// The new book appears in front of you, the shelf makes room, and it flies into its place.
export async function flyIn(b, relayout) {
  relayout(true);
  markOut(b);
  const el = slots.get(b.key);
  if (!el) return;
  const r = el.getBoundingClientRect();
  if (r.bottom < 80 || r.top > innerHeight - 80) el.scrollIntoView({ block: "center", behavior: "instant" });
  busy = true; returnFocus = el;
  reader.classList.remove("solo");
  reader.hidden = false;
  measure(); fill(b);
  lid.style.transform = "rotateY(0deg)"; carry.style.transform = "translateX(0)";
  bar.style.visibility = "hidden";
  if (motionOK()) {
    scrim.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 260, fill: "both" });
    tome.animate([{ opacity: 0, transform: `${rest(b)} translateY(26px)` }, { opacity: 1, transform: `${rest(b)} translateY(0px)` }], { duration: 420, easing: EASE });
    await new Promise((res) => setTimeout(res, 700));   // let the shelf finish making room before aiming at the gap
    const p = shelfPose(b);
    if (p) {
      const { frames, clips } = flightPath(p, b, 0);
      const land = tome.animate(frames.reverse(), { duration: 820, fill: "forwards" });
      stage.animate(clips.reverse(), { duration: 820, fill: "forwards" });
      scrim.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 560, delay: 260, easing: "ease-in-out", fill: "forwards" });
      await settled(land, 820);
    }
  }
  bar.style.visibility = "";
  putAway(b);
}

// The cover is a small thumbnail at the head of the page on a phone. A tap lifts a copy of it to the middle of
// the screen as large as fits; a tap anywhere puts it back exactly where it came from.
let unzoom = null;
export const zoomed = () => !!document.querySelector(".thumb-zoom");
export const putZoomBack = () => { unzoom?.(); unzoom = null; };
function dropZoom() { document.querySelectorAll(".thumb-zoom, .thumb-scrim").forEach((e) => e.remove()); const th = $("thumb"); if (th) th.style.visibility = ""; unzoom = null; }
function zoom() {
  if (reader.classList.contains("editing")) return $("cover-file-2").click();
  const th = $("thumb"), r = th.getBoundingClientRect(), ratio = th.naturalWidth / th.naturalHeight || 2 / 3;
  const W = Math.min(innerWidth - 48, (innerHeight - 120) * ratio), H = W / ratio;
  const big = Object.assign(document.createElement("img"), { src: th.src, alt: "", className: "thumb-zoom" });
  Object.assign(big.style, { left: (innerWidth - W) / 2 + "px", top: (innerHeight - H) / 2 + "px", width: W + "px", height: H + "px" });
  const sc = Object.assign(document.createElement("div"), { className: "thumb-scrim" });
  document.body.append(sc, big); th.style.visibility = "hidden";
  const from = `translate(${r.left - (innerWidth - W) / 2}px, ${r.top - (innerHeight - H) / 2}px) scale(${r.width / W})`;
  const opts = { duration: motionOK() ? 320 : 0, easing: EASE };
  big.animate([{ transform: from }, { transform: "none" }], opts);
  sc.animate([{ opacity: 0 }, { opacity: 1 }], opts);
  const back = async () => {
    big.removeEventListener("click", back); sc.removeEventListener("click", back);
    sc.animate([{ opacity: 1 }, { opacity: 0 }], { ...opts, fill: "forwards" });
    await settled(big.animate([{ transform: "none" }, { transform: from }], { ...opts, fill: "forwards" }), opts.duration);
    big.remove(); sc.remove(); th.style.visibility = "";
  };
  big.addEventListener("click", back); sc.addEventListener("click", back);
  unzoom = back;
}

// Esc on the open book: an edit, a question or the sources close before the book does.
export function dismissInBook() {
  if (!isOpen()) return false;
  for (const c of ["editing", "asking"]) if (reader.classList.contains(c)) { reader.classList.remove(c); return true; }
  if (reader.classList.contains("searching")) { reader.classList.remove("searching"); $("search-many").focus({ preventScroll: true }); return true; }
  return false;
}

/* ---------- The owner's tools on the open book ---------- */
const fitText = (el) => { el.style.height = "auto"; el.style.height = el.scrollHeight + "px"; };
// A periodical's ISSN (eight characters, or its 977 barcode) has an issue too.
const issnLike = (v) => { const d = v.replace(/[^\dx]/gi, ""); return /issn/i.test(v) || d.length === 8 || (d.length === 13 && d.startsWith("977")); };
function showIssue() { const f = pform.elements; $("p-issue").hidden = !issnLike(f.identifier.value.trim()) && !f.issue.value; }
function startEdit() {
  const b = current, f = pform.elements, [title, ...more] = b.title.split(/:\s+/);
  f.title.value = title; f.subtitle.value = more.join(": ");
  f.author.value = b.author || ""; f.translator.value = b.translator || ""; f.publisher.value = b.publisher || "";
  f.published.value = b.published ?? ""; f.pages.value = b.pages || ""; f.binding.value = bindingOf(b);
  f.identifier.value = b.identifier || ""; f.issue.value = b.issue || ""; f.visible.checked = !b.hidden;
  showIssue();
  $("perr").textContent = "";
  pform.querySelectorAll(".bad").forEach((el) => el.classList.remove("bad"));
  $("spine-color").value = tone(b).c;
  reader.classList.add("editing");
  pform.querySelectorAll("textarea").forEach(fitText);
  page.scrollTop = 0;
  if (!touch()) f.title.focus({ preventScroll: true });   // on a phone the reader taps the field they want; nothing opens the keyboard for them
}
// The form as the server takes it, with the checks worth making before asking.
function readForm() {
  const f = pform.elements, v = (n) => (f[n]?.value || "").trim(), sub = v("subtitle");
  const num = (n) => v(n) ? +v(n) : null;
  const data = {
    title: v("title") + (sub ? ": " + sub : ""), author: v("author"), translator: v("translator"), publisher: v("publisher"),
    binding: f.binding.value, identifier: v("identifier"), issue: v("issue"),
    published: num("published"), pages: num("pages"), hidden: !f.visible.checked,
  };
  const bad = [];
  if (!v("title")) bad.push(["title", "A book needs a title."]);
  if (v("published") && !/^\d{1,4}$/.test(v("published"))) bad.push(["published", "The year should be a number, like 1962."]);
  if (v("pages") && !(/^\d{1,5}$/.test(v("pages")) && +v("pages") > 0)) bad.push(["pages", "Pages should be a whole number."]);
  return { data, bad };
}
function markBad(name, msg) {
  pform.querySelectorAll(".bad").forEach((el) => el.classList.remove("bad"));
  if (name) (pform.elements[name].closest(".prow") || pform.elements[name]).classList.add("bad");
  $("perr").textContent = msg || "";
}
function paintSpine(color) {
  tome.style.setProperty("--c", color);
}
// A new cover: the boards, the plate, the thumbnail and the spine change; the page, and any edit on it, stay.
function recover(b) {
  const { c, fg } = tone(b);
  tome.style.setProperty("--c", c); tome.style.setProperty("--fg", fg);
  showCover(b);
}
/* The cover on the outside board, the plate inside and a phone's thumbnail. On a slow connection the full
   cover may not have arrived yet when the book opens: the small one (the shelf's thumbnail) stands in, and
   the full one takes its place the moment it has loaded, so the book never opens without a cover. */
function showCover(b) {
  const put = (url) => {
    $("lid-out").replaceChildren(coverImg(url));
    $("plate").replaceChildren(coverImg(url));
    $("thumb").src = url;
    // The plate keeps the cover's own proportions, as large as the board allows.
    const im = cover(url).im, ratio = im.naturalWidth && im.naturalHeight ? im.naturalWidth / im.naturalHeight : 2 / 3;
    const pw = Math.min(size.W * .8, size.H * .84 * ratio);
    tome.style.setProperty("--pw", Math.round(pw) + "px");
    tome.style.setProperty("--ph", Math.round(pw / ratio) + "px");
  };
  const url = bestCover(b);
  put(url);
  if (url !== b.cover) cover(b.cover).ready.then(() => { if (current === b && cover(b.cover).loaded) put(b.cover); });
}

export function mountReader(el) {
  reader = el;
  el.innerHTML = MARKUP;
  scrim = $("scrim"); carry = $("carry"); tome = $("tome"); lid = $("lid"); leaf = $("leaf"); bar = $("bar"); page = $("page");
  stage = el.querySelector(".stage"); pform = $("pform");
  addEventListener("keydown", () => { lastInput = "key"; }, true);
  addEventListener("pointerdown", () => { lastInput = "pointer"; }, true);
  // A swipe across the page steps to the next or previous book.
  let sx = null, sy = 0;
  page.addEventListener("pointerdown", (e) => { if (e.pointerType !== "mouse") { sx = e.clientX; sy = e.clientY; } });
  page.addEventListener("pointerup", (e) => {
    if (sx === null) return;
    const dx = e.clientX - sx, dy = e.clientY - sy; sx = null;
    if (Math.abs(dx) > 50 && Math.abs(dx) > Math.abs(dy) * 1.5) step(dx < 0 ? 1 : -1);
  });
  page.addEventListener("pointercancel", () => { sx = null; });
  $("close").addEventListener("click", closeBook);
  scrim.addEventListener("click", closeBook);
  bar.querySelectorAll("[data-step]").forEach((b) => b.addEventListener("click", () => step(+b.dataset.step)));
  addEventListener("keydown", (e) => {
    if (!isOpen() || e.defaultPrevented || e.target.closest?.("input, select, textarea, .modal, .drawer")) return;
    if (e.key === "Escape") { e.preventDefault(); closeBook(); }
    if (e.key === "ArrowRight") step(1);
    if (e.key === "ArrowLeft") step(-1);
  });
  $("thumb").addEventListener("click", zoom);
  onResize.push(() => { if (isOpen() && !busy) { measure(); fill(current); carry.style.transform = `translateX(${size.shift}px)`; } });

  $("search-many").addEventListener("click", () => { reader.classList.add("searching"); $("source-links").querySelector("a")?.focus({ preventScroll: true }); });
  $("sources-x").addEventListener("click", () => { reader.classList.remove("searching"); $("search-many").focus({ preventScroll: true }); });
  $("copy").addEventListener("click", () => {
    const link = `${location.origin}/${current.address}`;
    navigator.clipboard?.writeText(link).then(() => hooks.toast("Link copied"), () => hooks.toast(link));
  });

  $("edit").addEventListener("click", startEdit);
  $("pcancel").addEventListener("click", () => reader.classList.remove("editing"));
  // Title and subtitle wrap and grow like the printed title; Enter saves instead of breaking the line.
  pform.querySelectorAll("textarea").forEach((t) => {
    t.addEventListener("input", () => fitText(t));
    t.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); pform.requestSubmit(); } });
  });
  pform.elements.identifier.addEventListener("input", showIssue);
  pform.addEventListener("submit", async (e) => {
    e.preventDefault();
    const { data, bad } = readForm();
    markBad(bad[0]?.[0], bad[0]?.[1]);
    if (bad.length) return pform.elements[bad[0][0]].focus();
    try {
      await hooks.save(current, data);
    } catch (err) {
      markBad(/ISBN|ISSN/.test(err.message) ? "identifier" : /issue/i.test(err.message) ? "issue" : null, err.message);
      return;
    }
    reader.classList.remove("editing");
    refill(current, true);
    hooks.toast("Saved");
  });
  $("reading").addEventListener("click", () => current.reading ? hooks.finishReading(current) : hooks.startReading(current));
  $("reading2").addEventListener("click", () => current.reading ? hooks.setAside(current) : hooks.markRead(current));
  $("notes-btn").addEventListener("click", () => hooks.openNotes(current));
  $("hide").addEventListener("click", () => hooks.setHidden(current, !current.hidden));
  $("remove").addEventListener("click", () => { reader.classList.add("asking"); $("remove-no").focus({ preventScroll: true }); });
  $("remove-no").addEventListener("click", () => reader.classList.remove("asking"));
  $("remove-yes").addEventListener("click", async () => { const b = current; reader.classList.remove("asking"); await closeBook(); hooks.remove(b); });

  // A new cover recolors the spine at once; the title page stays in edit mode.
  const takeCover = async (input) => {
    const file = input.files[0]; input.value = "";
    if (!file) return;
    try { await hooks.replaceCover(current, file); } catch (err) { hooks.toast(err.message); return; }
    recover(current);
    $("spine-color").value = tone(current).c;
    hooks.toast("New cover in place. The spine takes its color.");
  };
  $("cover-file").addEventListener("change", (e) => takeCover(e.target));
  $("cover-file-2").addEventListener("change", (e) => takeCover(e.target));
  const spine = $("spine-color");
  spine.addEventListener("input", (e) => paintSpine(e.target.value));
  spine.addEventListener("change", async (e) => { try { await hooks.setSpine(current, e.target.value); } catch (err) { hooks.toast(err.message); } });
  $("spine-auto").addEventListener("click", async () => {
    try { await hooks.setSpine(current, ""); } catch (err) { hooks.toast(err.message); return; }
    paintSpine(tone(current).c); spine.value = tone(current).c;
    hooks.toast("The spine follows the cover again");
  });
}
