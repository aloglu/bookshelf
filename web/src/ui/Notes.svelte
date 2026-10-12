<script>
  // A book's notes page (/<book>/notes): the book, then its notes and highlights, any number of each, newest
  // first or in page order. A note is the owner's own words; a highlight is a passage as printed, its page
  // if there is one, and a thought on it if they like. Each is public or the owner's alone, and can name the
  // reading it came from. Beside them, for the owner only, the book's readings. Visitors see the book and
  // its public entries.
  import { tick } from "svelte";
  import { ui, data, entriesOf, addEntry, updateEntry, deleteEntry, restoreEntry, deleteReading, setPage } from "../lib/app.svelte.js";
  import { openBook, hooks } from "../engine/reader.js";
  import { BINDING_NAMES, bindingOf, fmt, plural, touch } from "../engine/model.js";
  import { known, ms, shortDate, shortWhen, spanDays, whenText } from "../lib/dates.js";
  import { toast } from "../lib/toast.js";

  let { active, onCorrect } = $props();
  // The shelf keeps one object per book and updates it in place, so the page works from a copy made on
  // every load: anything that reads it then follows what changed.
  const shelved = () => data.books.find((b) => b.key === ui.notesFor);
  const book = $derived.by(() => { const b = active ? data.books.find((x) => x.key === ui.notesFor) : null; return b ? { ...b } : null; });

  // Entries are fetched for the book showing, and again for whoever is looking after signing in or out.
  let entries = $state.raw([]), loadedFor = null;
  $effect(() => {
    const b = book, owner = ui.owner, key = b && `${b.id}:${owner}`;
    if (!b || key === loadedFor) return;
    loadedFor = key; entries = [];
    entriesOf(b).then((list) => { if (loadedFor === key) entries = list; }, (err) => toast(err.message));
  });

  /* ---------- The readings ---------- */
  // Every reading of the book, oldest first (as the server orders them); one in progress is the newest. With
  // two or more, each is named by its place ("Second reading").
  const order = (r) => known(r.started) ? r.started.time : r.ended.precision !== "unknown" ? r.ended.time : "";
  const readings = $derived(!book ? [] : data.readings.filter((r) => r.bookId === book.id).sort((x, y) => (order(x) < order(y) ? -1 : order(x) > order(y) ? 1 : 0)));
  const newestFirst = $derived([...readings].reverse());
  const ORDINALS = ["First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth"];
  function nameOf(id) {
    const i = readings.findIndex((r) => r.id === id);
    return i < 0 || readings.length < 2 ? "" : ORDINALS[i] ? `${ORDINALS[i]} reading` : `Reading ${i + 1}`;
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
    if (known(r.started) && r.ended.precision === "day" || known(r.started) && r.ended.precision === "instant") {
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
  $effect(() => { book?.key; foldOpen = false; asking = null; });
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
    if (tied) { entries = await entriesOf(book); toast(`Reading deleted; ${kinds(entries.filter((e) => !e.readingId))} kept`); }
  }

  /* ---------- The entries ---------- */
  let filter = $state("all"), byPage = $state(false);
  $effect(() => { book?.key; filter = "all"; byPage = false; });
  function kinds(list) {
    const n = list.filter((e) => e.kind === "note").length, h = list.length - n;
    return [n && plural(n, "note"), h && plural(h, "highlight")].filter(Boolean).join(" and ");
  }
  const kindsHere = $derived(new Set(entries.map((e) => e.kind)).size);
  const paged = $derived(entries.filter((e) => e.page != null).length);
  // In page order, entries without a page follow the rest, newest first.
  const newest = (x, y) => Date.parse(y.createdAt) - Date.parse(x.createdAt);
  const shown = $derived(entries.filter((e) => kindsHere < 2 || filter === "all" || e.kind === filter)
    .sort(byPage && paged > 1 ? (x, y) => (x.page ?? Infinity) - (y.page ?? Infinity) || newest(x, y) : newest));

  // Writing: one form writes a new entry at the top, or edits one in its place.
  let form = $state(), quoteEl = $state(), textEl = $state();
  let kind = $state("note"), quote = $state(""), text = $state(""), pageNo = $state(""), readingId = $state(""), isPublic = $state(false);
  let editing = $state(null), open = $state(false), err = $state(""), busy = $state(false);
  // A new entry belongs to the reading under way, or else the last one; the owner can choose another or none.
  function reset(o = {}) {
    kind = o.kind || "note"; quote = o.quote || ""; text = o.text || ""; pageNo = o.page != null ? String(o.page) : "";
    readingId = o.readingId ?? newestFirst[0]?.id ?? ""; isPublic = !!o.public;
    editing = o.id || null; open = !!o.id; err = "";
  }
  $effect(() => { book?.key; reset(); });
  // Opened to write about a reading ("Add a note" after finishing).
  export async function begin(id) {
    reset({ readingId: id }); open = true;
    await tick();
    if (!touch()) textEl?.focus({ preventScroll: true });
  }
  async function choose(k) {
    kind = k; open = true; err = "";
    await tick();
    if (!touch()) (k === "highlight" ? quoteEl : textEl)?.focus();
  }
  async function edit(e) {
    reset(e);
    await tick();
    if (!touch()) (e.kind === "highlight" ? quoteEl : textEl)?.focus({ preventScroll: true });
  }
  function cancel() { reset(); document.activeElement?.blur?.(); }
  async function save(ev) {
    ev.preventDefault();
    const hl = kind === "highlight", q = quote.trim(), t = text.trim(), raw = pageNo.trim();
    err = hl && !q ? "Copy in the passage first." : !hl && !t ? "Write something first." : "";
    if (!err && hl && raw && (!/^\d+$/.test(raw) || +raw < 1 || (book.pages && +raw > book.pages))) err = book.pages ? `Pages run from 1 to ${fmt(book.pages)}.` : "Enter a page number.";
    if (err) return;
    const input = { kind, text: t, quote: hl ? q : "", page: hl && raw ? +raw : null, readingId, public: isPublic };
    const b = book, was = editing;
    busy = true;
    try {
      entries = was ? await updateEntry(b, { id: was }, input) : await addEntry(b, input);
    } catch (e) {
      err = e.message; busy = false;
      return;
    }
    busy = false; reset();
    document.activeElement?.blur?.();
    const what = hl ? "Highlight" : "Note";
    toast(!was && input.public ? `${what} saved; visitors can read it` : `${what} saved`);
  }
  async function remove(e) {
    const b = book;
    try { entries = await deleteEntry(b, e); } catch (er) { toast(er.message); return; }
    toast(`${e.kind === "highlight" ? "Highlight" : "Note"} deleted`, { label: "Undo", run: async () => {
      try { const list = await restoreEntry(b, e); if (book === b) entries = list; } catch (er) { toast(er.message); }
    } });
  }
  // Fields grow with what's written in them.
  $effect(() => { quote; text; kind; tick().then(() => form?.querySelectorAll("textarea").forEach((el) => { el.style.height = "auto"; el.style.height = el.scrollHeight + "px"; })); });

  const facts = $derived(!book ? "" : [book.translator && `Translated by ${book.translator}`, book.publisher, book.published, book.pages && plural(book.pages, "page")].filter(Boolean).join(" · "));
  const kick = $derived(!book ? "" : [BINDING_NAMES[bindingOf(book)], ui.owner && book.hidden && "Hidden from the website",
    book.reading ? (ui.owner ? "Reading now" : "Currently reading") : ui.owner && book.readCount ? times(book.readCount) : ""].filter(Boolean).join(" · "));
  const I = { lock: `<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>`, eye: `<path d="M2.5 12C3.5 10.2 7 6 12 6s8.5 4.2 9.5 6c-1 1.8-4.5 6-9.5 6s-8.5-4.2-9.5-6Z"/><circle cx="12" cy="12" r="3"/>` };
</script>

{#snippet composer()}
  <form class="nb-compose" class:open class:hl={kind === "highlight"} novalidate bind:this={form} onsubmit={save} onfocusin={() => (open = true)} oninput={() => (open = true)}>
    <div class="seg" role="group" aria-label="Kind">
      <button type="button" aria-pressed={kind === "note"} onclick={() => choose("note")}>Note</button>
      <button type="button" aria-pressed={kind === "highlight"} onclick={() => choose("highlight")}>Highlight</button>
    </div>
    {#if kind === "highlight"}<textarea name="quote" rows="1" placeholder="The passage, as printed" aria-label="Passage" bind:value={quote} bind:this={quoteEl}></textarea>{/if}
    <textarea name="text" rows="1" placeholder={kind === "highlight" ? "A thought on it, if you like" : "Write a note"} aria-label={kind === "highlight" ? "Your thought" : "Note"} bind:value={text} bind:this={textEl}></textarea>
    <div class="nb-more">
      {#if kind === "highlight"}<label>Page<input name="page" inputmode="numeric" autocomplete="off" bind:value={pageNo}></label>{/if}
      {#if readings.length}
        <label>Reading<select name="reading" bind:value={readingId}>
          {#each newestFirst as r (r.id)}<option value={r.id}>{r.outcome === "reading" ? "This reading" : endedText(r)}</option>{/each}
          <option value="">None in particular</option>
        </select></label>
      {/if}
      <button type="button" class="nb-vis" aria-pressed={isPublic} title={isPublic ? "Visitors can read it on this page" : "Only you can see it"} onclick={() => (isPublic = !isPublic)}>
        <svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html isPublic ? I.eye : I.lock}</svg>{isPublic ? "Public" : "Only you"}
      </button>
      <span class="nb-btns"><button type="button" class="btn sm" onclick={cancel}>Cancel</button><button type="submit" class="btn sm primary" disabled={busy}>Save</button></span>
      <p class="nb-err" role="alert">{err}</p>
    </div>
  </form>
{/snippet}

<section class="nb" id="notes" hidden={!book} aria-labelledby="nb-title">
  {#if book}
    <header class="nb-head">
      <button type="button" class="nb-cover" aria-label="Open the book" onclick={(e) => openBook(shelved(), e.currentTarget.querySelector("img"))}><img src={book.thumb} alt=""></button>
      <div>
        <p class="nb-kick">{kick}</p>
        <h2 id="nb-title">{book.title}</h2>
        <p class="nb-by">{book.author}</p>
        <p class="nb-facts">{facts}</p>
      </div>
    </header>
    <div class="nb-cols">
      <div class="nb-main">
        <div class="nb-bar">
          <h3>{entries.length ? kinds(entries) : "Notes and highlights"}</h3>
          {#if kindsHere > 1}
            <div class="seg" role="group" aria-label="Show">
              {#each [["all", "All"], ["note", "Notes"], ["highlight", "Highlights"]] as [v, label] (v)}<button type="button" aria-pressed={filter === v} onclick={() => (filter = v)}>{label}</button>{/each}
            </div>
          {/if}
          {#if paged > 1}
            <div class="seg" role="group" aria-label="Order">
              <button type="button" aria-pressed={!byPage} onclick={() => (byPage = false)}>Newest</button><button type="button" aria-pressed={byPage} onclick={() => (byPage = true)}>By page</button>
            </div>
          {/if}
        </div>
        {#if ui.owner && !editing}{@render composer()}{/if}
        <ol class="nb-list">
          {#each shown as e (e.id)}
            <li class="nb-e">
              {#if editing === e.id}
                {@render composer()}
              {:else}
                {#if e.kind === "highlight"}<blockquote>{e.quote}</blockquote>{/if}
                {#if e.text}<p class="txt">{e.text}</p>{/if}
                <div class="nb-meta">
                  <span>{[e.page != null && `Page ${fmt(e.page)}`, ui.owner && nameOf(e.readingId), shortDate(Date.parse(e.createdAt))].filter(Boolean).join(" · ")}</span>
                  {#if ui.owner}
                    {#if e.public}<span class="pub"><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html I.eye}</svg>Public</span>{/if}
                    <span class="nb-tools"><button type="button" onclick={() => edit(e)}>Edit</button><button type="button" onclick={() => remove(e)}>Delete</button></span>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ol>
        {#if ui.owner && !entries.length}<p class="nb-empty">Nothing kept yet. A note can be a line or a page; a highlight keeps a passage you liked, and its page.</p>{/if}
      </div>
      {#if ui.owner}
        <aside class="nb-side" class:open={foldOpen} aria-labelledby="nb-rd-h">
          <h3 id="nb-rd-h">Readings</h3>
          <button type="button" class="nb-sum" aria-expanded={foldOpen} onclick={() => (foldOpen = !foldOpen)}>
            <span><b>{summary[0]}</b>{summary[1] ? ` · ${summary[1]}` : ""}</span><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
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
                      <div class="r-tools"><button type="button" onclick={() => hooks.finishReading(book)}>Finished</button><button type="button" onclick={() => hooks.setAside(book)}>Set aside</button></div>
                    {:else if asking === r.id}
                      <div class="r-ask"><span>Delete this reading?</span><button type="button" class="btn sm danger" onclick={() => removeReading(r)}>Delete</button><button type="button" class="btn sm" onclick={() => (asking = null)}>Keep it</button></div>
                    {:else}
                      <div class="r-tools"><button type="button" onclick={() => onCorrect(book, r)}>Edit</button><button type="button" onclick={() => (asking = r.id)}>Delete</button></div>
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
        </aside>
      {/if}
    </div>
  {/if}
</section>
