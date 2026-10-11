<script>
  // A book's readings, newest first: the one in progress, then each finished or set-aside reading with its
  // dates, how long it took and (once finished) its note. The owner can correct or delete each, and write
  // the notes. Visitors see only the notes the owner made public.
  import { tick } from "svelte";
  import { ui, data, deleteReading, saveNote } from "../lib/app.svelte.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast } from "../lib/toast.js";
  import { DAY, known, longDate, ms, whenText } from "../lib/dates.js";
  import { fmt, plural } from "../engine/model.js";

  let { onCorrect } = $props();
  let wrap, list = $state(), key = $state(null), writing = $state(null), asking = $state(null);
  let text = $state(""), isPublic = $state(false), busy = $state(false);
  const book = $derived(data.books.find((b) => b.key === key));
  const times = (n) => n === 1 ? "Read once" : n === 2 ? "Read twice" : `Read ${n} times`;
  const end = (r) => ms(r.ended) ?? -Infinity;
  const items = $derived(!book ? [] : ui.owner
    ? [...(book.reading ? [{ now: true, id: "now" }] : []), ...[...book.history].sort((x, y) => end(y) - end(x))]
    : [...(book.notes || [])].map((n, i) => ({ id: "n" + i, outcome: "finished", ended: n.finished, note: { text: n.text } })));

  export async function open(b, noteFor) {
    key = b.key; asking = null; writing = null;
    if (noteFor) write(book.history.find((r) => r.id === noteFor) || null);
    if (wrap.hidden) showPanel(wrap);
    if (noteFor) { await tick(); list.querySelector("textarea")?.focus({ preventScroll: true }); }
  }
  function write(r) {
    if (!r) return;
    writing = r.id; asking = null;
    text = r.note?.text || ""; isPublic = !!r.note?.public;
  }
  async function startWriting(r) { write(r); await tick(); list.querySelector("textarea")?.focus({ preventScroll: true }); }
  function title(r) {
    if (r.now) return "Reading now";
    const when = whenText(r.ended), verb = r.outcome === "finished" ? "Finished" : "Set aside";
    return !when ? verb : r.ended.precision === "year" ? `${verb} in ${when}` : `${verb} ${when}`;
  }
  function meta(r) {
    const out = [];
    if (r.now) return [`Started ${longDate(book.readingSince)}`, book.pages ? `page ${fmt(Math.min(book.page, book.pages))} of ${fmt(book.pages)}` : `page ${fmt(book.page)}`].join(" · ");
    if (!ui.owner) return "";
    if (known(r.started)) out.push(`Started ${whenText(r.started)}`);
    if (known(r.started) && known(r.ended)) out.push(plural(Math.max(1, Math.round((ms(r.ended) - ms(r.started)) / DAY)), "day"));
    const page = r.progress?.at(-1)?.page;
    if (r.outcome === "set_aside" && page) out.push(`stopped on page ${fmt(page)}`);
    if (!whenText(r.ended) && !known(r.started)) out.push("no date");
    return out.join(" · ");
  }
  async function run(f) { busy = true; try { await f(); } catch (err) { toast(err.message); } busy = false; }
  const save = (r) => run(async () => { const t = text.trim(); await saveNote(r, t, isPublic && !!t); writing = null; });
  const remove = (r) => run(async () => { await deleteReading(r); asking = null; });
</script>

<div class="modal" id="readings" hidden bind:this={wrap}>
  <div class="modal-scrim" data-close></div>
  <section class="card wide" role="dialog" aria-modal="true" aria-labelledby="rd-h">
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    <div><p class="wordmark">{!book ? "" : !ui.owner ? "Notes" : book.readCount ? times(book.readCount) : book.reading ? "Reading now" : "Readings"}</p><h2 id="rd-h">{book?.title}</h2></div>
    <ol class="rd-list" bind:this={list}>
      {#each items as r (r.id)}
        <li class="rd">
          <div class="rd-top"><b>{title(r)}</b><span class="rd-meta">{meta(r)}</span>
            {#if ui.owner && !r.now && asking !== r.id}
              <span class="rd-tools"><button type="button" class="link" onclick={async () => { const b = book; await hidePanel(wrap); onCorrect(b, r); }}>Edit</button><button type="button" class="link" onclick={() => (asking = r.id)}>Delete</button></span>
            {/if}
          </div>
          {#if asking === r.id}
            <div class="rd-ask"><span>Delete this reading{r.note ? " and its note" : ""}?</span><button type="button" class="btn sm danger" disabled={busy} onclick={() => remove(r)}>Delete</button><button type="button" class="btn sm" onclick={() => (asking = null)}>Keep it</button></div>
          {/if}
          {#if r.outcome === "finished"}
            {#if writing === r.id}
              <div class="rd-edit">
                <textarea class="inp" rows="4" aria-label="Note" placeholder="What stayed with you?" bind:value={text}></textarea>
                <label class="toggle"><span><b>Public note</b><small>Visitors can read it on the book’s page.</small></span><input type="checkbox" bind:checked={isPublic}></label>
                <div class="rd-btns"><button type="button" class="btn sm" onclick={() => (writing = null)}>Cancel</button><button type="button" class="btn sm primary" disabled={busy} onclick={() => save(r)}>Save note</button></div>
              </div>
            {:else}
              <div class="rd-note">
                {#if r.note}<p>{r.note.text}</p>{/if}
                {#if ui.owner}
                  <div class="foot"><button type="button" class="link" onclick={() => startWriting(r)}>{r.note ? "Edit note" : "Add a note"}</button>{#if r.note}<span class="pub">{r.note.public ? "Public" : "Only you"}</span>{/if}</div>
                {/if}
              </div>
            {/if}
          {/if}
        </li>
      {/each}
    </ol>
  </section>
</div>
