<script>
  // The header: the library's name, the tools (search, sort, day or evening, stats, and the owner's), what
  // is being read now, and the owner's links elsewhere.
  import { ui, data, setQuery, setSort, signOut, setSelecting } from "../lib/app.svelte.js";
  import { motionOK, root } from "../engine/model.js";
  import { toast } from "../lib/toast.js";

  let { setView, openShelved, onSearch, onSignIn, onAdd, onSettings, onMenu } = $props();

  const NETS = { goodreads: "Goodreads", storygraph: "The StoryGraph", instagram: "Instagram" };
  const links = $derived(Object.keys(NETS).filter((k) => data.library?.settings.social?.[k]).map((k) => ({ name: NETS[k], href: data.library.settings.social[k] })));

  // One book in hand is named with its author; two are both named; more are counted. Their covers fan out.
  const now = $derived(data.books.filter((b) => b.reading).sort((a, b) => b.readingSince - a.readingSince));
  let menuOpen = $state(false), menuPos = $state({ top: 0, left: 0 }), line, menu;
  export function closeMenu() { if (!menuOpen) return false; menuOpen = false; return true; }
  const pct = (b) => (b.pages ? Math.round(Math.min(b.page || 0, b.pages) / b.pages * 100) : 0);
  function clickNow(e) {
    if (now.length === 1) return openShelved(now[0]);
    if (menuOpen) { menuOpen = false; return; }
    const r = line.getBoundingClientRect();
    menuOpen = true;
    requestAnimationFrame(() => {
      menuPos = { top: Math.round(r.bottom + 6), left: Math.round(Math.max(12, Math.min(r.left, innerWidth - menu.offsetWidth - 12))) };
      if (e.detail === 0) menu.querySelector("button")?.focus();   // opened from the keyboard
    });
  }
  function menuKeys(e) {
    const items = [...menu.querySelectorAll("button")], i = items.indexOf(document.activeElement);
    if (e.key === "Escape") { e.stopPropagation(); e.preventDefault(); menuOpen = false; line.focus(); }
    if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); items[(i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus(); }
  }
  $effect(() => {
    if (!menuOpen) return;
    const away = (e) => { if (!e.target.closest?.("#nr-menu, #nowreading")) menuOpen = false; };
    const scrolled = () => { menuOpen = false; };
    document.addEventListener("pointerdown", away);
    addEventListener("scroll", scrolled, { passive: true });
    return () => { document.removeEventListener("pointerdown", away); removeEventListener("scroll", scrolled); };
  });

  // Search folds into an icon; it opens on click or tap, and Esc clears it, then folds it away.
  let q = $state("");
  function typed() { setQuery(q); onSearch(); }
  function searchKeys(e) {
    if (e.key === "Enter" && !q) { e.preventDefault(); e.target.blur(); return; }   // nothing to look for: fold it away
    if (e.key !== "Escape") return;
    e.stopPropagation(); e.preventDefault();
    if (q) { q = ""; typed(); } else e.target.blur();
  }
  // Day or evening for this visit; the owner's setting decides how the library opens.
  function flip() {
    const dark = root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    root.dataset.theme = dark ? "light" : "dark";
  }
  function home(e) { e.preventDefault(); setView("shelf"); scrollTo({ top: 0, behavior: motionOK() ? "smooth" : "instant" }); }
  async function out() { try { await signOut(); } catch (err) { toast(err.message); } }
</script>

<header>
  <div class="brand">
    <h1><a href="/" class="home" id="site-title" onclick={home}>{ui.name}</a></h1>
  </div>
  <div class="tools">
    <div class="search" class:has-query={!!q} id="search">
      <label class="field" title="Search">
        <svg class="i" viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.6-3.6"/></svg>
        <input id="q" type="search" placeholder="Title, author, publisher, year" autocomplete="off" aria-label="Search books" bind:value={q} oninput={typed} onkeydown={searchKeys}>
      </label>
    </div>
    <div class="seg sort-seg" id="sort" role="group" aria-label="Sort by">
      {#each [["author", "Author"], ["title", "Title"], ["year", "Year"]] as [k, name]}
        <button type="button" aria-pressed={ui.sort === k} onclick={() => ui.sort !== k && setSort(k)}>{name}</button>
      {/each}
    </div>
    <button type="button" class="btn icon" id="mode" aria-label="Switch between day and evening" onclick={flip}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z"/></svg></button>
    <button type="button" class="btn icon stats-btn" id="stats-btn" aria-label="Statistics" title="Statistics" aria-pressed={ui.view === "stats"} onclick={() => setView(ui.view === "stats" ? "shelf" : "stats")}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 20V11M12 20V5M19 20v-6M3 20h18"/></svg></button>
    <button type="button" class="btn owner-only" id="select" aria-pressed={ui.selecting} onclick={() => setSelecting(!ui.selecting)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="m8.5 12 2.5 2.5 4.5-5"/></svg>Select</button>
    <button type="button" class="btn primary owner-only" id="add" onclick={onAdd}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>Add book</button>
    <button type="button" class="btn icon owner-only" id="settings-btn" aria-label="Settings" title="Settings" onclick={onSettings}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/></svg></button>
    <button type="button" class="btn icon owner-only" id="signout-btn" aria-label="Sign out" title="Sign out" onclick={out}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10"/></svg></button>
    <button type="button" class="btn icon phone-only" id="menu-btn" aria-label="Menu" title="Menu" aria-haspopup="dialog" onclick={onMenu}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16"/></svg></button>
    <button type="button" class="btn icon visitor-only" id="signin-btn" aria-label="Sign in" title="Sign in" onclick={onSignIn}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M10 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4M14 16l4-4-4-4M18 12H8"/></svg></button>
  </div>
  <button type="button" class="nowreading" id="nowreading" aria-haspopup="true" aria-expanded={menuOpen} hidden={!now.length} bind:this={line} onclick={clickNow}>
    <span class="fan">{#each now.slice(0, 3) as b (b.key)}<img alt="" src={b.thumb}>{/each}</span>
    <span>Now reading <b>{now[0]?.title}</b>{#if now.length === 1}{" by "}<span class="au">{now[0].author}</span>{:else if now.length === 2}{" and "}<b>{now[1].title}</b>{:else if now.length > 2}{` and ${now.length - 1} more`}{/if}</span>
  </button>
  <div class="nr-menu" id="nr-menu" role="menu" tabindex="-1" aria-label="Now reading" hidden={!menuOpen} bind:this={menu} style:top="{menuPos.top}px" style:left="{menuPos.left}px" onkeydown={menuKeys}>
    {#each now as b (b.key)}
      <button type="button" role="menuitem" onclick={() => { menuOpen = false; openShelved(b); }}><img alt="" src={b.thumb}><span><b>{b.title}</b><small>{b.author} · {pct(b)}%</small></span></button>
    {/each}
  </div>
  <p class="elsewhere" id="elsewhere" hidden={!links.length}>
    {#each links as l}<a href={l.href} target="_blank" rel="me noopener">{l.name}</a>{/each}
  </p>
</header>
