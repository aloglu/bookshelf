<script>
  // A book's notes page (/<book>/notes): the book (cover, title, author) in a narrow column, and its notes in
  // the rest, newest first or in page order. A note has a page of its own too (/<book>/notes/<id>, its date
  // links there): the same page with that note alone, and a way to all of them. A note is a passage from the book, the owner's words, or both
  // (words may come before the passage and after it), with a place if they like: a page, a chapter or a
  // location. Each is public or the owner's alone, and belongs to the reading under way when it was written.
  // The owner writes at the top, edits a note where it stands, and has one ⋯ menu for what visitors see, the
  // link, Markdown and readings. Visitors see the public notes and nothing else.
  import { tick, untrack } from "svelte";
  import { ui, data, settings, entriesOf, entriesFor, addEntry, updateEntry, deleteEntry, restoreEntry, deleteReading, setPage, setNotesHidden } from "../lib/app.svelte.js";
  import { openBook, hooks } from "../engine/reader.js";
  import { fmt, plural } from "../engine/model.js";
  import { known, shortWhen, spanDays, whenText } from "../lib/dates.js";
  import { toast } from "../lib/toast.js";
  import { ICON, whereText, newest, byPlace, noteDate } from "../lib/notes.js";
  import NoteEditor from "./NoteEditor.svelte";

  let { active, onCorrect, onExport, onNote, onAll } = $props();
  // The shelf keeps one object per book and updates it in place, so the page works from a copy made on
  // every load: anything that reads it then follows what changed.
  const shelved = () => data.books.find((b) => b.key === ui.notesFor);
  const book = $derived.by(() => { const b = active ? data.books.find((x) => x.key === ui.notesFor) : null; return b ? { ...b } : null; });
  const st = $derived(settings());

  // Notes are fetched for the book showing, and again for whoever is looking after signing in or out.
  let entries = $state.raw([]), loadedFor = null, loaded = $state(false);
  // Leaving the page forgets what it showed, so coming back (after an import, a restore) fetches it anew.
  $effect(() => { if (!active) loadedFor = null; });
  $effect(() => {
    const b = book, owner = ui.owner, key = b && `${b.id}:${owner}`;
    if (!b || key === loadedFor) return;
    loadedFor = key; entries = []; loaded = false;
    entriesFor(b).then((list) => {
      if (loadedFor !== key) return;
      entries = list; loaded = true;
    }, (err) => toast(err.message));
  });
  // A note on its own page; one this reader can't see (gone, or not public) leaves the book's page in its place.
  const single = $derived(active ? ui.noteId : "");
  const one = $derived(single ? entries.find((e) => e.id === single) : null);
  $effect(() => { if (single && loaded && !one) untrack(() => onAll(shelved(), "replace")); });

  /* ---------- The readings ---------- */
  // Every reading of the book, oldest first; one in progress is the newest. With two or more, each is named by
  // its place ("Second reading", or "2nd reading" beside a note).
  const order = (r) => known(r.started) ? r.started.time : r.ended.precision !== "unknown" ? r.ended.time : "";
  const readings = $derived(!book ? [] : data.readings.filter((r) => r.bookId === book.id).sort((x, y) => (order(x) < order(y) ? -1 : order(x) > order(y) ? 1 : 0)));
  const newestFirst = $derived([...readings].reverse());
  const ORDINALS = ["First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth"];
  const SHORT = ["1st", "2nd", "3rd"];
  function nameOf(id) {
    const i = readings.findIndex((r) => r.id === id);
    return i < 0 || readings.length < 2 ? "" : ORDINALS[i] ? `${ORDINALS[i]} reading` : `Reading ${i + 1}`;
  }
  function shortName(id) {
    const i = readings.findIndex((r) => r.id === id);
    return i < 0 || readings.length < 2 ? "" : `${SHORT[i] || `${i + 1}th`} reading`;
  }
  function endedText(r) {
    const verb = r.outcome === "finished" ? "Finished" : "Set aside", when = whenText(r.ended);
    return !when ? `${verb}, date unknown` : r.ended.precision === "year" ? `${verb} in ${when}` : `${verb} ${when}`;
  }
  const cap = (s) => s && s[0].toUpperCase() + s.slice(1);
  const times = (n) => n === 1 ? "Read once" : n === 2 ? "Read twice" : `Read ${n} times`;
  const perDay = (pages, days) => pages && days > 1 ? `about ${plural(Math.max(1, Math.round(pages / days)), "page")} a day` : "";
  function lines(r) {
    const name = nameOf(r.id);
    if (r.outcome === "reading") {
      const days = spanDays(r.started, { precision: "instant", time: new Date().toISOString() }), page = r.progress?.at(-1)?.page || 0;
      return [cap([name, `started ${days === 1 ? "today" : shortWhen(r.started)}`].filter(Boolean).join(" · ")),
        days > 1 && page ? `${plural(days, "day")} · ${perDay(page, days)}` : ""].filter(Boolean);
    }
    const out = [cap([name, known(r.started) && `started ${shortWhen(r.started)}`].filter(Boolean).join(" · "))];
    if (known(r.started) && (r.ended.precision === "day" || r.ended.precision === "instant")) {
      const days = spanDays(r.started, r.ended), read = r.outcome === "finished" ? book.pages : r.progress?.at(-1)?.page;
      out.push([days === 1 ? "In one day" : plural(days, "day"), perDay(read, days)].filter(Boolean).join(" · "));
    }
    const stopped = r.progress?.at(-1)?.page;
    if (r.outcome === "set_aside" && stopped) out.push(book.pages ? `Stopped on page ${fmt(stopped)} of ${fmt(book.pages)}` : `Stopped on page ${fmt(stopped)}`);
    return out.filter(Boolean);
  }
  const summary = $derived.by(() => {
    if (!book) return ["", ""];
    const last = newestFirst.find((r) => r.outcome === "finished"), lastWhen = last && whenText(last.ended);
    if (book.reading) return ["Reading now", book.pages ? `page ${fmt(Math.min(book.page, book.pages))} of ${fmt(book.pages)}` : `page ${fmt(book.page)}`];
    if (book.readCount) return [times(book.readCount), lastWhen ? `last ${lastWhen}` : ""];
    return [readings.length ? "Set aside" : "Not read yet", ""];
  });
  let foldOpen = $state(false), asking = $state(null);
  async function page(e) {
    const b = book, n = Math.max(0, Math.min(b.pages || Infinity, Math.round(+e.currentTarget.value || 0)));
    try { await setPage(b, n); } catch (err) { toast(err.message); return; }
    if (b.pages && n >= b.pages) toast(`Finished “${b.title}”?`, { label: "Mark finished", run: () => hooks.finishReading(b) });
    else toast(`On page ${fmt(n)}`);
  }
  async function removeReading(r) {
    const tied = entries.filter((e) => e.readingId === r.id).length;
    try { await deleteReading(r); } catch (err) { toast(err.message); return; }
    asking = null;
    if (tied) { entries = await entriesOf(book); toast(`Reading deleted; its ${plural(tied, "note")} kept`); }
  }

  /* ---------- The notes ---------- */
  let byPage = $state(false), rfilter = $state(""), group = $state(false), menu = $state(false), editing = $state(null), editResume = $state(null);
  // Another book starts afresh.
  const bookKey = $derived(book && `${book.key}/${single}`);
  $effect(() => { bookKey; untrack(() => { byPage = false; rfilter = ""; group = false; menu = false; foldOpen = false; asking = null; editing = null; composeKey++; composeResume = null; composeReading = ""; }); });
  const placed = $derived(entries.filter((e) => e.where && (e.where.kind === "page" || e.where.kind === "loc")).length);
  const shown = $derived(entries.filter((e) => !ui.owner || !rfilter || e.readingId === rfilter).sort(byPage && placed > 1 ? byPlace : newest));
  const grouped = $derived(ui.owner && group && readings.length > 1);
  const loose = $derived(shown.filter((e) => !readings.some((r) => r.id === e.readingId)));
  const publicCount = $derived(entries.filter((e) => e.public).length);
  const visitorsRead = $derived(!!st?.notes && !book?.notesHidden);

  // Writing: the editor at the top is remounted (fresh) after a note is kept or thrown away.
  let composeKey = $state(0), composeResume = $state(null), composeReading = $state(""), composer = $state();
  const defaultReading = $derived(newestFirst[0]?.id ?? "");
  // Opened to write about a reading ("Add a note" after finishing).
  export async function begin(id) {
    composeReading = id; composeResume = null; composeKey++;
    await tick();
    composer?.begin();
  }
  async function create(input) {
    const b = book;
    entries = await addEntry(b, input);
    composeKey++; composeResume = null; composeReading = "";
    document.activeElement?.blur?.();
    toast(input.public && visitorsRead ? "Note saved; visitors can read it" : "Note saved");
  }
  function dropDraft(draft, written) {
    composeKey++; composeResume = null;
    if (written) toast("Draft discarded", { label: "Undo", run: () => { composeResume = draft; composeKey++; } });
  }
  async function save(e, input) {
    entries = await updateEntry(book, e, input);
    editing = null; editResume = null;
    toast("Note saved");
  }
  function dropEdit(e, draft, changed) {
    editing = null; editResume = null;
    if (changed) toast("Changes discarded", { label: "Undo", run: () => { editResume = draft; editing = e.id; } });
  }
  function edit(e) { editResume = null; editing = e.id; menu = false; }
  async function remove(e) {
    const b = book;
    try { entries = await deleteEntry(b, e); } catch (er) { toast(er.message); return; }
    if (editing === e.id) editing = null;
    toast("Note deleted", { label: "Undo", run: async () => {
      try { const list = await restoreEntry(b, e); if (book?.id === b.id) entries = list; } catch (er) { toast(er.message); }
    } });
  }
  async function copy(url) {
    try { await navigator.clipboard.writeText(url); toast("Link copied"); } catch { toast(url); }
  }
  const pageURL = () => `${location.origin}/${book.address}/notes`;
  // A note's date leads to its own page, and its own page to all of them: links, so they open in a new tab
  // too; a plain click goes there here.
  function follow(ev, go) {
    if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey || ev.button) return;
    ev.preventDefault();
    go();
  }
  async function toggleHidden() {
    menu = false;
    const hide = !book.notesHidden;
    try { await setNotesHidden(book, hide); } catch (err) { toast(err.message); return; }
    toast(hide ? "This book’s notes are hidden from visitors" : "Visitors can read this book’s public notes");
  }
  // The menu closes on a click outside it, or Esc.
  $effect(() => {
    if (!menu) return;
    const away = (e) => { if (!e.target.closest?.(".nb-bar .more")) menu = false; };
    const esc = (e) => { if (e.key === "Escape") { e.stopPropagation(); menu = false; } };
    document.addEventListener("pointerdown", away); addEventListener("keydown", esc, true);
    return () => { document.removeEventListener("pointerdown", away); removeEventListener("keydown", esc, true); };
  });
  const open = (e) => openBook(shelved(), e.currentTarget.closest(".nb-head").querySelector(".cv-btn img"));
</script>

{#snippet svg(name)}<svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON[name]}</svg>{/snippet}

{#snippet note(e)}
  {#if ui.owner && editing === e.id}
    <NoteEditor entry={e} resume={editResume} href="/{book.address}/notes/{e.id}" onopen={() => onNote(shelved(), e.id)} onsave={(input) => save(e, input)} ondiscard={(d, changed) => dropEdit(e, d, changed)} ondelete={() => remove(e)} />
  {:else}
    <li class="nb-e">
      {#if e.pre}<p class="txt">{e.pre}</p>{/if}
      {#if e.quote}<blockquote>{e.quote}</blockquote>{/if}
      {#if e.text}<p class="txt">{e.text}</p>{/if}
      <div class="nb-meta">
        {#if e.where}<span class="mi where">{@render svg("place")}{whereText(e.where)}</span>{/if}
        {#if single}<span class="mi">{@render svg("date")}{noteDate(e.createdAt)}</span>
        {:else}<a class="mi dt" href="/{book.address}/notes/{e.id}" onclick={(ev) => follow(ev, () => onNote(shelved(), e.id))}>{@render svg("date")}{noteDate(e.createdAt)}</a>{/if}
        {#if ui.owner && shortName(e.readingId)}<span class="mi">{@render svg("reading")}{shortName(e.readingId)}</span>{/if}
        {#if ui.owner && !e.public}<span class="mi">{@render svg("lock")}Only me</span>{/if}
        <span class="nb-tools">
          {#if e.public && visitorsRead}<button type="button" class="lnk" onclick={() => copy(`${pageURL()}/${e.id}`)}>{@render svg("link")}Copy link</button>{/if}
          {#if ui.owner}<button type="button" class="lnk" onclick={() => edit(e)}>{@render svg("pen")}Edit</button>{/if}
        </span>
      </div>
    </li>
  {/if}
{/snippet}

<section class="nb" id="notes" hidden={!book} aria-labelledby="nb-title">
  {#if book}
    <div class="nb2">
      <aside class="nb-side" class:open={foldOpen}>
        <div class="nb-head">
          <button type="button" class="cv-btn" aria-label="Details for {book.title}" onclick={open}><img src={book.face} alt=""><span class="cv-hint">Click for details</span></button>
          <div>
            <h2 id="nb-title"><button type="button" onclick={open}>{book.title}</button></h2>
            <p class="nb-by">{book.author}</p>
            <button type="button" class="tap-hint" onclick={open}>Tap for details {@render svg("chev")}</button>
          </div>
        </div>
        {#if ui.owner}
          <button type="button" class="nb-sum" aria-expanded={foldOpen} onclick={() => (foldOpen = !foldOpen)}>
            <span><b>{summary[0]}</b>{summary[1] ? ` · ${summary[1]}` : ""}</span>{@render svg("chevron")}
          </button>
          <div class="nb-fold">
            {#if readings.length}
              <ol class="nb-rd">
                {#each newestFirst as r (r.id)}
                  <li class="nb-r" class:now={r.outcome === "reading"} class:aside={r.outcome === "set_aside"}>
                    <b>{r.outcome === "reading" ? "Reading now" : endedText(r)}</b>
                    {#each lines(r) as line, i (i)}<span class="m">{line}</span>{/each}
                    {#if r.outcome === "reading"}
                      {#if book.pages}
                        {@const pct = Math.round(Math.min(book.page, book.pages) / book.pages * 100)}
                        <div class="meter"><div class="bar" role="img" aria-label="{pct} percent read"><i style="--v: {pct}%"></i></div></div>
                      {/if}
                      <label class="pg">On page <input type="number" min="0" max={book.pages || undefined} value={Math.min(book.page, book.pages || Infinity)} inputmode="numeric" onchange={page}>{#if book.pages} of {fmt(book.pages)}{/if}</label>
                      <div class="r-tools"><button type="button" class="lnk" onclick={() => hooks.finishReading(book)}>Finished</button><button type="button" class="lnk" onclick={() => hooks.setAside(book)}>Set aside</button></div>
                    {:else if asking === r.id}
                      <div class="r-ask"><span>Delete this reading?</span><button type="button" class="btn sm danger" onclick={() => removeReading(r)}>Delete</button><button type="button" class="btn sm" onclick={() => (asking = null)}>Keep it</button></div>
                    {:else}
                      <div class="r-tools"><button type="button" class="lnk" onclick={() => onCorrect(book, r)}>Edit</button><button type="button" class="lnk" onclick={() => (asking = r.id)}>Delete</button></div>
                    {/if}
                  </li>
                {/each}
              </ol>
            {:else}
              <p class="none">Not read yet. Start a reading to keep its dates and the pages you reach, or mark it as read if you read it before.</p>
            {/if}
            {#if !book.reading}
              <div class="acts"><button type="button" class="btn sm" onclick={() => hooks.startReading(book)}>Reading now</button><button type="button" class="btn sm" onclick={() => hooks.markRead(book)}>{book.readCount ? "Mark as read again" : "Mark as read"}</button></div>
            {/if}
          </div>
        {/if}
      </aside>

      <div class="nb-main">
        {#if single}
          {#if one}
            <ol class="nb-list one">{@render note(one)}</ol>
            <a class="lnk nb-all" href="/{book.address}/notes" onclick={(ev) => follow(ev, () => onAll(shelved()))}>{entries.length > 1 ? `All ${entries.length} notes on this book` : "This book’s notes page"}{@render svg("chev")}</a>
          {/if}
        {:else}
          {#if ui.owner}
            {#key composeKey}
              <NoteEditor bind:this={composer} resume={composeResume} readingId={composeReading || defaultReading} startsPublic={!!st?.startsPublic} onsave={create} ondiscard={dropDraft} />
            {/key}
          {/if}
          <div class="nb-bar">
            <h3>{plural(shown.length, "note")}</h3>
            {#if ui.owner && rfilter}
              <button type="button" class="btn sm" aria-pressed="true" aria-label="Showing the {shortName(rfilter)} only. Show all." onclick={() => (rfilter = "")}>{shortName(rfilter)}{@render svg("x")}</button>
            {/if}
            {#if placed > 1}
              <div class="seg" role="group" aria-label="Order">
                <button type="button" aria-pressed={!byPage} onclick={() => (byPage = false)}>Newest</button><button type="button" aria-pressed={byPage} onclick={() => (byPage = true)}>By page</button>
              </div>
            {/if}
            {#if ui.owner}
              <span class="more">
                <button type="button" class="btn icon more-btn" aria-label="More" aria-haspopup="menu" aria-expanded={menu} onclick={() => (menu = !menu)}>{@render svg("more")}</button>
                {#if menu}
                  <div class="menu" role="menu" aria-label="More">
                    <p class="info">{!st?.notes ? "Notes are switched off for visitors in Settings." : book.notesHidden ? `Hidden from visitors. ${publicCount} of ${entries.length} are marked public.` : `Visitors can read ${publicCount} of ${entries.length}.`}</p>
                    {#if st?.notes}<button type="button" role="menuitem" onclick={toggleHidden}>{@render svg(book.notesHidden ? "eye" : "lock")}{book.notesHidden ? "Show to visitors" : "Hide from visitors"}</button>{/if}
                    <button type="button" role="menuitem" disabled={!visitorsRead} onclick={() => { menu = false; copy(pageURL()); }}>{@render svg("link")}Copy link</button>
                    <button type="button" role="menuitem" onclick={() => { menu = false; onExport(book); }}>{@render svg("down")}Download Markdown</button>
                    {#if readings.length > 1}
                      <p class="info sep">Readings</p>
                      {#each [["", "All readings"], ...readings.map((r) => [r.id, shortName(r.id)])] as [id, label] (id)}
                        <button type="button" role="menuitemradio" aria-checked={rfilter === id} onclick={() => { rfilter = id; menu = false; }}>{#if rfilter === id}{@render svg("check")}{:else}<span class="ph"></span>{/if}<span>{label}</span></button>
                      {/each}
                      <button type="button" role="menuitemcheckbox" aria-checked={group} onclick={() => { group = !group; menu = false; }}>{#if group}{@render svg("check")}{:else}<span class="ph"></span>{/if}<span>Group by reading</span></button>
                    {/if}
                  </div>
                {/if}
              </span>
            {/if}
          </div>
          {#if grouped}
            {#each newestFirst as r (r.id)}
              {@const list = shown.filter((e) => e.readingId === r.id)}
              {#if list.length}
                <section class="rd"><h4 class="rd-cap">{nameOf(r.id)} · {r.outcome === "reading" ? "under way" : endedText(r).toLowerCase()}</h4><ol class="nb-list">{#each list as e (e.id)}{@render note(e)}{/each}</ol></section>
              {/if}
            {/each}
            {#if loose.length}
              <section class="rd"><h4 class="rd-cap">Not from a reading</h4><ol class="nb-list">{#each loose as e (e.id)}{@render note(e)}{/each}</ol></section>
            {/if}
          {:else}
            <ol class="nb-list">{#each shown as e (e.id)}{@render note(e)}{/each}</ol>
          {/if}
          {#if ui.owner && !entries.length}<p class="nb-empty">Nothing kept yet. A line you liked, a thought, or both.</p>{/if}
        {/if}
      </div>
    </div>
  {/if}
</section>
