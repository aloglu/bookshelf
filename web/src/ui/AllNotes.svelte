<script>
  // All notes (/notes): every book's notes in one list, newest first, each led by its book, by month. A note
  // is written on its book's page (it always belongs to a book), but the owner can edit one here, in the same
  // editor. Each note's date leads to its own page. The page is there while it is turned on in Settings for
  // visitors and always there for the owner, who sees every note or, while visitors can open the page,
  // exactly what they see.
  import { untrack } from "svelte";
  import { ui, data, settings, allNotes, updateEntry, deleteEntry, restoreEntry } from "../lib/app.svelte.js";
  import { plural } from "../engine/model.js";
  import { toast } from "../lib/toast.js";
  import { ICON, whereText, noteDate, monthOf } from "../lib/notes.js";
  import NoteEditor from "./NoteEditor.svelte";

  let { active, onBook, onNote, onExport } = $props();
  const st = $derived(settings());
  const open = $derived(!!st?.notes && !!st?.timeline);
  $effect(() => { if (!open) asVisitors = false; });

  let asVisitors = $state(false), list = $state.raw([]), total = $state(0), more = $state(false), busy = $state(false), menu = $state(false), shownFor = null;
  // Only the latest answer is shown: switching between Everything and As visitors see it while a page is still
  // coming can bring the answers back out of order.
  let asked = 0;
  async function load(before, count = 0) {
    const n = ++asked;
    busy = true;
    try {
      const r = await allNotes({ asVisitors: ui.owner && asVisitors, before: before?.createdAt, beforeId: before?.id, count });
      if (n !== asked) return;
      list = before ? [...list, ...r.entries] : r.entries; total = r.total; more = r.more;
    } catch (err) { if (n === asked) toast(err.message); }
    if (n === asked) busy = false;
  }
  // Opening the page always fetches it anew (notes may have been written since). Coming back to it keeps what
  // it showed meanwhile, and as many notes as it had (older ones included), so it stands as it was left.
  $effect(() => {
    if (!active) return;
    const who = `${ui.owner}:${asVisitors}`;
    untrack(() => {
      if (who !== shownFor) { shownFor = who; list = []; editing = null; load(); }
      else load(null, list.length);
    });
  });

  const bookOf = (id) => data.books.find((b) => b.id === id);
  // Month headings between the notes.
  const rows = $derived.by(() => {
    let cur = "";
    return list.filter((e) => bookOf(e.bookId)).map((e) => { const m = monthOf(e.createdAt), head = m !== cur ? m : ""; cur = m; return { e, b: bookOf(e.bookId), head }; });
  });
  async function copy(url) {
    try { await navigator.clipboard.writeText(url); toast("Link copied"); } catch { toast(url); }
  }
  $effect(() => {
    if (!menu) return;
    const away = (e) => { if (!e.target.closest?.(".tl .more")) menu = false; };
    const esc = (e) => { if (e.key === "Escape") { e.stopPropagation(); menu = false; } };
    document.addEventListener("pointerdown", away); addEventListener("keydown", esc, true);
    return () => { document.removeEventListener("pointerdown", away); removeEventListener("keydown", esc, true); };
  });
  const shownToVisitors = (e, b) => e.public && !!st?.notes && !b.notesHidden;
  const noteURL = (e, b) => `/${b.address}/notes/${e.id}`;
  // A note's date leads to its own page: a link, so it opens in a new tab too; a plain click goes there here.
  function follow(ev, e, b) {
    if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey || ev.button) return;
    ev.preventDefault();
    onNote(b, e.id);
  }

  // Editing a note here: the book's own editor, saved and deleted as on the book's page (with Undo).
  let editing = $state(null), editResume = $state(null);
  const put = (id, e) => { list = list.map((x) => (x.id === id ? e : x)); };
  async function save(e, b, input) {
    const fresh = (await updateEntry(b, e, input)).find((x) => x.id === e.id);
    if (fresh) put(e.id, { ...fresh, bookId: e.bookId });
    editing = null; editResume = null;
    toast("Note saved");
  }
  function dropEdit(e, draft, changed) {
    editing = null; editResume = null;
    if (changed) toast("Changes discarded", { label: "Undo", run: () => { editResume = draft; editing = e.id; } });
  }
  async function remove(e, b) {
    const at = list.findIndex((x) => x.id === e.id);
    try { await deleteEntry(b, e); } catch (er) { toast(er.message); return; }
    editing = null;
    list = list.filter((x) => x.id !== e.id); total--;
    toast("Note deleted", { label: "Undo", run: async () => {
      try {
        const back = (await restoreEntry(b, e)).find((x) => x.createdAt === e.createdAt && x.text === e.text && x.quote === e.quote);
        if (back) { list = [...list.slice(0, at), { ...back, bookId: e.bookId }, ...list.slice(at)]; total++; }
      } catch (er) { toast(er.message); }
    } });
  }
</script>

{#snippet svg(name)}<svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON[name]}</svg>{/snippet}

<section class="tl" id="allnotes" hidden={!active} aria-labelledby="tl-title">
  {#if active}
    <div class="tl-head"><h2 id="tl-title">Notes</h2><p>What I’ve been keeping from my books, newest first.</p></div>
    {#if ui.owner}
      <div class="nb-bar">
        <h3>{plural(total, "note")}</h3>
        {#if open}
          <div class="seg" role="group" aria-label="View">
            <button type="button" aria-pressed={!asVisitors} onclick={() => (asVisitors = false)}>Everything</button><button type="button" aria-pressed={asVisitors} onclick={() => (asVisitors = true)}>As visitors see it</button>
          </div>
        {/if}
        <span class="more">
          <button type="button" class="btn icon more-btn" aria-label="More" aria-haspopup="menu" aria-expanded={menu} onclick={() => (menu = !menu)}>{@render svg("more")}</button>
          {#if menu}
            <div class="menu" role="menu" aria-label="More">
              <p class="info">{open ? "Visitors can open this page too. It can be turned off in Settings." : "Only you can open this page. It can be opened to visitors in Settings."}</p>
              {#if open}<button type="button" role="menuitem" onclick={() => { menu = false; copy(`${location.origin}/notes`); }}>{@render svg("link")}Copy link</button>{/if}
              <button type="button" role="menuitem" onclick={() => { menu = false; onExport(null); }}>{@render svg("down")}Download Markdown</button>
            </div>
          {/if}
        </span>
      </div>
    {/if}
    {#each rows as { e, b, head } (e.id)}
      {#if head}<h3 class="tl-month">{head}</h3>{/if}
      <article class="tl-e">
        <button type="button" class="cv-btn" aria-label={b.title} onclick={() => onBook(b)}><img src={b.face} alt=""></button>
        <div>
          <button type="button" class="bk" onclick={() => onBook(b)}><b>{b.title}</b> · {b.author}</button>
          <ul class="nb-list">
          {#if ui.owner && !asVisitors && editing === e.id}
            <NoteEditor entry={e} resume={editResume} href={noteURL(e, b)} onopen={() => onNote(b, e.id)} onsave={(input) => save(e, b, input)} ondiscard={(d, changed) => dropEdit(e, d, changed)} ondelete={() => remove(e, b)} />
          {:else}
            <li class="nb-e">
              {#if e.pre}<p class="txt">{e.pre}</p>{/if}
              {#if e.quote}<blockquote>{e.quote}</blockquote>{/if}
              {#if e.text}<p class="txt">{e.text}</p>{/if}
              <div class="nb-meta">
                {#if e.where}<span class="mi where">{@render svg("place")}{whereText(e.where)}</span>{/if}
                <a class="mi dt" href={noteURL(e, b)} onclick={(ev) => follow(ev, e, b)}>{@render svg("date")}{noteDate(e.createdAt)}</a>
                {#if ui.owner && !asVisitors && !e.public}<span class="mi">{@render svg("lock")}Only me</span>{/if}
                <span class="nb-tools">
                  {#if shownToVisitors(e, b)}<button type="button" class="lnk" onclick={() => copy(location.origin + noteURL(e, b))}>{@render svg("link")}Copy link</button>{/if}
                  {#if ui.owner && !asVisitors}<button type="button" class="lnk" onclick={() => { editResume = null; editing = e.id; }}>{@render svg("pen")}Edit</button>{/if}
                </span>
              </div>
            </li>
          {/if}
          </ul>
        </div>
      </article>
    {:else}
      {#if !busy}<p class="nb-empty">{ui.owner && !asVisitors ? "Nothing kept yet. Notes are written on a book’s own page." : "Nothing public yet."}</p>{/if}
    {/each}
    {#if more}<div class="tl-more"><button type="button" class="btn" disabled={busy} onclick={() => load(list.at(-1))}>Show older</button></div>{/if}
  {/if}
</section>
