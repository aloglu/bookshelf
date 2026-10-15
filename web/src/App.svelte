<script>
  // The page: the header, the shelves (the engine's), the stats page, a book's notes page, and the owner's
  // cards around them.
  import { onMount, tick } from "svelte";
  import { ui, data, load, reload, fetchEntriesEarly, setSelecting, toggleChosen, saveBook, replaceCover, setSpine, setHidden, removeBooks, startReading, finishReading, setAside } from "./lib/app.svelte.js";
  import { mountShelves, relayout, slots, view } from "./engine/shelf.js";
  import { mountReader, openBook, closeBook, closeOnto, showInHand, canLeave, hooks, isOpen, dismissInBook, zoomed, putZoomBack } from "./engine/reader.js";
  import { cloth } from "./engine/look.js";
  import { isPhone, motionOK, root } from "./engine/model.js";
  import { closePicker } from "./lib/colorpicker.js";
  import { typingRules, openPanel, hidePanel } from "./lib/panels.js";
  import { toast } from "./lib/toast.js";
  import { lines } from "./lib/markdown.js";
  import Header from "./ui/Header.svelte";
  import Stats from "./ui/Stats.svelte";
  import Selbar from "./ui/Selbar.svelte";
  import PhoneMenu from "./ui/PhoneMenu.svelte";
  import SignIn from "./ui/SignIn.svelte";
  import Notes from "./ui/Notes.svelte";
  import AllNotes from "./ui/AllNotes.svelte";
  import Export from "./ui/Export.svelte";
  import Marker from "./ui/Marker.svelte";
  import Adder from "./ui/Adder.svelte";
  import Settings from "./ui/Settings.svelte";

  let shelvesEl, readerEl, header, notesPage, marker, adder, settingsPage, phoneMenu, exporter;

  $effect(() => { root.toggleAttribute("data-owner", ui.owner); });
  $effect(() => { root.toggleAttribute("data-gated", ui.gated); });
  $effect(() => { root.toggleAttribute("data-nostats", !!data.library && !data.library.settings.stats); });
  $effect(() => { if (ui.view !== "shelf") root.dataset.view = ui.view; else delete root.dataset.view; });

  // Addresses: / is the shelf, /stats the stats page, /notes all the notes, /signin the sign-in page,
  // /settings/<section> a section of the owner's settings (/settings alone is the list of them on a phone), a
  // book's own address (in the owner's chosen style) opens that book, that address with /notes after it shows
  // its notes page, and /notes/<id> after that one note on a page of its own. A private library shows visitors
  // the sign-in page whatever the address.
  const statsOpen = () => ui.owner || data.library?.settings.stats;
  // All notes is always the owner's; visitors get it while it is turned on in Settings.
  const allNotesOpen = () => ui.owner || (!!data.library?.settings.notes && !!data.library?.settings.timeline);
  // A book's notes page is always the owner's; visitors see it once something on it is public.
  const notesOpen = (b) => !!b && (ui.owner || b.entries > 0);
  const notesBook = () => data.books.find((b) => b.key === ui.notesFor);
  const SECTIONS = ["shelf", "site", "visitors", "library", "about"];
  const PATHS = { stats: "/stats", allnotes: "/notes", signin: "/signin" };
  const here = () => (ui.view === "notes" ? `/${notesBook().address}/notes${ui.noteId ? "/" + ui.noteId : ""}` : ui.view === "settings" ? "/settings" + (ui.section ? "/" + ui.section : "") : PATHS[ui.view] || "/");
  // What an address shows: a page ({ v }), or a book taken out ({ b }) over the page its entry remembers.
  function place(path) {
    const p = decodeURIComponent(path.slice(1));
    if (!p) return { v: "shelf" };
    if (p === "stats") return { v: "stats" };
    if (p === "notes") return { v: "allnotes" };
    if (p === "signin") return { v: "signin" };
    if (p === "settings") return { v: "settings", section: "" };
    if (p.startsWith("settings/") && SECTIONS.includes(p.slice(9))) return { v: "settings", section: p.slice(9) };
    const [, name, notes, note = ""] = p.match(/^(.+?)(\/notes(?:\/([^/]+))?)?$/);
    const bare = name.replace(/[\s-]/g, "");
    const b = data.books.find((x) => x.address === name || x.slug === name || (x.isbn && x.isbn === bare));
    if (!b) return { v: "shelf", lost: true };
    return notes ? { v: "notes", b, note } : { b };
  }

  /* ---------- History ---------- */
  // Every place is a step in the browser's history, so Back and Forward work as on any site: each page, each
  // Settings section, each book taken out, and each book stepped to with Previous and Next. Cards, sorting,
  // searching as you type and choosing books are not places. An entry remembers how far down its page was
  // (y); a book's entry also the page it was taken out over (under) and how many books in a row lead to it
  // (run); depth counts the steps since the visit began, so putting a book back can go back past its run
  // without leaving Bookshelf.
  history.scrollRestoration = "manual";
  const entry = () => history.state || { depth: 0 };
  let shownDepth = 0;
  function push(path, extra = {}) {
    if (location.pathname === path) return;
    history.replaceState({ ...entry(), y: scrollY }, "");
    history.pushState({ depth: entry().depth + 1, from: location.pathname, ...extra }, "", path);
    shownDepth = entry().depth;
  }
  function replace(path, extra = {}) {
    const { depth, from } = entry();
    history.replaceState({ depth, from, ...extra }, "", path);
  }
  // how: "push" (a new step), "replace" (this step shows something else) or "none" (Back or Forward got here).
  function setView(v, b = null, { section = "", note = "", how = "push" } = {}) {
    const asked = v;
    if (v === "stats" && !statsOpen()) v = "shelf";
    if (v === "notes" && !notesOpen(b)) v = "shelf";
    if (v === "allnotes" && !allNotesOpen()) v = "shelf";
    if (v === "signin" && ui.owner) v = "shelf";
    if (v === "settings" && !ui.owner) v = "shelf";
    if (v !== asked && how === "none") how = "replace";   // nothing for this reader there: the step shows the shelf
    // A wide screen shows a section beside the list, so its address names the section shown.
    if (v === "settings" && !section && !isPhone()) section = "shelf";
    ui.view = v;
    ui.notesFor = v === "notes" ? b.key : null;
    ui.noteId = v === "notes" ? note : "";
    ui.section = v === "settings" ? section : "";
    if (how === "push") push(here()); else if (how === "replace") replace(here());
    if (v !== "shelf" && how !== "none") scrollTo({ top: 0 });
  }
  // Signing out, or the last public note going private, leaves this reader nothing to see there.
  $effect(() => { if (ui.view === "notes" && !notesOpen(notesBook())) setView("shelf", null, { how: "replace" }); });
  $effect(() => { if (ui.view === "allnotes" && !allNotesOpen()) setView("shelf", null, { how: "replace" }); });
  $effect(() => { if (ui.view === "settings" && ui.ready && !ui.owner) setView("shelf", null, { how: "replace" }); });

  // The open book: taking one out is a step, and so is each book stepped to; putting it back (Put back, Esc,
  // a click beside it) goes back past them all to the page it was taken out over.
  let leaving = null;
  function putBack() {
    const { run, depth, under } = entry();
    if (!depth) return replace(under);   // the visit began on this book: nothing to go back to
    leaving = under;
    history.go(-Math.min(run, depth));   // past the start of the visit if it began on one of these books
    // A browser keeps only so many steps; if the run went further back than that, the address is set here.
    setTimeout(() => { if (leaving && place(location.pathname).b) { replace(leaving); leaving = null; } }, 400);
  }
  Object.assign(hooks, {
    opened: (b) => push("/" + b.address, { under: location.pathname, run: 1 }),
    stepped: (b) => push("/" + b.address, { under: entry().under, run: (entry().run || 0) + 1 }),
    closed: () => { if (entry().run) putBack(); },   // put back by the reader, not by Back
  });

  // Back or Forward: the screen follows the address. A card, a menu or the colour panel closes with the page it
  // was over. A book's entry opens that book over its page, or swaps it into hand; any other entry puts the
  // open book back and shows its page where it was left.
  let undoing = false;
  async function popped() {
    if (undoing) { undoing = false; return; }
    const st = entry(), dir = Math.sign((st.depth ?? 0) - shownDepth);
    if (dir && isOpen() && !canLeave()) { undoing = true; history.go(-dir); return; }   // an edit under way stays put
    shownDepth = st.depth ?? 0;
    if (zoomed()) putZoomBack();
    header.closeMenu(); closePicker();
    const p = openPanel(); if (p) hidePanel(p);
    let at = place(location.pathname);
    if (leaving) {
      if (at.b && !at.v) { replace(leaving); at = place(leaving); }
      leaving = null;
    }
    if (at.b && !at.v) return takeOut(at.b, st, dir);
    if (isOpen()) await closeBook();
    setView(at.v, at.b, { section: at.section, note: at.note, how: at.lost ? "replace" : "none" });
    await restore(st.y);
  }
  // The page as it was left: the shelf laid out again (it was out of sight) and scrolled where it stood.
  async function restore(y) {
    await tick();
    if (ui.view !== "shelf") return scrollTo({ top: ui.view === "settings" || y == null ? 0 : y, behavior: "instant" });
    relayout(false);
    if (y > innerHeight) await layOutShelves();
    scrollTo({ top: y || 0, behavior: "instant" });
  }
  async function takeOut(b, st, dir) {
    if (isOpen()) return showInHand(b, dir);
    const under = place(st.under || "/");
    setView(under.v || "shelf", under.b, { section: under.section, note: under.note, how: "none" });
    await restore(st.y);
    if (ui.view === "shelf") return openShelved(b);
    openBook(b, ui.view === "notes" ? document.querySelector(".nb-head .cv-btn img") : null);
  }

  // Signing in goes back to the page it was asked for from. A private library showed its sign-in page in place
  // of the address asked for, which is shown now.
  let routed = false;
  function signedIn() {
    if (!routed) return route();
    if (entry().depth > 0) history.back(); else setView("shelf", null, { how: "replace" });
  }
  // A Settings section; on a phone, "‹ Settings" goes back to the list when that is where it came from.
  function toSection(k) {
    if (!k && entry().from === "/settings") history.back();
    else setView("settings", null, { section: k });
  }
  // From the open book, the notes page comes up beneath it and the book closes onto its cover there.
  // With a reading, a note is begun for it ("Add a note" after finishing).
  async function openNotes(b, readingId) {
    if (isOpen()) {
      await fetchEntriesEarly(b);
      const ok = await closeOnto(async () => { setView("notes", b); await tick(); return document.querySelector(".nb-head .cv-btn img"); });
      if (!ok) return;
    } else setView("notes", b);
    if (readingId && ui.view === "notes") { await tick(); notesPage.begin(readingId); }
  }
  // A book out of sight is scrolled to first; it leaves the shelf only once it has come to rest there (a flight
  // that starts while the page still moves takes off from the wrong place and looks rushed). Rest means the book
  // is on screen and has stopped moving; scrollend isn't relied on, as Safari may not send it.
  async function openShelved(b) {
    const el = slots.get(b?.key);
    if (!el) return;
    const r = el.getBoundingClientRect(), far = r.bottom < 0 || r.top > innerHeight;
    if (far) await layOutShelves();
    if (far && motionOK()) {
      el.scrollIntoView({ block: "center", behavior: "smooth" });
      await new Promise((done) => {
        const until = performance.now() + 3000;
        let last = NaN, still = 0, again = true;
        const look = () => {
          const r = el.getBoundingClientRect(), seen = r.bottom > 0 && r.top < innerHeight;
          still = Math.abs(r.top - last) < 0.5 ? still + 1 : 0; last = r.top;
          // Stopped short of it all the same: one more scroll, the rest of the way.
          if (still >= 4 && !seen && again) { again = false; still = 0; el.scrollIntoView({ block: "center", behavior: "smooth" }); }
          if ((still >= 4 && seen) || performance.now() > until) done(); else setTimeout(look, 40);
        };
        setTimeout(look, 40);
      });
    } else if (far) el.scrollIntoView({ block: "center", behavior: "instant" });
    openBook(b, el);
  }
  // Shelves out of view aren't laid out; each stands in at a guessed height until it has been drawn once (then
  // it keeps its real one). On a first visit a scroll to a far book would aim with those guesses and stop in
  // the wrong place, so every shelf is drawn first, for a frame (shelves are rebuilt as the library changes, so
  // this is done before every such scroll).
  async function layOutShelves() {
    root.classList.add("all-shelves");
    await new Promise((done) => { requestAnimationFrame(() => requestAnimationFrame(done)); setTimeout(done, 200); });
    root.classList.remove("all-shelves");
  }
  // Where the visit begins: the address shows what it names, in place of the shelf (no step of its own). An
  // address that names nothing becomes the shelf's. A reload keeps the entry's place in the history.
  function route() {
    routed = true;
    shownDepth = entry().depth;
    const at = place(location.pathname);
    if (at.v) return setView(at.v, at.b, { section: at.section, note: at.note, how: location.pathname === "/" ? "none" : "replace" });
    if (!entry().run) replace(location.pathname, { under: "/", run: 1 });
    takeOut(at.b, entry(), 0);
  }

  // Esc closes whatever is on top: the cover held up, a menu, a card, the stats or notes page, an edit or
  // question on the open book, then choosing books.
  function dismissTop() {
    if (zoomed()) { putZoomBack(); return true; }
    if (header.closeMenu()) return true;
    const p = openPanel();
    if (p) { hidePanel(p); return true; }
    if (ui.view === "settings" && settingsPage.dismiss()) return true;
    if (ui.view !== "shelf" && !isOpen()) { setView("shelf"); return true; }
    if (dismissInBook()) return true;
    if (!isOpen() && ui.selecting) { setSelecting(false); return true; }
    return false;
  }

  Object.assign(hooks, {
    save: saveBook, replaceCover, setSpine, toast,
    startReading, setAside, finishReading: (b) => finishReading(b, openNotes),
    markRead: (b) => marker.open(b), openNotes: (b) => openNotes(b),
    setHidden: (b, hidden) => setHidden([b], hidden), remove: (b) => removeBooks([b]),
  });
  view.onOpen = (b, el) => openBook(b, el);
  view.onChoose = toggleChosen;

  onMount(async () => {
    cloth();
    typingRules();
    addEventListener("keydown", (e) => { if (e.key === "Escape" && dismissTop()) e.preventDefault(); });
    mountShelves(shelvesEl);
    mountReader(readerEl);
    try {
      await load();
    } catch (err) {
      toast(err.message);
      return;
    }
    relayout(false);
    addEventListener("popstate", popped);
    if (!ui.gated) route();
  });
  // The shelves follow the query as it is typed, a frame at a time.
  let qFrame = 0;
  function searched() { cancelAnimationFrame(qFrame); qFrame = requestAnimationFrame(() => relayout()); if (ui.view !== "shelf") setView("shelf"); }
</script>

<div class="wrap">
  <Header bind:this={header} {setView} {openShelved} onSearch={searched}
    onAdd={() => adder.open()} onMenu={() => phoneMenu.open()} />
  <SignIn active={ui.gated || ui.view === "signin"} onDone={signedIn} />
  <Settings bind:this={settingsPage} active={ui.view === "settings" && ui.owner} section={ui.section} onSection={toSection} onExport={(b) => exporter.open(b)} />
  <Stats active={ui.view === "stats"} />
  <Notes bind:this={notesPage} active={ui.view === "notes"} onCorrect={(b, r) => marker.open(b, r)} onExport={(b) => exporter.open(b)}
    onNote={(b, id) => setView("notes", b, { note: id })} onAll={(b, how) => setView("notes", b, { how })} />
  <AllNotes active={ui.view === "allnotes"} onBook={(b) => setView("notes", b)} onNote={(b, id) => setView("notes", b, { note: id })} onExport={() => exporter.open(null)} />
  <div class="tip" id="tip" role="tooltip" hidden></div>
  <main class="case" aria-label="Bookshelves"><div class="shelves" bind:this={shelvesEl}></div></main>
  {#if data.library?.settings.footer && data.library.settings.footerText}<footer class="credits">{@html lines(data.library.settings.footerText)}</footer>{/if}
</div>

<div class="reader" id="reader" role="dialog" aria-modal="true" aria-labelledby="d-title" hidden bind:this={readerEl}></div>

<Selbar />
<PhoneMenu bind:this={phoneMenu} {setView} />
<Marker bind:this={marker} onNote={openNotes} />
<Adder bind:this={adder} {openShelved} />
<Export bind:this={exporter} />
