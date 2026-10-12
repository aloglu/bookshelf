<script>
  // The page: the header, the shelves (the engine's), the stats page, and the owner's cards around them.
  import { onMount } from "svelte";
  import { ui, data, load, reload, setSelecting, toggleChosen, saveBook, replaceCover, setSpine, setHidden, removeBooks, startReading, finishReading, setAside } from "./lib/app.svelte.js";
  import { mountShelves, relayout, slots, view } from "./engine/shelf.js";
  import { mountReader, openBook, hooks, isOpen, dismissInBook, zoomed, putZoomBack } from "./engine/reader.js";
  import { cloth } from "./engine/look.js";
  import { motionOK, root } from "./engine/model.js";
  import { typingRules, openPanel, hidePanel } from "./lib/panels.js";
  import { toast } from "./lib/toast.js";
  import Header from "./ui/Header.svelte";
  import Stats from "./ui/Stats.svelte";
  import Selbar from "./ui/Selbar.svelte";
  import PhoneMenu from "./ui/PhoneMenu.svelte";
  import SignIn from "./ui/SignIn.svelte";
  import Readings from "./ui/Readings.svelte";
  import Marker from "./ui/Marker.svelte";
  import Adder from "./ui/Adder.svelte";
  import Settings from "./ui/Settings.svelte";

  let shelvesEl, readerEl, header, signin, readings, marker, adder, settingsDrawer, phoneMenu;

  $effect(() => { root.toggleAttribute("data-owner", ui.owner); });
  $effect(() => { root.toggleAttribute("data-gated", ui.gated); });
  $effect(() => { root.toggleAttribute("data-nostats", !!data.library && !data.library.settings.stats); });
  $effect(() => { if (ui.view === "stats") root.dataset.view = "stats"; else delete root.dataset.view; });

  // Addresses: / is the shelf, /stats the stats page, /signin opens sign-in, and a book's own address
  // (in the owner's chosen style) opens that book.
  const statsOpen = () => ui.owner || data.library?.settings.stats;
  const here = () => (ui.view === "stats" ? "/stats" : "/");
  const go = (path) => { if (location.pathname !== path) history.replaceState(null, "", path); };
  function setView(v) {
    if (v === "stats" && !statsOpen()) v = "shelf";
    ui.view = v;
    go(here());
    if (v === "stats") scrollTo({ top: 0 });
  }
  function openShelved(b) {
    const el = slots.get(b?.key);
    if (!el) return;
    const r = el.getBoundingClientRect(), far = r.bottom < 0 || r.top > innerHeight;
    if (far) el.scrollIntoView({ block: "center", behavior: motionOK() ? "smooth" : "instant" });
    setTimeout(() => openBook(b, el), far && motionOK() ? 450 : 0);
  }
  function route() {
    const path = decodeURIComponent(location.pathname.slice(1));
    if (!path) return;
    if (path === "stats") return setView("stats");
    if (path === "signin") { go("/"); if (!ui.owner) signin.open(); return; }
    const bare = path.replace(/[\s-]/g, "");
    const b = data.books.find((x) => x.address === path || x.slug === path || (x.isbn && x.isbn === bare));
    if (b) openShelved(b); else go("/");
  }

  // Esc closes whatever is on top: the cover held up, a menu, a card, the stats page, an edit or question
  // on the open book, then choosing books.
  function dismissTop() {
    if (zoomed()) { putZoomBack(); return true; }
    if (header.closeMenu()) return true;
    const p = openPanel();
    if (p) { hidePanel(p); return true; }
    if (ui.view === "stats" && !isOpen()) { setView("shelf"); return true; }
    if (dismissInBook()) return true;
    if (!isOpen() && ui.selecting) { setSelecting(false); return true; }
    return false;
  }

  const openNote = (b, readingId) => readings.open(b, readingId);
  Object.assign(hooks, {
    save: saveBook, replaceCover, setSpine, toast,
    startReading, setAside, finishReading: (b) => finishReading(b, openNote),
    markRead: (b) => marker.open(b), openReadings: (b) => readings.open(b),
    setHidden: (b, hidden) => setHidden([b], hidden), remove: (b) => removeBooks([b]),
    opened: (b) => go("/" + b.address), closed: () => go(here()),
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
    route();
  });
  // The shelves follow the query as it is typed, a frame at a time.
  let qFrame = 0;
  function searched() { cancelAnimationFrame(qFrame); qFrame = requestAnimationFrame(() => relayout()); if (ui.view === "stats") setView("shelf"); }
</script>

<div class="wrap">
  <Header bind:this={header} {setView} {openShelved} onSearch={searched}
    onSignIn={() => signin.open()} onAdd={() => adder.open()} onSettings={() => settingsDrawer.open("site")} onMenu={() => phoneMenu.open()} />
  <section class="gate" hidden={!ui.gated}>
    <svg class="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>
    <h2>This library is private</h2>
    <button type="button" class="btn primary" onclick={() => signin.open()}>Sign in</button>
  </section>
  <Stats active={ui.view === "stats"} />
  <div class="tip" id="tip" role="tooltip" hidden></div>
  <main class="case" aria-label="Bookshelves"><div class="shelves" bind:this={shelvesEl}></div></main>
  <footer class="credits">Made by <a href="https://alog.lu" rel="noopener">alog.lu</a> · Code available on <a href="https://github.com/aloglu/bookshelf" rel="noopener">GitHub</a></footer>
</div>

<div class="reader" id="reader" role="dialog" aria-modal="true" aria-labelledby="d-title" hidden bind:this={readerEl}></div>

<Selbar />
<PhoneMenu bind:this={phoneMenu} {setView} onSignIn={() => signin.open()} onSettings={() => settingsDrawer.open("site")} />
<SignIn bind:this={signin} />
<Readings bind:this={readings} onCorrect={(b, r) => marker.open(b, r, () => readings.open(b))} />
<Marker bind:this={marker} />
<Adder bind:this={adder} {openShelved} />
<Settings bind:this={settingsDrawer} />
