<script>
  // The note editor: one window for a new note and for editing one, so both look and work the same. Your
  // thoughts have a window of their own, always; + Passage adds the passage from the book (your words so far
  // go above it, and what you write next goes below it), + Page adds a place (a page, a chapter or a
  // location) with a small kind button inside it. The last two buttons say who can read it and keep it
  // (Save for a new note, Done for one being edited). A × throws the draft, or the changes, away.
  import { tick, flushSync } from "svelte";
  import { touch } from "../engine/model.js";
  import { ICON, KINDS, LONG, NEXT, whereField, whereFrom, typedWhere, noteDate } from "../lib/notes.js";

  // entry: null for a new note. resume: a draft to go on with (Undo after throwing one away). Editing one, the
  // row of buttons also has its date (a link to its own page, href; onopen goes there) and Delete.
  let { entry = null, resume = null, readingId = "", startsPublic = false, href = "", onopen, onsave, ondiscard, ondelete } = $props();
  function follow(ev) {
    if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey || ev.button) return;
    ev.preventDefault();
    onopen?.();
  }

  const make = () => {
    const d = resume || {};
    const from = entry || {};
    const quote = d.quote ?? from.quote ?? "";
    return {
      pre: d.pre ?? from.pre ?? "", quote, text: d.text ?? from.text ?? "",
      showQuote: d.showQuote ?? !!quote, hasPre: d.hasPre ?? !!from.pre,
      showWhere: d.showWhere ?? !!from.where, kind: d.kind ?? from.where?.kind ?? "page", raw: d.raw ?? (from.where ? whereField(from.where) : ""),
      isPublic: d.isPublic ?? (entry ? !!entry.public : startsPublic), open: !!(entry || resume),
    };
  };
  const first = make();
  const firstDraft = JSON.stringify({ ...first, open: undefined });
  let pre = $state(first.pre), quote = $state(first.quote), text = $state(first.text);
  let showQuote = $state(first.showQuote), hasPre = $state(first.hasPre);
  let showWhere = $state(first.showWhere), kind = $state(first.kind), raw = $state(first.raw);
  let isPublic = $state(first.isPublic), open = $state(first.open), err = $state(""), busy = $state(false);
  let preEl = $state(), quoteEl = $state(), textEl = $state(), whereEl = $state(), root = $state();

  const editing = $derived(!!entry);
  const draft = () => ({ pre, quote, text, showQuote, hasPre, showWhere, kind, raw, isPublic, open: undefined });
  const written = $derived(!!(pre.trim() || quote.trim() || text.trim() || raw.trim()));
  const show = $derived(open || written);
  // Opening the editor by itself (Add a note, Edit) puts the cursor in it, except on a touch screen, where the
  // keyboard comes up only from the reader's own tap.
  const focus = async (el) => { await tick(); if (!touch()) (typeof el === "function" ? el() : el)?.focus({ preventScroll: editing }); };
  // A tap on Write a note, + Passage or + Page puts the cursor in its field at once, inside the tap: iOS opens
  // the keyboard (or keeps it up) only for a field focused during the tap itself.
  const take = (get) => { flushSync(); get()?.focus({ preventScroll: editing }); };

  // Called from outside: a new note begun for a reading (Add a note, after finishing).
  export function begin() { open = true; focus(() => textEl); }

  function togglePassage() {
    open = true; err = "";
    if (!showQuote) {
      showQuote = true;
      // Words typed first stay above the passage, which starts on a new line below them.
      if (text.trim()) { pre = text.replace(/\s+$/, ""); text = ""; hasPre = true; }
      take(() => quoteEl);
    } else {
      // Hiding the passage keeps what was typed in it; the words either side join up again.
      showQuote = false;
      if (hasPre) { text = [pre.trim(), text.trim()].filter(Boolean).join("\n"); pre = ""; hasPre = false; }
      take(() => textEl);
    }
  }
  function addPlace() { showWhere = true; open = true; err = ""; take(() => whereEl); }
  // An empty place field folds back into "+ Page" when left (the kind button keeps it open: it never takes focus).
  // On iOS a tap on the kind button can still take focus from the field, so a tap just now counts as staying.
  let kindTap = 0;
  function leftPlace() { if (!raw.trim() && Date.now() - kindTap > 500) showWhere = false; }
  const cycle = () => { kind = NEXT[kind]; raw = typedWhere(kind, raw); whereEl?.focus(); };
  const typed = () => { const v = typedWhere(kind, raw); if (v !== raw) raw = v; };

  async function save(ev) {
    ev.preventDefault();
    const q = showQuote ? quote.trim() : "", p = q ? pre.trim() : "", t = q ? text.trim() : [pre.trim(), text.trim()].filter(Boolean).join("\n");
    if (!q && !p && !t) { err = showQuote ? "Add the passage, or write something." : "Write something first."; return; }
    let where = null;
    if (showWhere && raw.trim()) {
      where = whereFrom(kind, raw);
      if (!where) { err = `Give the ${LONG[kind].toLowerCase()} as a number.`; return; }
    }
    busy = true; err = "";
    try {
      await onsave({ pre: p, quote: q, text: t, where, readingId: entry ? entry.readingId : readingId, public: isPublic });
    } catch (e) {
      err = e.message; busy = false;
      return;
    }
    busy = false;
  }
  // What was there is handed back, with whether anything was written (a new note) or changed (an edit).
  function discard() {
    const had = draft();
    document.activeElement?.blur?.();
    ondiscard?.(had, entry ? JSON.stringify(had) !== firstDraft : written);
  }
  // Esc leaves the field; it doesn't take the page away from a note half written.
  function keys(ev) { if (ev.key === "Escape") { ev.stopPropagation(); ev.preventDefault(); ev.target.blur?.(); } }

  // The editor opened for editing puts the cursor in the first field (not on a touch screen: the keyboard stays away).
  $effect(() => { if (editing) focus(() => preEl || quoteEl || textEl); });
  // The fields grow with what is written in them (by CSS where the browser can; otherwise here).
  const grows = typeof CSS !== "undefined" && CSS.supports("field-sizing", "content");
  $effect(() => {
    pre; quote; text; showQuote; hasPre;
    if (grows) return;
    tick().then(() => root?.querySelectorAll("textarea").forEach((el) => { el.style.height = "auto"; el.style.height = el.scrollHeight + "px"; }));
  });
  const forPh = $derived(showQuote ? "" : "What did you think?");
</script>

{#snippet fields()}
  <div class="cp-fields">
    {#if showQuote && hasPre}<textarea class="ct pre" rows="1" aria-label="Your thoughts" onkeydown={keys} bind:value={pre} bind:this={preEl}></textarea>{/if}
    {#if showQuote}<textarea class="cq" rows="1" placeholder="The passage, as printed in the book" aria-label="Passage" onkeydown={keys} bind:value={quote} bind:this={quoteEl}></textarea>{/if}
    <textarea class="ct" class:after={showQuote && hasPre} rows="1" placeholder={forPh} aria-label="Your thoughts" onkeydown={keys} bind:value={text} bind:this={textEl}></textarea>
  </div>
{/snippet}

{#snippet foot(primary)}
  <div class="cp-foot">
    <div class="cp-opts">
      <button type="button" class="btn" aria-pressed={showQuote} aria-label="Passage" title="A passage from the book" onclick={togglePassage}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.quote}</svg><span class="lb">Passage</span></button>
      <button type="button" class="btn pg-add" aria-label="Page" title="Where in the book" hidden={showWhere} onclick={addPlace}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.plus}</svg><span class="lb">Page</span></button>
      <span class="btn pg-on" hidden={!showWhere}>
        <button type="button" class="pg-kind" aria-label="{LONG[kind]}: tap to switch between page, chapter and location" title="{LONG[kind]}. Tap to switch between page, chapter and location" onpointerdown={(e) => { kindTap = Date.now(); e.preventDefault(); }} onclick={cycle}>{KINDS[kind]}<svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.swap}</svg></button>
        <input autocomplete="off" inputmode="numeric" placeholder="212" aria-label="{LONG[kind]} number" bind:value={raw} bind:this={whereEl} oninput={typed} onblur={leftPlace} onkeydown={keys}>
      </span>
      {#if editing}<a class="btn cp-date" {href} title="This note’s own page" onclick={follow}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.date}</svg><span class="lb">{noteDate(entry.createdAt)}</span></a>{/if}
    </div>
    <div class="cp-act">
      {#if editing}<button type="button" class="btn del" aria-label="Delete this note" title="Delete this note" onclick={ondelete}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.trash}</svg><span class="lb">Delete</span></button>{/if}
      <button type="button" class="btn" aria-pressed={isPublic} aria-label={isPublic ? "Public" : "Only me"} title={isPublic ? "Public: visitors can read this. Click to make it private." : "Only me: only you can read this. Click to let visitors read it."} onclick={() => (isPublic = !isPublic)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html isPublic ? ICON.eye : ICON.lock}</svg><span class="lb">{isPublic ? "Public" : "Only me"}</span></button>
      {@render primary()}
    </div>
  </div>
{/snippet}

{#if editing}
  <li class="nb-e editing" id="e{entry.id}" bind:this={root}>
    <form novalidate class="cp-edit" onsubmit={save} style="display: contents">
      <button type="button" class="btn icon cp-x" aria-label="Discard changes" title="Discard changes" onclick={discard}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.x}</svg></button>
      {@render fields()}
      {#snippet done()}<button type="submit" class="btn primary" disabled={busy}>Done</button>{/snippet}
      {@render foot(done)}
      <p class="err" role="alert">{err}</p>
    </form>
  </li>
{:else}
  <form class="cp" class:open={show} id="cp-nw" novalidate bind:this={root} onsubmit={save}>
    <button type="button" class="cp-bar" onclick={() => { open = true; take(() => textEl); }}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.pen}</svg>Write a note</button>
    <div class="cp-body">
      {#if show}<button type="button" class="btn icon cp-x" aria-label="Discard this note" title="Discard this note" onclick={discard}><svg class="i" viewBox="0 0 24 24" aria-hidden="true">{@html ICON.x}</svg></button>{/if}
      {@render fields()}
      {#snippet saveBtn()}<button type="submit" class="btn primary" disabled={busy}>Save</button>{/snippet}
      {@render foot(saveBtn)}
      <p class="err" role="alert">{err}</p>
    </div>
  </form>
{/if}
