<script>
  // Notes as Markdown, one book's or all of them: a preview first, then copy it or save it as a file. Only what
  // visitors can read, unless the owner includes their private notes.
  import { tick } from "svelte";
  import { notesMarkdown, markdownURL } from "../lib/app.svelte.js";
  import { showPanel } from "../lib/panels.js";
  import { toast } from "../lib/toast.js";
  import { copyText } from "../lib/copy.js";
  import { ICON } from "../lib/notes.js";

  let wrap, shown = $state(false), book = $state(null), withPrivate = $state(false), text = $state(""), loading = $state(false);
  // book: the book whose notes to export, or null for every book's.
  export async function open(b) {
    book = b; withPrivate = false; text = ""; shown = true;
    await tick();
    showPanel(wrap);
    preview();
  }
  // Only the latest preview is shown: switching private notes on and off quickly can bring the answers back out
  // of order, and an earlier one must not stand under the switch's new state.
  let asked = 0;
  async function preview() {
    const n = ++asked;
    loading = true;
    let got = "";
    try { got = await notesMarkdown(book?.id, withPrivate); } catch (err) { if (n === asked) toast(err.message); }
    if (n !== asked) return;
    text = got; loading = false;
  }
  async function copy() {
    toast(await copyText(text) ? "Markdown copied" : "Copying isn’t allowed here. Save it as a file instead.");
  }
</script>

<div class="modal" id="export" hidden bind:this={wrap} onpanelclose={() => (shown = false)}>
  <div class="modal-scrim" data-close></div>
  {#if shown}
  <section class="card wide export-card" role="dialog" aria-modal="true" aria-labelledby="export-h">
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.x}</svg></button>
    <h2 id="export-h">{book ? `Notes on ${book.title}` : "All your notes"}</h2>
    <p class="lede">Plain Markdown. It opens in Obsidian, Notion, or any text editor. {book ? "One file for this book." : "One file, a heading for each book."}</p>
    <div class="stack">
      <label class="toggle"><span><b>Include notes only you can see</b><small>Off exports just what visitors can read.</small></span><input type="checkbox" bind:checked={withPrivate} onchange={preview}></label>
      <pre class="mdprev" aria-label="Preview" aria-busy={loading}>{text}</pre>
      <div class="row-btns end">
        <button type="button" class="btn" disabled={!text || loading} onclick={copy}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.copy}</svg>Copy</button>
        <a class="btn primary" href={markdownURL(book?.id, withPrivate)} download><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.down}</svg>Save as file</a>
      </div>
    </div>
  </section>
  {/if}
</div>
