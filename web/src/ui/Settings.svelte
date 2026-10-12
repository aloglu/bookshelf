<script>
  // Settings: the site, its look, the library's files and backups, the account. Every change shows at once
  // and saves itself; dragging a color or a slider saves when it comes to rest.
  import { ui, data, patchSettings, reload, signOut, setSort, checkUpdates, setIcon, iconURL } from "../lib/app.svelte.js";
  import { api } from "../lib/api.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast, savedToast } from "../lib/toast.js";
  import { applyLook, presetOf, PRESETS, WOODS } from "../engine/look.js";
  import { relayout, view } from "../engine/shelf.js";
  import { BINDING_NAMES, lastName, reheight, spineHeight } from "../engine/model.js";

  let wrap, tab = $state("site"), st = $state(null);
  let backups = $state([]), importNote = $state(""), over = $state(false), restoring = $state(null);
  const NETS = {
    goodreads: { name: "Goodreads", host: /(^|\.)goodreads\.com$/, url: (v) => /^\d+(-[\w-]+)?$/.test(v) ? `https://www.goodreads.com/user/show/${v}` : `https://www.goodreads.com/${v}` },
    storygraph: { name: "The StoryGraph", host: /(^|\.)thestorygraph\.com$/, url: (v) => `https://app.thestorygraph.com/profile/${v}` },
    instagram: { name: "Instagram", host: /(^|\.)instagram\.com$/, url: (v) => `https://www.instagram.com/${v}/` },
  };
  let social = $state({}), badSocial = $state({});
  const RIBBONS = [["#8e2532", "Crimson"], ["#23365e", "Navy"], ["#2e5a3c", "Forest"], ["#b8902f", "Old gold"], ["#1d1a19", "Black"]];
  const PAIRS = [["linen", "Linen", "#e3d8c5", "#17110d"], ["tobacco", "Tobacco", "#c4a47d", "#2a1c12"], ["chestnut", "Chestnut", "#8d5d3b", "#3a2416"], ["umber", "Umber", "#4f3828", "#1e1510"], ["forest", "Forest green", "#364637", "#152018"]];

  export async function open(which = "site") {
    if (!data.library) return;   // still loading; the button only shows once it has
    tab = which;
    st = structuredClone($state.snapshot(data.library.settings));
    social = Object.fromEntries(Object.keys(NETS).map((k) => [k, (st.social[k] || "").replace(/^https:\/\/(www\.|app\.)?/, "")]));
    badSocial = {}; importNote = ""; restoring = null;
    showPanel(wrap, "[data-close]");
    loadBackups();
    checkUpdates();
  }
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
    if ("bindings" in patch || "maxHeight" in patch || "bookSize" in patch || "bookcase" in patch || "caseParts" in patch) relayout(false);
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
  // Walls: a named pair, or either wall on its own.
  const preset = $derived(st ? presetOf(st.wallDay, st.wallNight) : null);
  function hexTyped(key, e) {
    let v = e.target.value.trim(); if (!v.startsWith("#")) v = "#" + v;
    if (/^#[0-9a-f]{3}$/i.test(v)) v = "#" + [...v.slice(1)].map((x) => x + x).join("");
    if (/^#[0-9a-f]{6}$/i.test(v) && v.toLowerCase() !== st[key]) change({ [key]: v.toLowerCase() }, { soon: true });
  }
  // The preview: one real book from the shelf in each binding, at the shelf's scale.
  const preview = $derived(!st ? [] : Object.entries(BINDING_NAMES).map(([k, name], i) => {
    const b = data.books.find((x) => x.binding === k) || data.books[i % Math.max(1, data.books.length)];
    return b && { k, name, b, h: Math.round(spineHeight({ ...b, binding: k, seed: 3 }, st.bindings) * .8) };
  }).filter(Boolean));
  const eg = $derived(data.books.find((b) => b.isbn && !b.hidden) || data.books[0]);
  const hyphens = $derived(eg?.isbn ? eg.identifier : eg?.slug);

  /* ---------- Library ---------- */
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
</script>

<div class="drawer-wrap" id="drawer-wrap" hidden bind:this={wrap}>
  <div class="drawer-scrim" data-close></div>
  <aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-h">
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    <h2 id="drawer-h">Settings</h2>
    <div class="seg dtabs" role="tablist">
      {#each [["site", "Site"], ["look", "Look"], ["library", "Library"], ["account", "Account"]] as [k, name]}
        <button type="button" role="tab" aria-selected={tab === k} class:dot={k === "account" && ui.update?.newer} onclick={() => (tab = k)}>{name}</button>
      {/each}
    </div>
    {#if st}
    <section hidden={tab !== "site"}>
      <label class="lab">Name<input class="inp" autocomplete="off" value={st.name} oninput={(e) => change({ name: e.target.value }, { soon: true })}></label>
      <div class="lab">Icon<small class="sub-lab">For the browser tab and a phone’s home screen; beside the name too, if you like.</small>
        <div class="iconpick">
          <img src={iconURL(st, 192)} alt="The library’s icon">
          <div class="row-btns">
            <label class="btn sm">{st.icon ? "Use another" : "Use your own"}<input type="file" accept="image/*" hidden onchange={(e) => { pickIcon(e.target.files[0]); e.target.value = ""; }}></label>
            {#if st.icon}<button type="button" class="btn sm" onclick={() => pickIcon(null)}>Use Bookshelf’s</button>{/if}
          </div>
        </div>
      </div>
      <label class="toggle"><span><b>Show the icon beside the name</b><small>In the header, before the library’s name.</small></span><input type="checkbox" checked={st.iconShown} onchange={(e) => change({ iconShown: e.target.checked })}></label>
    </section>
    <section hidden={tab !== "site"}>
      <h3>What visitors see</h3>
      <label class="toggle"><span><b>Private library</b><small>Only you can see it. Visitors get a sign-in page.</small></span><input type="checkbox" checked={st.private} onchange={(e) => change({ private: e.target.checked }, { note: e.target.checked ? "Saved. Visitors now see a sign-in page." : "Saved. The library is open to visitors." })}></label>
      <label class="toggle"><span><b>Statistics page</b><small>Let visitors see the numbers behind your library.</small></span><input type="checkbox" checked={st.stats} onchange={(e) => change({ stats: e.target.checked })}></label>
      <p class="fine">Any book can be hidden from its own page. Your readings are never shown, and a note only when you make it public.</p>
    </section>
    <section hidden={tab !== "site"}>
      <h3>How it opens</h3>
      <div class="lab">Sorted by<div class="seg">
        {#each [["author", "Author"], ["title", "Title"], ["year", "Year"]] as [k, name]}
          <button type="button" aria-pressed={st.sort === k} onclick={() => { change({ sort: k }); if (ui.sort !== k) setSort(k); }}>{name}</button>
        {/each}
      </div></div>
      <div class="lab">Day or evening<div class="seg">
        {#each [["system", "System"], ["light", "Day"], ["dark", "Evening"]] as [k, name]}
          <button type="button" aria-pressed={st.theme === k} onclick={() => change({ theme: k })}>{name}</button>
        {/each}
      </div></div>
    </section>
    <section hidden={tab !== "site"}>
      <h3>Elsewhere</h3>
      <p class="fine">Shown under your library’s name. Paste a profile link or type a username.</p>
      <div class="social">
        {#each Object.entries(NETS) as [k, n]}
          <label class="lab">{n.name}<input class="inp" class:bad={badSocial[k]} placeholder={k === "goodreads" ? "goodreads.com/user/show/…" : k === "instagram" ? "@username" : "Username"} autocomplete="off" spellcheck="false" bind:value={social[k]} oninput={() => typedSocial(k)}></label>
        {/each}
      </div>
    </section>
    <section hidden={tab !== "site"}>
      <h3>Links</h3>
      <div class="lab">Each book’s address
        <div class="radios">
          {#each [["isbn", eg?.isbn || eg?.slug], ["hyphens", hyphens], ["title", eg?.slug]] as [k, example]}
            <label><input type="radio" name="permalink" value={k} checked={st.permalink === k} onchange={() => change({ permalink: k })}><code>/<span>{example || "…"}</span></code></label>
          {/each}
        </div>
      </div>
      <div class="lab">Search the book on
        <div class="checks">
          {#each [["openlibrary", "Open Library"], ["google", "Google Books"], ["goodreads", "Goodreads"]] as [k, name]}
            <label><input type="checkbox" checked={st.sources.includes(k)} onchange={(e) => change({ sources: e.target.checked ? [...st.sources, k] : st.sources.filter((x) => x !== k) })}>{name}</label>
          {/each}
        </div>
      </div>
    </section>

    <section hidden={tab !== "look"}>
      <div class="lab">Wall color
        {#each [["wallDay", "l", "Day"], ["wallNight", "d", "Evening"]] as [key, m, when]}
          <div class="wallpick" data-m={m}>
            <label class="well" title="Pick the {when.toLowerCase()} wall" style:--pick={st[key]}><input type="color" aria-label="{when} wall color" value={st[key]} oninput={(e) => change({ [key]: e.target.value }, { soon: true })}></label>
            <input class="inp hex" maxlength="7" spellcheck="false" autocomplete="off" aria-label="{when} wall as a hex code" value={st[key].toUpperCase()} oninput={(e) => hexTyped(key, e)}>
            <span class="when">{when}</span>
          </div>
        {/each}
        <div class="swatches" id="walls">
          <span class="fine">Or start from a pair</span>
          {#each PAIRS as [k, name, sw, sn]}
            <button type="button" aria-label={name} title={name} style="--sw: {sw}; --sn: {sn}" aria-pressed={preset === k} onclick={() => change({ wallDay: PRESETS[k][0], wallNight: PRESETS[k][1] })}></button>
          {/each}
        </div>
      </div>
    </section>
    <section hidden={tab !== "look"}>
      <div class="lab">Wood<small class="sub-lab">For the shelves, and the bookcase when there is one.</small>
        {#each [["woodDay", "Day"], ["woodNight", "Evening"]] as [key, when]}
          <div class="woodrow"><span class="when">{when} · <span>{WOODS[st[key]]}</span></span><div class="woods">
            {#each Object.entries(WOODS) as [k, name]}
              <button type="button" title={name} aria-label={name} style="--wood: var(--w-{k})" aria-pressed={st[key] === k} onclick={() => change({ [key]: k })}></button>
            {/each}
          </div></div>
        {/each}
      </div>
    </section>
    <section hidden={tab !== "look"}>
      <label class="toggle"><span><b>Bookcase</b><small>Stand the shelves in a bookcase of the same wood.</small></span><input type="checkbox" checked={st.bookcase} onchange={(e) => change({ bookcase: e.target.checked })}></label>
      <div class="checks inline">
        {#each [["sides", "Sides"], ["top", "Top"], ["back", "Back panel"]] as [k, name]}
          <label><input type="checkbox" disabled={!st.bookcase} checked={st.caseParts[k]} onchange={(e) => change({ caseParts: { ...st.caseParts, [k]: e.target.checked } })}>{name}</label>
        {/each}
      </div>
    </section>
    <section hidden={tab !== "look"}>
      <div class="lab">Reading ribbon<small class="sub-lab">Marks the books you are reading on the shelf.</small>
        <div class="swatches ribbons">
          {#each RIBBONS as [c, name]}
            <button type="button" aria-label={name} title={name} style="--sw: {c}" aria-pressed={st.ribbon === c} onclick={() => change({ ribbon: c })}></button>
          {/each}
          <label class="well small" class:on={!RIBBONS.some(([c]) => c === st.ribbon)} title="Any color"><input type="color" aria-label="Ribbon color" value={st.ribbon} oninput={(e) => change({ ribbon: e.target.value }, { soon: true })}></label>
        </div>
      </div>
    </section>
    <section hidden={tab !== "look"}>
      <label class="toggle"><span><b>Show bindings on the spines</b><small>Hardcover, cloth and leather each look like themselves, and stand a little taller.</small></span><input type="checkbox" checked={st.bindings} onchange={(e) => change({ bindings: e.target.checked })}></label>
      <div class="bindprev" aria-hidden="true">
        {#each preview as p (p.k)}
          <figure>
            <div class="lie" style="--t: 30px; --h: {p.h}px"><div class="slot" data-b={p.k} style="--c: {p.b.spine.color}; --fg: {p.b.spine.text}; --t: 30px; --h: {p.h}px"><div class="spine"><span class="st">{p.b.title}</span><span class="sa">{lastName(p.b.author)}</span></div></div></div>
            <figcaption>{p.name}</figcaption>
          </figure>
        {/each}
      </div>
    </section>
    <section hidden={tab !== "look"}>
      <label class="lab">Book size <output>{st.bookSize}%</output><input type="range" min="70" max="130" step="5" value={st.bookSize} oninput={(e) => change({ bookSize: +e.target.value }, { soon: true })}></label>
      <label class="lab">Tallest spine <output>{st.maxHeight >= 100 ? "No limit" : st.maxHeight + "%"}</output><input type="range" min="70" max="100" step="5" value={st.maxHeight} oninput={(e) => change({ maxHeight: +e.target.value }, { soon: true })}></label>
    </section>

    <section hidden={tab !== "library"}>
      <p class="lede">{data.books.length.toLocaleString("en-US")} book{data.books.length === 1 ? "" : "s"} on your shelves{hidden ? `, ${hidden} of them hidden from the website` : ""}.</p>
      <h3>Bring books in</h3>
      <label class="drop" class:over ondragover={(e) => { e.preventDefault(); over = true; }} ondragleave={() => (over = false)} ondrop={(e) => { e.preventDefault(); over = false; importFile(e.dataTransfer.files[0]); }}>
        <span><b>Choose a file</b> or drop it here</span><span class="fine">CSV, JSON or a .bookshelf archive</span>
        <input type="file" accept=".csv,.json,.bookshelf,.zip" hidden onchange={(e) => { importFile(e.target.files[0]); e.target.value = ""; }}>
      </label>
      <p class="fine" hidden={!importNote}>{importNote}</p>
    </section>
    <section hidden={tab !== "library"}>
      <h3>Take them out</h3>
      <div class="row-btns">
        <a class="btn sm" href="/api/export?format=csv" download>CSV</a>
        <a class="btn sm" href="/api/export?format=json" download>JSON</a>
        <a class="btn sm" href="/api/export?format=archive" download>Full archive with covers</a>
      </div>
    </section>
    <section hidden={tab !== "library"}>
      <h3>Backups</h3>
      <div class="pair">
        <label class="lab">Back up<select class="inp" value={st.backupEvery} onchange={(e) => change({ backupEvery: e.target.value })}><option value="day">Every day</option><option value="week">Every week</option><option value="never">Only before imports and restores</option></select></label>
        <label class="lab">Keep the last<select class="inp" value={String(st.backupKeep)} onchange={(e) => change({ backupKeep: +e.target.value })}>{#each [7, 14, 30, 60, 90] as n}<option value={String(n)}>{n}</option>{/each}</select></label>
      </div>
      <p class="fine">Saved in the <span class="tag">backups</span> folder of Bookshelf’s data folder (<span class="tag">/data/backups</span> in Docker). Keep the data folder on a volume or a folder on the host, and they outlive the container.</p>
      <ul class="backups">
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
      </ul>
      <div class="row-btns"><button type="button" class="btn sm" onclick={backupNow}>Back up now</button>
        <label class="btn sm">Restore from a file<input type="file" accept=".bookshelf,.zip" hidden onchange={(e) => { upload = e.target.files[0] || null; e.target.value = ""; }}></label></div>
      {#if upload}
        <div class="rd-ask"><span>Replace your whole library with {upload.name}? What’s here now is backed up first.</span>
          <button type="button" class="btn sm danger solid" disabled={uploading} onclick={restoreFile}>{uploading ? "Restoring…" : "Restore it"}</button><button type="button" class="btn sm" disabled={uploading} onclick={() => (upload = null)}>Cancel</button></div>
      {/if}
    </section>

    <section hidden={tab !== "account"}>
      <h3>Updates</h3>
      {#if ui.update?.dev}
        <p class="fine">This is a development build of Bookshelf ({ui.update.version}), so it isn’t checked for updates.</p>
      {:else if ui.update?.newer}
        <p class="lede update-news"><b>Bookshelf {ui.update.latest} is out.</b> You have {ui.update.version}.</p>
        <div class="row-btns"><a class="btn sm" href={ui.update.url} target="_blank" rel="noopener">What’s new</a><a class="btn sm" href={UPDATING} target="_blank" rel="noopener">How to update</a></div>
      {:else if ui.update}
        <p class="fine">
          Bookshelf {ui.update.version}.
          {#if !st.checkUpdates}Checking is off; new versions are listed on the <a class="link" href={RELEASES} target="_blank" rel="noopener">releases page</a>.
          {:else if ui.update.problem && !ui.update.checkedAt}The last check didn’t work: {ui.update.problem}.
          {:else if ui.update.checkedAt}Up to date (checked {checked(ui.update.checkedAt)}).
          {:else}Not checked yet.{/if}
        </p>
      {/if}
      {#if !ui.update?.dev}
        <label class="toggle"><span><b>Check for updates</b><small>Bookshelf asks GitHub twice a day which version is the newest. Nothing about your library is sent.</small></span><input type="checkbox" checked={st.checkUpdates} onchange={(e) => toggleUpdates(e.target.checked)}></label>
      {/if}
    </section>
    <section hidden={tab !== "account"}>
      <h3>Account</h3>
      <p class="fine">Signed in as <b>{ui.user}</b>. To change the password, change <span class="tag">BOOKSHELF_ADMIN_PASSWORD</span> where Bookshelf runs (or run <span class="tag">bookshelf set-password</span> again) and restart it.</p>
      <div class="row-btns"><button type="button" class="btn sm" onclick={out}>Sign out</button></div>
    </section>
    {/if}
  </aside>
</div>
