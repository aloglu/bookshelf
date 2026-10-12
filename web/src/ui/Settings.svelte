<script>
  // Settings: one sheet with its sections down the side (on a phone, the list of sections and then one at a
  // time). Every change shows at once and saves itself; dragging a color or a slider saves when it comes to
  // rest. The shelf section has a preview of the shelf, by day or by evening, drawn by the shelf's own rules.
  import { tick } from "svelte";
  import { ui, data, patchSettings, reload, signOut, showSort, checkUpdates, setIcon, iconURL } from "../lib/app.svelte.js";
  import { SORT_WAYS, isPhone, lastName, reheight, sortBooks } from "../engine/model.js";
  import { api } from "../lib/api.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast, savedToast } from "../lib/toast.js";
  import { pill } from "../lib/pill.js";
  import { pickColor } from "../lib/colorpicker.js";
  import { applyLook, WOODS } from "../engine/look.js";
  import { relayout, view } from "../engine/shelf.js";

  // The footer a library starts with (the server's DefaultFooter), to put back after changing it.
  const ORIGINAL_FOOTER = "Made by [alog.lu](https://alog.lu) · Code available on [GitHub](https://github.com/aloglu/bookshelf)";
  const SECTIONS = [
    ["shelf", "Shelf", "How the shelf looks, by day and by evening."],
    ["site", "Header and footer", "The name above the shelf and the line beneath it."],
    ["visitors", "Visitors", "Who can see the library, and how it opens for them."],
    ["library", "Your library", ""],
    ["about", "Bookshelf", "The version you run, and your account."],
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

  let wrap, st = $state(null), section = $state("shelf"), inside = $state(true), mode = $state("day");
  let backups = $state([]), importNote = $state(""), over = $state(false), restoring = $state(null);
  let social = $state({}), badSocial = $state({});

  // Opens on the first section on a wide screen, or on the list of sections on a phone.
  export async function open() {
    if (!data.library) return;   // still loading; the button only shows once it has
    st = structuredClone($state.snapshot(data.library.settings));
    social = Object.fromEntries(Object.keys(NETS).map((k) => [k, (st.social[k] || "").replace(/^https:\/\/(www\.|app\.)?/, "")]));
    badSocial = {}; importNote = ""; restoring = null;
    section = "shelf"; inside = !isPhone(); settle();
    mode = document.documentElement.dataset.theme === "dark" || (!document.documentElement.dataset.theme && matchMedia("(prefers-color-scheme: dark)").matches) ? "night" : "day";
    await tick();   // the section is drawn before it takes focus
    showPanel(wrap, inside ? ".spane-head [data-close]" : ".phone-x");
    loadBackups();
    checkUpdates();
  }
  function go(k) { section = k; inside = true; pane?.scrollTo({ top: 0 }); settle(); }
  let pane = $state();
  // While a section is first drawn nothing animates (its switches and segments arrive as they are); only a
  // change the owner makes slides.
  let still = $state(false);
  function settle() { still = true; requestAnimationFrame(() => requestAnimationFrame(() => (still = false))); }
  async function loadBackups() { try { backups = await api("GET", "/api/backups"); } catch { backups = []; } }

  // A change shows at once (on the shelf, the walls, the wood) and is saved: at once for a click, or once the
  // owner stops typing or dragging.
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
  // What's being chosen: the day's paint and wood, or the evening's.
  const wallKey = $derived(mode === "day" ? "wallDay" : "wallNight");
  const woodKey = $derived(mode === "day" ? "woodDay" : "woodNight");
  const paint = (wall) => change({ [wallKey]: wall }, { soon: true });
  // The preview: the library's own books, the first of the shelf, as many as fill most of its width, one of
  // them being read if any is. They are drawn at the wide screen's shelf scale on every screen (where the
  // lettering fits the spines as it should) and the whole shelf is zoomed down to a box of fixed height, sized
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
    const reader = shown.find((b) => b.reading);
    if (reader && !list.includes(reader) && list.length) list[Math.floor(list.length / 2)] = reader;
    return list.map((b) => ({ b, t: Math.round(b.t * k), h: Math.round(b.h * k) }));
  });
  const rowH = $derived(Math.max(0, ...pvBooks.map((p) => p.h)) + 4);
  function picker(e, value, set) { pickColor(e.currentTarget, value, { input: set }); }

  /* ---------- Visitors ---------- */
  const eg = $derived(data.books.find((b) => b.isbn && !b.hidden) || data.books[0]);
  const addressOf = (k) => "/" + ((k === "isbn" ? eg?.isbn || eg?.slug : k === "hyphens" ? (eg?.isbn ? eg.identifier : eg?.slug) : eg?.slug) || "…");
  const sortWay = $derived(st ? SORT_WAYS[st.sort][st.sortDesc ? 1 : 0] : "");

  /* ---------- Your library ---------- */
  const hidden = $derived(data.books.filter((b) => b.hidden).length);
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
  async function out() { await hidePanel(wrap); try { await signOut(); } catch (err) { toast(err.message); } }
  const ICONS = {
    shelf: `<path d="M3 20h18M5 20V6h3v14M10 20V4h3v16M15.5 20l-1-12 3-.4 1.4 12.4"/>`,
    site: `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M7 6.5h4"/>`,
    visitors: `<circle cx="12" cy="8" r="3.5"/><path d="M5 20c.8-3.6 3.6-5.5 7-5.5s6.2 1.9 7 5.5"/>`,
    library: `<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5M5 19h14"/>`,
    about: `<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5M12 8h.01"/>`,
  };
</script>

{#snippet sw(checked, onchange, label)}<input type="checkbox" class="switch" aria-label={label} {checked} onchange={(e) => onchange(e.target.checked)}>{/snippet}
{#snippet name(text, hint)}<div class="sname">{text}{#if hint}<small>{hint}</small>{/if}</div>{/snippet}

<div class="modal settings-wrap" id="settings" hidden bind:this={wrap}>
  <div class="modal-scrim" data-close></div>
  <section class="card settings" class:inside class:still role="dialog" aria-modal="true" aria-labelledby="settings-h">
    <nav class="snav" aria-label="Settings">
      <h2 id="settings-h">Settings</h2>
      {#each SECTIONS as [k, title]}
        <button type="button" aria-current={section === k && inside} class:dot={k === "about" && ui.update?.newer} onclick={() => go(k)}>
          <svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICONS[k]}</svg><span>{title}</span>
          <svg class="i chev" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>
        </button>
      {/each}
      <span class="spacer"></span>
      {#if ui.update?.version}<p class="ver">Bookshelf {ui.update.version}</p>{/if}
      <button type="button" class="btn icon x phone-x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    </nav>
    {#if st}
    {@const [, title, lede] = SECTIONS.find(([k]) => k === section)}
    <div class="spane" bind:this={pane}>
      <div class="spane-head">
        <button type="button" class="btn icon back" aria-label="All settings" onclick={() => (inside = false)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 6-6 6 6 6"/></svg></button>
        <h2>{title}</h2>
        <button type="button" class="btn icon" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
      </div>
      {#if section === "library"}
        <p class="slede">{data.books.length.toLocaleString("en-US")} book{data.books.length === 1 ? "" : "s"} on your shelves{hidden ? `, ${hidden} of them hidden from the website` : ""}.</p>
      {:else}
        <p class="slede">{lede}</p>
      {/if}

      <div class="sbody">
      {#if section === "shelf"}
        <div class="pvbox">
          <div class="seg modeswitch" role="group" aria-label="Choose the paint and wood for" use:pill>
            <button type="button" aria-pressed={mode === "day"} onclick={() => (mode = "day")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4"/></svg>Day</button>
            <button type="button" aria-pressed={mode === "night"} onclick={() => (mode = "night")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z"/></svg>Evening</button>
          </div>
          <div class="shelfpv" class:night={mode === "night"} aria-hidden="true" bind:clientWidth={pvWidth}
            style="--pwall: {st[wallKey]}; --wood: var(--wood-{mode}); --wood-v: var(--wood-{mode}-v, none); --wood-back: var(--wood-{mode}-back, none)">
            <div class="zoom"><div class="shelves"><div class="shelf"><div class="books" style="--row-h: {rowH}px">
              {#each pvBooks as p (p.b.key)}
                <div class="slot" class:reading={p.b.reading} data-b={p.b.binding in { hardcover: 1, cloth: 1, leather: 1 } ? p.b.binding : "paperback"} style="--t: {p.t}px; --h: {p.h}px; --c: {p.b.spine.color}; --fg: {p.b.spine.text}"><div class="spine"><span class="st">{p.b.title}</span><span class="sa">{lastName(p.b.author)}</span></div></div>
              {/each}
            </div><div class="plank"></div></div></div></div>
          </div>
        </div>
        <div class="sgroup"><h3>Paint and wood</h3>
          <div class="srow stack">{@render name("Wall")}<div class="sctl">
            <div class="swatches walls">
              {#each PAINTS as [n, d, e]}
                {@const c = mode === "day" ? d : e}
                <button type="button" title={n} aria-label={n} style="--sw: {c}" aria-pressed={st[wallKey] === c} onclick={() => paint(c)}></button>
              {/each}
              <button type="button" class="well" title="Your own" aria-label="Your own wall color" style="--pick: {st[wallKey]}" onclick={(e) => picker(e, st[wallKey], paint)}></button>
            </div>
          </div></div>
          <div class="srow stack">{@render name("Wood", WOODS[st[woodKey]])}<div class="sctl"><div class="woods">
            {#each Object.entries(WOODS) as [k, n]}
              <button type="button" title={n} aria-label={n} style="--wood: var(--w-{k})" aria-pressed={st[woodKey] === k} onclick={() => change({ [woodKey]: k })}></button>
            {/each}
          </div></div></div>
        </div>
        <div class="sgroup"><h3>Bookcase</h3>
          <p class="sdesc">Each part on its own, in the shelves’ wood: the shelves the books stand on, the sides, a top and a back panel. With all of them off, the books stand on their own.</p>
          {#each PARTS as [k, n]}
            <div class="srow">{@render name(n)}<div class="sctl">{@render sw(st.caseParts[k], (on) => change({ caseParts: { ...st.caseParts, [k]: on } }), n)}</div></div>
          {/each}
        </div>
        <div class="sgroup"><h3>Books</h3>
          <div class="srow">{@render name("Binding details", "Hardcovers show their boards, cloth its weave, leather its bands and gilt")}<div class="sctl">{@render sw(st.bindings, (on) => change({ bindings: on }), "Binding details")}</div></div>
          <div class="srow">{@render name("Size")}<div class="sctl"><div class="range"><input type="range" min="70" max="130" step="5" aria-label="Size" value={st.bookSize} oninput={(e) => change({ bookSize: +e.target.value }, { soon: true })}><output>{st.bookSize}%</output></div></div></div>
          <div class="srow">{@render name("Spine height", "The tallest a spine may stand")}<div class="sctl"><div class="range"><input type="range" min="70" max="100" step="5" aria-label="Spine height" value={st.maxHeight} oninput={(e) => change({ maxHeight: +e.target.value }, { soon: true })}><output>{st.maxHeight >= 100 ? "No limit" : st.maxHeight + "%"}</output></div></div></div>
          <div class="srow stack">{@render name("Ribbon", "On the books you’re reading")}<div class="sctl"><div class="swatches">
            {#each RIBBONS as [c, n]}
              <button type="button" title={n} aria-label={n} style="--sw: {c}" aria-pressed={st.ribbon === c} onclick={() => change({ ribbon: c })}></button>
            {/each}
            <button type="button" class="well small" class:on={!RIBBONS.some(([c]) => c === st.ribbon)} title="Any color" aria-label="Any ribbon color" onclick={(e) => picker(e, st.ribbon, (c) => change({ ribbon: c }, { soon: true }))}></button>
          </div></div></div>
        </div>

      {:else if section === "site"}
        <div class="sgroup"><h3>Header</h3>
          <div class="srow">{@render name("Name")}<div class="sctl"><input class="inp" autocomplete="off" aria-label="Name" value={st.name} oninput={(e) => change({ name: e.target.value }, { soon: true })}></div></div>
          <div class="srow">{@render name("Icon")}<div class="sctl iconpick">
            <img src={iconURL(st, 192)} alt="The library’s icon">
            <label class="btn sm">{st.icon ? "Use another" : "Use your own"}<input type="file" accept="image/*" hidden onchange={(e) => { pickIcon(e.target.files[0]); e.target.value = ""; }}></label>
            {#if st.icon}<button type="button" class="btn sm" onclick={() => pickIcon(null)}>Use Bookshelf’s</button>{/if}
          </div></div>
          <div class="srow">{@render name("Icon beside the name")}<div class="sctl">{@render sw(st.iconShown, (on) => change({ iconShown: on }), "Icon beside the name")}</div></div>
        </div>
        <div class="sgroup"><h3>Links under the name</h3>
          {#each Object.entries(NETS) as [k, n]}
            <div class="srow">{@render name(n.name)}<div class="sctl"><input class="inp" class:bad={badSocial[k]} aria-label={n.name} placeholder={k === "goodreads" ? "Profile link" : k === "instagram" ? "@username" : "Username"} autocomplete="off" spellcheck="false" bind:value={social[k]} oninput={() => typedSocial(k)}></div></div>
          {/each}
        </div>
        <div class="sgroup"><h3>Footer</h3>
          <div class="srow full"><div class="sctl">
            <textarea class="inp" rows="3" aria-label="Footer text" value={st.footerText} oninput={(e) => change({ footerText: e.target.value }, { soon: true })}></textarea>
            {#if st.footerText !== ORIGINAL_FOOTER}<button type="button" class="btn sm" onclick={() => change({ footerText: ORIGINAL_FOOTER })}>Use the original</button>{/if}
          </div></div>
          <div class="srow">{@render name("Show the footer")}<div class="sctl">{@render sw(st.footer, (on) => change({ footer: on }), "Show the footer")}</div></div>
        </div>

      {:else if section === "visitors"}
        <div class="sgroup"><h3>Access</h3>
          <div class="srow">{@render name("Private library", "Visitors see only a sign-in page")}<div class="sctl">{@render sw(st.private, (on) => change({ private: on }, { note: on ? "Saved. Visitors now see a sign-in page." : "Saved. The library is open to visitors." }), "Private library")}</div></div>
          <div class="srow">{@render name("Statistics", "Visitors can open the numbers")}<div class="sctl">{@render sw(st.stats, (on) => change({ stats: on }), "Statistics")}</div></div>
          <p class="fine">Hidden books and your readings are never shown to visitors, nor private notes.</p>
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

      {:else if section === "library"}
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

      {:else if section === "about"}
        <div class="sgroup"><h3>Version</h3>
          {#if ui.update?.dev}
            <div class="srow">{@render name(`Bookshelf ${ui.update.version}`, "A development build, so it isn’t checked for updates")}<div class="sctl"></div></div>
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
            <div class="srow">{@render name("Check for updates", "Asks GitHub; nothing about your library is sent")}<div class="sctl">{@render sw(st.checkUpdates, toggleUpdates, "Check for updates")}</div></div>
            {#if ui.update && !ui.update.newer}
              <p class="fine">
                {#if !st.checkUpdates}New versions are listed on the <a class="link" href={RELEASES} target="_blank" rel="noopener">releases page</a>.
                {:else if ui.update.problem && !ui.update.checkedAt}{ui.update.problem}.
                {:else if ui.update.checkedAt}Checked {checked(ui.update.checkedAt)}.{/if}
              </p>
            {/if}
          {/if}
        </div>
        <div class="sgroup"><h3>Account</h3>
          <div class="srow">{@render name(`Signed in as ${ui.user}`)}<div class="sctl"><button type="button" class="btn sm" onclick={out}>Sign out</button></div></div>
          <p class="fine">To change the password, change <span class="tag">BOOKSHELF_ADMIN_PASSWORD</span> where Bookshelf runs (or run <span class="tag">bookshelf set-password</span> again) and restart it.</p>
        </div>
      {/if}
      </div>
    </div>
    {/if}
  </section>
</div>
