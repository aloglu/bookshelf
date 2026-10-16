<script>
  // Settings: the owner's own page (/settings), with its sections down the side (on a phone, the list of
  // sections and then one at a time). Every change shows at once and saves itself; dragging a color or a slider saves when it comes to
  // rest. The shelf section has a preview of the shelf, by day or by evening, drawn by the shelf's own rules.
  import { untrack } from "svelte";
  import { ui, data, patchSettings, reload, signOut, showSort, checkUpdates, setIcon, iconURL } from "../lib/app.svelte.js";
  import { SORT_WAYS, fold, isPhone, lastName, reheight, sortBooks, spineHeight } from "../engine/model.js";
  import { api } from "../lib/api.js";
  import { toast, savedToast } from "../lib/toast.js";
  import { pill } from "../lib/pill.js";
  import { pickColor } from "../lib/colorpicker.js";
  import { applyLook, WOODS } from "../engine/look.js";
  import { relayout, view } from "../engine/shelf.js";
  import { setNotesHidden, previewClippings, importClippings } from "../lib/app.svelte.js";
  import { plural } from "../engine/model.js";
  import { ICON } from "../lib/notes.js";

  let { active, section, onSection, onExport } = $props();

  // Each section: its name, and what is in it (under its name in a phone's list).
  const SECTIONS = [
    ["shelf", "Shelf", "Paint, wood and ribbon, the bookcase, the books"],
    ["site", "Header and footer", "The name, its icon, your links, the footer"],
    ["visitors", "Visitors", "Access, notes, how they arrive, a book’s page"],
    ["library", "Your library", "Import, export, notes, backups"],
    ["about", "About", "Your account, the version, Bookshelf"],
  ];
  const NETS = {
    goodreads: { name: "Goodreads", host: /(^|\.)goodreads\.com$/, url: (v) => /^\d+(-[\w-]+)?$/.test(v) ? `https://www.goodreads.com/user/show/${v}` : `https://www.goodreads.com/${v}` },
    storygraph: { name: "The StoryGraph", host: /(^|\.)thestorygraph\.com$/, url: (v) => `https://app.thestorygraph.com/profile/${v}` },
    instagram: { name: "Instagram", host: /(^|\.)instagram\.com$/, url: (v) => `https://www.instagram.com/${v}/` },
  };
  const RIBBONS = [["#8e2532", "Crimson"], ["#23365e", "Navy"], ["#2e5a3c", "Forest"], ["#b8902f", "Old gold"], ["#1d1a19", "Black"]];
  // Wall paints, each with its day and its evening color.
  const PAINTS = [["Linen", "#e9e0d0", "#17110d"], ["Tobacco", "#cdb08a", "#2a1c12"], ["Chestnut", "#94633f", "#3a2416"], ["Umber", "#5b4130", "#1e1510"], ["Forest green", "#3d4c3e", "#152018"]];
  const PARTS = [["shelves", "Shelves"], ["sides", "Sides"], ["top", "Top"], ["back", "Back panel"]];
  const ADDRESS = [["isbn", "ISBN"], ["hyphens", "Hyphenated"], ["title", "Title"]];
  const SOURCES = [["openlibrary", "Open Library"], ["google", "Google Books"], ["goodreads", "Goodreads"]];

  // The text fields here carry the markers password managers honour (1Password, LastPass, Bitwarden,
  // Dashlane) so they aren't taken for a login: "Username" in a link's hint reads like one.
  let st = $state(null), mode = $state("day");
  let backups = $state([]), importNote = $state(""), over = $state(false), restoring = $state(null);
  let social = $state({}), badSocial = $state({});

  // The section shown is the address's (/settings/<section>); on a phone /settings alone lists them, and on a
  // wide screen the list stands beside every section.
  let phone = $state(isPhone());
  $effect(() => { const m = matchMedia("(max-width: 720px)"), f = () => (phone = m.matches); m.addEventListener("change", f); return () => m.removeEventListener("change", f); });
  const shown = $derived(section || "shelf");
  const listing = $derived(phone && !section);
  $effect(() => { if (active) untrack(open); });
  $effect(() => { section; untrack(settle); });
  function open() {
    if (!data.library) return;
    st = structuredClone($state.snapshot(data.library.settings));
    social = Object.fromEntries(Object.keys(NETS).map((k) => [k, (st.social[k] || "").replace(/^https:\/\/(www\.|app\.)?/, "")]));
    badSocial = {}; importNote = ""; restoring = null;
    settle();
    mode = document.documentElement.dataset.theme === "dark" || (!document.documentElement.dataset.theme && matchMedia("(prefers-color-scheme: dark)").matches) ? "night" : "day";
    loadBackups();
    checkUpdates();
  }
  // The sections are links (they can be opened in a new tab); a plain click shows one here.
  function follow(e, k) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button) return;
    e.preventDefault();
    onSection(k);
  }
  // Esc first drops a question or a file waiting to be confirmed; then it leaves the page.
  export function dismiss() {
    if (!restoring && !upload && !clips) return false;
    restoring = null; upload = null; clips = null;
    return true;
  }
  // While a section is first drawn nothing animates (its switches and segments arrive as they are); only a
  // change the owner makes slides.
  let still = $state(false);
  function settle() { still = true; requestAnimationFrame(() => requestAnimationFrame(() => (still = false))); }
  async function loadBackups() { try { backups = await api("GET", "/api/backups"); } catch { backups = []; } }

  // A change shows at once (in the preview, the walls, the wood, and on the shelf for when it is back in sight)
  // and is saved: at once for a click, or once the owner stops typing or dragging.
  let timer = 0, pending = {};
  function change(patch, { soon = false, note } = {}) {
    Object.assign(st, patch);
    Object.assign(pending, patch);
    const local = $state.snapshot(st);
    applyLook(local); view.settings = local;
    if ("bindings" in patch || "maxHeight" in patch) reheight(data.books, local);
    if ("bindings" in patch || "maxHeight" in patch || "bookSize" in patch || "caseParts" in patch) relayout(false);
    clearTimeout(timer);
    timer = setTimeout(() => commit(note), soon ? 450 : 0);
  }
  async function commit(note) {
    const patch = pending; pending = {};
    try {
      await patchSettings(patch);
      savedToast(note);
    } catch (err) {
      toast(err.message);
      st = structuredClone($state.snapshot(data.library.settings));
      await reload();
    }
  }
  function typedSocial(net) {
    const v = (social[net] || "").trim();
    let url = "";
    if (v && /^(https?:\/\/)?[\w.-]+\.[a-z]{2,}\//i.test(v)) {
      try { const u = new URL(/^https?:/i.test(v) ? v : "https://" + v); url = NETS[net].host.test(u.hostname) ? "https://" + u.hostname + u.pathname.replace(/\/+$/, "") : null; } catch { url = null; }
    } else if (v) {
      const handle = v.replace(/^@/, "");
      url = /^[\w.-]{1,40}$/.test(handle) ? NETS[net].url(handle) : null;
    }
    badSocial[net] = url === null;
    if (url !== null) change({ social: { ...st.social, [net]: url } }, { soon: true });
  }

  /* ---------- The shelf ---------- */
  // What's being chosen: the day's paint, wood and ribbon, or the evening's.
  const wallKey = $derived(mode === "day" ? "wallDay" : "wallNight");
  const woodKey = $derived(mode === "day" ? "woodDay" : "woodNight");
  const ribbonKey = $derived(mode === "day" ? "ribbonDay" : "ribbonNight");
  const paint = (wall) => change({ [wallKey]: wall }, { soon: true });
  // The preview: the library's own books, the first of the shelf, as many as fill most of its width, one of
  // them being read if any is. They are drawn at the wide screen's shelf scale on every screen (where the
  // lettering fits the spines as it should) and the whole shelf is scaled down, like a picture, to a box of fixed height, sized
  // for the tallest books the settings allow; so a setting never resizes the box (see .shelfpv).
  let pvWidth = $state(0);
  const pvZoom = () => parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--pvz")) || .5;
  const pvBooks = $derived.by(() => {
    if (!st) return [];
    const k = 1.15 * st.bookSize / 100, room = (pvWidth / pvZoom()) * .82;
    const shown = sortBooks(data.books.filter((b) => !b.hidden), st.sort, st.sortDesc);
    const list = [];
    let used = 0;
    for (const b of shown) { const w = Math.round(b.t * k) + 2; if (used + w > room) break; list.push(b); used += w; }
    // One book wears the ribbon: one being read if there is one, otherwise one in the middle as a sample, so
    // the ribbon's color can be seen.
    const reader = shown.find((b) => b.reading), mid = Math.floor(list.length / 2);
    if (reader && !list.includes(reader) && list.length) list[mid] = reader;
    const ribboned = list.includes(reader) ? reader : list[mid];
    // Heights by the shelf's own rule from the settings being chosen (the books' own heights change in place,
    // which the preview wouldn't notice), so binding details and spine height show here at once.
    const limit = st.maxHeight / 100;
    return list.map((b) => ({ b, ribbon: b === ribboned, t: Math.round(b.t * k), h: Math.round(spineHeight(b, st.bindings, limit) * k) }));
  });
  const rowH = $derived(Math.max(0, ...pvBooks.map((p) => p.h)) + 4);
  function picker(e, value, set) { pickColor(e.currentTarget, value, { input: set }); }

  /* ---------- Visitors ---------- */
  const eg = $derived(data.books.find((b) => b.isbn && !b.hidden) || data.books[0]);
  const addressOf = (k) => "/" + ((k === "isbn" ? eg?.isbn || eg?.slug : k === "hyphens" ? (eg?.isbn ? eg.identifier : eg?.slug) : eg?.slug) || "…");
  const sortWay = $derived(st ? SORT_WAYS[st.sort][st.sortDesc ? 1 : 0] : "");

  /* ---------- Notes ---------- */
  // Visitors' notes are exceptions, not a list: only the books whose notes are hidden are listed, and a finder
  // (type a title or author) picks one to hide, or one to export. Built for hundreds of books.
  const words = (t) => fold(t).replace(/[^\p{L}\p{N}]+/gu, " ").trim();
  let hideQuery = $state(""), exportQuery = $state("");
  const noted = $derived(data.books.filter((b) => b.entries > 0));
  const hiddenNotes = $derived(data.books.filter((b) => b.notesHidden));
  const reach = $derived(st?.notes ? data.books.filter((b) => !b.hidden && !b.notesHidden && b.notesPublic) : []);
  function found(q, pool) {
    const n = words(q);
    const hits = n ? pool.filter((b) => words(`${b.title} ${b.author}`).includes(n)) : [...pool].sort((a, b) => Date.parse(b.notesLast || 0) - Date.parse(a.notesLast || 0));
    return { n, hits, shown: hits.slice(0, n ? 5 : 4) };
  }
  async function hideNotes(b, hidden) {
    try { await setNotesHidden(b, hidden); } catch (err) { toast(err.message); return; }
    if (hidden) { hideQuery = ""; toast(`${b.title} is hidden from visitors`, { label: "Undo", run: () => setNotesHidden(b, false).catch((e) => toast(e.message)) }); }
    else toast("Visitors can read its public notes again");
  }
  // Kindle: what the file holds and what it would add, before anything is written.
  let clips = $state(null), clipsPublic = $state(false), clipsBusy = $state(false), clipsOver = $state(false);
  async function readClippings(file) {
    if (!file) return;
    clipsBusy = true;
    try { clips = { file, name: file.name, plan: await previewClippings(file) }; clipsPublic = false; } catch (err) { clips = null; toast(err.message); }
    clipsBusy = false;
  }
  async function runClippings() {
    clipsBusy = true;
    try {
      const r = await importClippings(clips.file, clipsPublic);
      toast(`${plural(r.added, "note")} added to ${plural(r.books, "book")}`);
      clips = null;
    } catch (err) { toast(err.message); }
    clipsBusy = false;
  }

  /* ---------- Your library ---------- */
  async function importFile(file) {
    if (!file) return;
    importNote = `Bringing in ${file.name}…`;
    const body = new FormData(); body.append("file", file);
    try {
      const r = await api("POST", `/api/import?name=${encodeURIComponent(file.name)}`, body);
      importNote = `${file.name}: ${r.added.toLocaleString("en-US")} book${r.added === 1 ? "" : "s"} added${r.skipped ? `, ${r.skipped.toLocaleString("en-US")} already on your shelf` : ""}. A backup was made first.`;
      await reload({ animate: true });
      loadBackups();
    } catch (err) { importNote = `${file.name}: ${err.message}`; }
  }
  const KINDS = { automatic: "Automatic", manual: "Made by hand", "before-import": "Before an import", "before-restore": "Before a restore" };
  const stamp = (t) => new Date(t).toLocaleString("en-US", { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
  async function backupNow() { try { await api("POST", "/api/backups"); toast("Backed up"); loadBackups(); } catch (err) { toast(err.message); } }
  async function restore(x) {
    try {
      await api("POST", `/api/backups/${encodeURIComponent(x.name)}/restore`);
      restoring = null;
      await reload({ animate: true });
      st = structuredClone($state.snapshot(data.library.settings));
      toast(`Library restored to ${stamp(x.createdAt)}`);
      loadBackups();
    } catch (err) { toast(err.message); }
  }
  // Restoring from a file: a .bookshelf archive (a backup kept elsewhere, or another library's export)
  // replaces everything here, after a backup of what is here now.
  let upload = $state(null), uploading = $state(false);
  async function restoreFile() {
    const file = upload;
    uploading = true;
    const body = new FormData(); body.append("file", file);
    try {
      await api("POST", "/api/restore", body);
      upload = null;
      await reload({ animate: true });
      st = structuredClone($state.snapshot(data.library.settings));
      toast(`Library restored from ${file.name}`);
      loadBackups();
    } catch (err) { toast(`${file.name}: ${err.message}`); }
    uploading = false;
  }

  /* ---------- Bookshelf ---------- */
  // Turning the check on asks GitHub at once; the answer comes a moment later.
  function toggleUpdates(on) { change({ checkUpdates: on }); if (on) setTimeout(checkUpdates, 2500); }
  const UPDATING = "https://github.com/aloglu/bookshelf/blob/main/docs/updating.md";
  const RELEASES = "https://github.com/aloglu/bookshelf/releases";
  const checked = (t) => new Date(t).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
  // Any picture becomes the icon, cropped to a square at its middle.
  async function pickIcon(file) {
    try {
      const next = await setIcon(file);
      st.icon = next.icon;
      savedToast(file ? "Saved. The new icon is in place." : "Saved. Bookshelf’s icon is back.");
    } catch (err) { toast(err.message); }
  }
  async function out() { try { await signOut(); } catch (err) { toast(err.message); } }
  const ICONS = {
    shelf: `<path d="M3 20h18M5 20V6h3v14M10 20V4h3v16M15.5 20l-1-12 3-.4 1.4 12.4"/>`,
    site: `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M7 6.5h4"/>`,
    visitors: `<circle cx="12" cy="8" r="3.5"/><path d="M5 20c.8-3.6 3.6-5.5 7-5.5s6.2 1.9 7 5.5"/>`,
    library: `<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5M5 19h14"/>`,
    about: `<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5M12 8h.01"/>`,
  };
</script>

{#snippet sw(checked, onchange, label)}<input type="checkbox" class="switch" aria-label={label} {checked} onchange={(e) => onchange(e.target.checked)}>{/snippet}
{#snippet finder(kind, q, set)}
  {@const pool = kind === "hide" ? noted.filter((b) => !b.notesHidden) : noted}
  {@const f = found(q, pool)}
  {@const list = !f.n && kind === "hide" ? [] : f.shown}
  <div class="finder">
    <label class="fsearch"><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.search}</svg><input type="search" class="inp" placeholder="Find a book by title or author" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" aria-label="Find a book" value={q} oninput={(e) => set(e.target.value)}></label>
    {#if list.length}
      <p class="fcap">{f.n ? plural(f.hits.length, "match").replace("matchs", "matches") : "Most recent notes"}</p>
      <ul class="flist">
        {#each list as b (b.key)}
          <li><img src={b.face} alt=""><span><b>{b.title}</b><small>{b.author} · {plural(b.entries, "note")}{b.notesPublic ? `, ${b.notesPublic} public` : ""}</small></span>
            {#if kind === "hide"}<button type="button" class="lnk" onclick={() => hideNotes(b, true)}>Hide</button>{:else}<button type="button" class="lnk" onclick={() => onExport(b)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.down}</svg>Markdown</button>{/if}
          </li>
        {/each}
      </ul>
      {#if f.hits.length > list.length}<p class="fcap">{f.hits.length - list.length} more. Keep typing to narrow it.</p>{/if}
    {:else}
      <p class="fcap">{f.n ? `No book with notes matches “${q}”.` : kind === "hide" ? "Type a title or author to hide a book." : "No notes to export yet."}</p>
    {/if}
  </div>
{/snippet}
{#snippet name(text, hint)}<div class="sname">{text}{#if hint}<small>{hint}</small>{/if}</div>{/snippet}

<section class="settings" id="settings" class:still hidden={!active} aria-label="Settings">
  {#if !(phone && section)}
    <div class="set-head"><h2>Settings</h2><p>Every change shows at once and saves itself.</p></div>
  {/if}
  {#if listing}
    <nav class="set-list" aria-label="Settings sections">
      {#each SECTIONS as [k, title, sum]}
        <a href="/settings/{k}" class:dot={k === "about" && ui.update?.newer} onclick={(e) => follow(e, k)}>
          <svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICONS[k]}</svg><span><b>{title}</b><small>{sum}</small></span>
          <svg class="i chev" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>
        </a>
      {/each}
      {#if ui.update?.version}<p class="ver">Bookshelf {ui.update.version}</p>{/if}
    </nav>
  {:else if st}
  {@const [, title] = SECTIONS.find(([k]) => k === shown)}
  <div class="set-grid">
    <nav class="set-rail" aria-label="Settings sections">
      {#each SECTIONS as [k, t]}
        <a href="/settings/{k}" aria-current={k === shown ? "page" : undefined} class:dot={k === "about" && ui.update?.newer} onclick={(e) => follow(e, k)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICONS[k]}</svg>{t}</a>
      {/each}
      {#if ui.update?.version}<p class="ver">Bookshelf {ui.update.version}</p>{/if}
    </nav>
    <div class="set-pane">
    {#if phone}
      <a class="set-back" href="/settings" onclick={(e) => follow(e, "")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 6-6 6 6 6"/></svg>Settings</a>
      <h2 class="set-title">{title}</h2>
    {/if}
    <div class="sbody">
    {#if shown === "shelf"}
      <div class="pvbox">
        <div class="seg modeswitch" role="group" aria-label="Choose the paint, wood and ribbon for" use:pill>
          <button type="button" aria-pressed={mode === "day"} onclick={() => (mode = "day")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4"/></svg>Day</button>
          <button type="button" aria-pressed={mode === "night"} onclick={() => (mode = "night")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z"/></svg>Evening</button>
        </div>
        <div class="shelfpv" class:night={mode === "night"} aria-hidden="true" bind:clientWidth={pvWidth}
          style="--pwall: {st[wallKey]}; --ribbon: {st[ribbonKey]}; --wood: var(--wood-{mode}); --wood-v: var(--wood-{mode}-v, none); --wood-back: var(--wood-{mode}-back, none)">
          <div class="zoom"><div class="shelves"><div class="shelf"><div class="books" style="--row-h: {rowH}px">
            {#each pvBooks as p (p.b.key)}
              <div class="slot" class:reading={p.ribbon} data-b={p.b.binding in { hardcover: 1, cloth: 1, leather: 1 } ? p.b.binding : "paperback"} style="--t: {p.t}px; --h: {p.h}px; --c: {p.b.spine.color}; --fg: {p.b.spine.text}"><div class="spine"><span class="st">{p.b.title}</span><span class="sa">{lastName(p.b.author)}</span></div></div>
            {/each}
          </div><div class="plank"></div></div></div></div>
        </div>
      </div>
      <div class="sgroup"><h3>Paint, wood and ribbon</h3>
        <div class="srow stack">{@render name("Wall")}<div class="sctl">
          <div class="swatches walls">
            {#each PAINTS as [n, d, e]}
              {@const c = mode === "day" ? d : e}
              <button type="button" title={n} aria-label={n} style="--sw: {c}" aria-pressed={st[wallKey] === c} onclick={() => paint(c)}></button>
            {/each}
            <button type="button" class="well small" class:on={!PAINTS.some(([, d, e]) => (mode === "day" ? d : e) === st[wallKey])} title="Any color" aria-label="Any wall color" onclick={(e) => picker(e, st[wallKey], paint)}></button>
          </div>
        </div></div>
        <div class="srow stack">{@render name("Wood", WOODS[st[woodKey]])}<div class="sctl"><div class="woods">
          {#each Object.entries(WOODS) as [k, n]}
            <button type="button" title={n} aria-label={n} style="--wood: var(--w-{k})" aria-pressed={st[woodKey] === k} onclick={() => change({ [woodKey]: k })}></button>
          {/each}
        </div></div></div>
        <div class="srow stack">{@render name("Ribbon", "On the books you’re reading")}<div class="sctl"><div class="swatches">
          {#each RIBBONS as [c, n]}
            <button type="button" title={n} aria-label={n} style="--sw: {c}" aria-pressed={st[ribbonKey] === c} onclick={() => change({ [ribbonKey]: c })}></button>
          {/each}
          <button type="button" class="well small" class:on={!RIBBONS.some(([c]) => c === st[ribbonKey])} title="Any color" aria-label="Any ribbon color" onclick={(e) => picker(e, st[ribbonKey], (c) => change({ [ribbonKey]: c }, { soon: true }))}></button>
        </div></div></div>
      </div>
      <div class="sgroup"><h3>Bookcase</h3>
        <p class="sdesc">Which parts of the bookcase to build, in the shelves’ wood.</p>
        {#each PARTS as [k, n]}
          <div class="srow">{@render name(n)}<div class="sctl">{@render sw(st.caseParts[k], (on) => change({ caseParts: { ...st.caseParts, [k]: on } }), n)}</div></div>
        {/each}
      </div>
      <div class="sgroup"><h3>Books</h3>
        <div class="srow">{@render name("Binding details", "Boards, cloth and leather on spines")}<div class="sctl">{@render sw(st.bindings, (on) => change({ bindings: on }), "Binding details")}</div></div>
        <div class="srow">{@render name("Size")}<div class="sctl"><div class="range"><input type="range" min="70" max="130" step="5" aria-label="Size" value={st.bookSize} oninput={(e) => change({ bookSize: +e.target.value }, { soon: true })}><output>{st.bookSize}%</output></div></div></div>
        <div class="srow">{@render name("Spine height", "The tallest a spine may stand")}<div class="sctl"><div class="range"><input type="range" min="70" max="100" step="5" aria-label="Spine height" value={st.maxHeight} oninput={(e) => change({ maxHeight: +e.target.value }, { soon: true })}><output>{st.maxHeight >= 100 ? "No limit" : st.maxHeight + "%"}</output></div></div></div>
      </div>

    {:else if shown === "site"}
      <div class="sgroup"><h3>Header</h3>
        <div class="srow">{@render name("Name")}<div class="sctl"><input class="inp" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" aria-label="Name" value={st.name} oninput={(e) => change({ name: e.target.value }, { soon: true })}></div></div>
        <div class="srow">{@render name("Icon")}<div class="sctl iconpick">
          <img src={iconURL(st, 192)} alt="The library’s icon">
          <label class="btn sm">{st.icon ? "Use another" : "Use your own"}<input type="file" accept="image/*" hidden onchange={(e) => { pickIcon(e.target.files[0]); e.target.value = ""; }}></label>
          {#if st.icon}<button type="button" class="btn sm" onclick={() => pickIcon(null)}>Use Bookshelf’s</button>{/if}
        </div></div>
        <div class="srow">{@render name("Icon beside the name")}<div class="sctl">{@render sw(st.iconShown, (on) => change({ iconShown: on }), "Icon beside the name")}</div></div>
      </div>
      <div class="sgroup"><h3>Links under the name</h3>
        {#each Object.entries(NETS) as [k, n]}
          <div class="srow">{@render name(n.name)}<div class="sctl"><input class="inp" class:bad={badSocial[k]} data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" aria-label={n.name} placeholder={k === "goodreads" ? "Profile link" : k === "instagram" ? "@username" : "Username"} autocomplete="off" spellcheck="false" bind:value={social[k]} oninput={() => typedSocial(k)}></div></div>
        {/each}
      </div>
      <div class="sgroup"><h3>Footer</h3>
        <div class="srow full"><div class="sctl">
          <textarea class="inp" rows="3" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" aria-label="Footer text" placeholder="A line under the shelves" value={st.footerText} oninput={(e) => change({ footerText: e.target.value }, { soon: true })}></textarea>
        </div></div>
        <div class="srow">{@render name("Show the footer")}<div class="sctl">{@render sw(st.footer, (on) => change({ footer: on }), "Show the footer")}</div></div>
      </div>

    {:else if shown === "visitors"}
      <div class="sgroup"><h3>Access</h3>
        <div class="srow">{@render name("Private library", "Visitors see only a sign-in page")}<div class="sctl">{@render sw(st.private, (on) => change({ private: on }, { note: on ? "Saved. Visitors now see a sign-in page." : "Saved. The library is open to visitors." }), "Private library")}</div></div>
        <div class="srow">{@render name("Statistics", "Visitors can open the numbers")}<div class="sctl">{@render sw(st.stats, (on) => change({ stats: on }), "Statistics")}</div></div>
        <div class="srow">{@render name("What you’re reading", "Now reading, ribbons, Statistics")}<div class="sctl">{@render sw(st.showReading, (on) => change({ showReading: on }), "What you’re reading")}</div></div>
        <div class="srow">{@render name("How far along you are", "Page, percent read and start date")}<div class="sctl"><input type="checkbox" class="switch" aria-label="How far along you are" checked={st.showReading && st.showProgress} disabled={!st.showReading} onchange={(e) => change({ showProgress: e.target.checked })}></div></div>
        <p class="fine">Hidden books, the readings you’ve finished and your private notes are never shown to visitors.</p>
      </div>
      <div class="sgroup"><h3>Notes</h3>
        <p class="sdesc">What visitors can read of the notes you keep. Each book and each note can still be set on its own.</p>
        <div class="srow">{@render name("Show notes", "The notes you mark public")}<div class="sctl">{@render sw(st.notes, (on) => change({ notes: on }, { note: on ? undefined : "Saved. Visitors can no longer read any notes." }), "Show notes")}</div></div>
        <div class="srow">{@render name("All notes page", "Every public note, newest first")}<div class="sctl"><input type="checkbox" class="switch" aria-label="All notes page" checked={st.timeline && st.notes} disabled={!st.notes} onchange={(e) => change({ timeline: e.target.checked })}></div></div>
        <div class="srow">{@render name("New notes start as")}<div class="sctl"><div class="seg" role="group" aria-label="New notes start as" use:pill>
          <button type="button" aria-pressed={!st.startsPublic} onclick={() => change({ startsPublic: false })}>Only me</button><button type="button" aria-pressed={st.startsPublic} onclick={() => change({ startsPublic: true })}>Public</button>
        </div></div></div>
        <p class="fine">{#if st.notes}Visitors can read the public notes on {plural(reach.length, "book")}{st.timeline ? " and on All notes" : ""}.{:else}Visitors can read no notes.{/if}</p>
      </div>
      <div class="sgroup"><h3>Hidden books</h3>
        <p class="sdesc">Hide a book’s notes from visitors. Its public notes return when you show it again.</p>
        <p class="fcap">{hiddenNotes.length ? `Hidden from visitors · ${hiddenNotes.length}` : "No book is hidden."}</p>
        {#if hiddenNotes.length}
          <ul class="flist hidden">{#each hiddenNotes as b (b.key)}<li><img src={b.face} alt=""><span><b>{b.title}</b><small>{b.author}</small></span><button type="button" class="lnk" onclick={() => hideNotes(b, false)}>Show again</button></li>{/each}</ul>
        {/if}
        {@render finder("hide", hideQuery, (v) => (hideQuery = v))}
      </div>
      <div class="sgroup aligned"><h3>When they arrive</h3>
        <div class="srow">{@render name("Theme")}<div class="sctl"><div class="seg" role="group" aria-label="Theme" use:pill>
          {#each [["system", "Automatic"], ["light", "Day"], ["dark", "Evening"]] as [k, n]}
            <button type="button" aria-pressed={st.theme === k} onclick={() => change({ theme: k })}>{n}</button>
          {/each}
        </div></div></div>
        <!-- As on the shelf: choosing the sort again turns it round, and the shelf follows. -->
        <div class="srow">{@render name("Sorted by", sortWay[0].toUpperCase() + sortWay.slice(1))}<div class="sctl"><div class="seg" role="group" aria-label="Sorted by" use:pill>
          {#each [["author", "Author"], ["title", "Title"], ["year", "Year"]] as [k, n]}
            {@const on = st.sort === k}
            {@const desc = on && !!st.sortDesc}
            <button type="button" aria-pressed={on} aria-label="{n}, {SORT_WAYS[k][desc ? 1 : 0]}" title={on ? `${SORT_WAYS[k][desc ? 1 : 0]}; again for ${SORT_WAYS[k][desc ? 0 : 1]}` : `Sort by ${n.toLowerCase()}`}
              onclick={() => { const d = on ? !desc : false; change({ sort: k, sortDesc: d }); showSort(k, d); }}>{n}<svg class="dir" class:down={desc} class:shown={on} viewBox="0 0 24 24" aria-hidden="true"><path d="M12 19V5M6 11l6-6 6 6"/></svg></button>
          {/each}
        </div></div></div>
      </div>
      <div class="sgroup aligned"><h3>A book’s page</h3>
        <div class="srow">{@render name("Address", addressOf(st.permalink))}<div class="sctl"><div class="seg" role="group" aria-label="Address" use:pill>
          {#each ADDRESS as [k, n]}
            <button type="button" aria-pressed={st.permalink === k} title={addressOf(k)} onclick={() => change({ permalink: k })}>{n}</button>
          {/each}
        </div></div></div>
      </div>
      <div class="sgroup"><h3>Search links</h3>
        {#each SOURCES as [k, n]}
          <div class="srow">{@render name(n)}<div class="sctl">{@render sw(st.sources.includes(k), (on) => change({ sources: on ? [...st.sources, k] : st.sources.filter((x) => x !== k) }), n)}</div></div>
        {/each}
        <p class="fine">On each book’s page, to look the book up elsewhere.</p>
      </div>

    {:else if shown === "library"}
      <div class="sgroup"><h3>Bring books in</h3>
        <div class="srow full"><div class="sctl">
          <label class="drop" class:over ondragover={(e) => { e.preventDefault(); over = true; }} ondragleave={() => (over = false)} ondrop={(e) => { e.preventDefault(); over = false; importFile(e.dataTransfer.files[0]); }}>
            <span><b>Choose a file</b> or drop it here</span><span class="fine">CSV, JSON or a .bookshelf archive</span>
            <input type="file" accept=".csv,.json,.bookshelf,.zip" hidden onchange={(e) => { importFile(e.target.files[0]); e.target.value = ""; }}>
          </label>
        </div></div>
        {#if importNote}<p class="fine">{importNote}</p>{/if}
      </div>
      <div class="sgroup"><h3>Take them out</h3>
        <div class="srow">{@render name("Export")}<div class="sctl">
          <a class="btn sm" href="/api/export?format=csv" download>CSV</a>
          <a class="btn sm" href="/api/export?format=json" download>JSON</a>
          <a class="btn sm" href="/api/export?format=archive" download>Full archive with covers</a>
        </div></div>
      </div>
      <div class="sgroup"><h3>Export notes</h3>
        <p class="sdesc">Markdown, to keep or to move to another notebook.</p>
        <div class="srow">{@render name("All books")}<div class="sctl"><button type="button" class="btn sm" onclick={() => onExport(null)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.down}</svg>Markdown</button></div></div>
        <div class="sfind"><div class="sname">One book<small>Find it, then take it as Markdown.</small></div>{@render finder("export", exportQuery, (v) => (exportQuery = v))}</div>
      </div>
      <div class="sgroup"><h3>Import notes</h3>
        <p class="sdesc">Highlights and notes from a Kindle, matched to the books on your shelf by title.</p>
        {#if !clips}
          <div class="srow full"><div class="sctl">
            <label class="drop" class:over={clipsOver} ondragover={(e) => { e.preventDefault(); clipsOver = true; }} ondragleave={() => (clipsOver = false)} ondrop={(e) => { e.preventDefault(); clipsOver = false; readClippings(e.dataTransfer.files[0]); }}>
              <span><b>Choose My Clippings.txt</b> or drop it here</span><span class="fine">Kindle keeps it in its documents folder.</span>
              <input type="file" accept=".txt,text/plain" hidden disabled={clipsBusy} onchange={(e) => { readClippings(e.target.files[0]); e.target.value = ""; }}>
            </label>
          </div></div>
        {:else}
          <p class="fine">Read {plural(clips.plan.clippings, "clipping")} from {plural(clips.plan.groups.length, "book")} in <b>{clips.name}</b>. A note beside a highlight became one note, repeated highlights were folded together, bookmarks were left out.</p>
          <ul class="imp-list">
            {#each clips.plan.groups as g, i (i)}
              {#if g.bookId}
                <li><b>{g.book}</b><small>{g.add ? `${g.add} to add` : "Nothing new"}{g.same ? `${g.add ? ", " : ". "}${g.same} already here` : ""}</small><span class="st2" class:ok={g.add}>{g.add ? "On your shelf" : "Up to date"}</span></li>
              {:else}
                <li class="skip"><b>{g.title}</b><small>{g.author ? `${g.author} · ` : ""}{plural(g.total, "clipping")}</small><span class="st2">Not on your shelf, skipped</span></li>
              {/if}
            {/each}
          </ul>
          <div class="srow">{@render name("Imported notes are", "Kindle’s page, or else its location")}<div class="sctl"><div class="seg" role="group" aria-label="Imported notes are" use:pill>
            <button type="button" aria-pressed={!clipsPublic} onclick={() => (clipsPublic = false)}>Only me</button><button type="button" aria-pressed={clipsPublic} onclick={() => (clipsPublic = true)}>Public</button>
          </div></div></div>
          <div class="row-btns end"><button type="button" class="btn sm" disabled={clipsBusy} onclick={() => (clips = null)}>Cancel</button><button type="button" class="btn sm primary" disabled={clipsBusy || !clips.plan.add} onclick={runClippings}>Add {plural(clips.plan.add, "note")} to {plural(clips.plan.books, "book")}</button></div>
        {/if}
      </div>
      <div class="sgroup"><h3>Backups</h3>
        <div class="srow">{@render name("Back up")}<div class="sctl"><select class="inp" aria-label="Back up" value={st.backupEvery} onchange={(e) => change({ backupEvery: e.target.value })}><option value="day">Every day</option><option value="week">Every week</option><option value="never">Only before imports and restores</option></select></div></div>
        <div class="srow">{@render name("Keep", "The newest are kept")}<div class="sctl"><select class="inp" aria-label="Keep" value={String(st.backupKeep)} onchange={(e) => change({ backupKeep: +e.target.value })}>{#each [7, 14, 30, 60, 90] as n}<option value={String(n)}>{n}</option>{/each}</select></div></div>
        <div class="srow full"><div class="sctl"><ul class="backups">
          {#each backups as x (x.name)}
            <li>
              <span>{stamp(x.createdAt)}<small>{KINDS[x.kind] || x.kind} · {x.books.toLocaleString("en-US")} book{x.books === 1 ? "" : "s"}</small></span>
              {#if restoring === x.name}
                <span class="row-btns"><button type="button" class="btn sm danger solid" onclick={() => restore(x)}>Restore it</button><button type="button" class="btn sm" onclick={() => (restoring = null)}>Cancel</button></span>
              {:else}
                <span class="row-btns"><a class="btn sm" href="/api/backups/{encodeURIComponent(x.name)}" download>Download</a><button type="button" class="btn sm" onclick={() => (restoring = x.name)}>Restore</button></span>
              {/if}
            </li>
          {:else}
            <li><span class="fine">No backups yet.</span></li>
          {/each}
        </ul></div></div>
        <div class="srow">{@render name("")}<div class="sctl">
          <button type="button" class="btn sm" onclick={backupNow}>Back up now</button>
          <label class="btn sm">Restore from a file<input type="file" accept=".bookshelf,.zip" hidden onchange={(e) => { upload = e.target.files[0] || null; e.target.value = ""; }}></label>
        </div></div>
        {#if upload}
          <div class="rd-ask"><span>Replace your whole library with {upload.name}? What’s here now is backed up first.</span>
            <button type="button" class="btn sm danger solid" disabled={uploading} onclick={restoreFile}>{uploading ? "Restoring…" : "Restore it"}</button><button type="button" class="btn sm" disabled={uploading} onclick={() => (upload = null)}>Cancel</button></div>
        {/if}
        <p class="fine">Kept in the data folder’s <span class="tag">backups</span> folder (<span class="tag">/data/backups</span> in Docker).</p>
      </div>

    {:else if shown === "about"}
      <div class="sgroup"><h3>Account</h3>
        <div class="srow">{@render name(`Signed in as ${ui.user}`)}<div class="sctl"><button type="button" class="btn sm" onclick={out}>Sign out</button></div></div>
        <p class="fine">To change it, set <span class="tag">BOOKSHELF_ADMIN_PASSWORD</span> or run <span class="tag">bookshelf set-password</span>, then restart.</p>
      </div>
      <div class="sgroup"><h3>Version</h3>
        {#if ui.update?.dev}
          <div class="srow">{@render name(`Bookshelf ${ui.update.version}`, "Development builds aren’t checked")}<div class="sctl"></div></div>
        {:else if ui.update?.newer}
          <div class="srow">{@render name(`Bookshelf ${ui.update.latest} is out`, `You have ${ui.update.version}`)}<div class="sctl">
            <a class="btn sm" href={ui.update.url} target="_blank" rel="noopener">What’s new</a><a class="btn sm" href={UPDATING} target="_blank" rel="noopener">How to update</a>
          </div></div>
        {:else if ui.update}
          <div class="srow">{@render name(`Bookshelf ${ui.update.version}`)}<div class="sctl"><span class="status" class:ok={st.checkUpdates && ui.update.checkedAt}>
            {#if !st.checkUpdates}Not checked{:else if ui.update.problem && !ui.update.checkedAt}The last check didn’t work{:else if ui.update.checkedAt}Up to date{:else}Not checked yet{/if}
          </span></div></div>
        {/if}
        {#if !ui.update?.dev}
          <div class="srow">{@render name("Check for updates", "Asks GitHub; nothing is shared")}<div class="sctl">{@render sw(st.checkUpdates, toggleUpdates, "Check for updates")}</div></div>
          {#if ui.update && !ui.update.newer}
            <p class="fine">
              {#if !st.checkUpdates}New versions are listed on the <a class="link" href={RELEASES} target="_blank" rel="noopener">releases page</a>.
              {:else if ui.update.problem && !ui.update.checkedAt}{ui.update.problem}.
              {:else if ui.update.checkedAt}Checked {checked(ui.update.checkedAt)}.{/if}
            </p>
          {/if}
        {/if}
      </div>
      <div class="sgroup"><h3>Bookshelf</h3>
        <p class="credit">Bookshelf is made by Abdullah Aloğlu. Its code is free to read, change and share.</p>
        <p class="credit-links"><a class="link" href="https://alog.lu" target="_blank" rel="noopener">Website</a><a class="link" href="https://github.com/aloglu/bookshelf" target="_blank" rel="noopener">GitHub</a></p>
      </div>
    {/if}
    </div>
    </div>
  </div>
  {/if}
</section>
