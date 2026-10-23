<script>
  // Adding a book: look up its ISBN (typed, or read from the barcode), check the details on a card with its
  // cover, then the shelf scrolls to the saved book and opens it. A periodical's ISSN, or a book with neither, is
  // entered by hand.
  import { tick } from "svelte";
  import { api } from "../lib/api.js";
  import { data, ui } from "../lib/app.svelte.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast } from "../lib/toast.js";
  import { cameraProblem, scan } from "../lib/scan.js";
  import { setQuery } from "../lib/app.svelte.js";
  import { touch } from "../engine/model.js";

  let { openShelved, showAdded } = $props();
  let wrap, card, findInput, video, addForm = $state();
  let step = $state("find"), id = $state(""), status = $state(null), asking = 0, scanning = null;
  let draft = $state(null), err = $state(""), busy = $state(false);

  export function open() {
    step = "find"; id = ""; status = null;
    showPanel(wrap, "#isbn");
  }
  const issnLike = (v) => { const d = v.replace(/[^\dx]/gi, ""); return /issn/i.test(v) || d.length === 8 || (d.length === 13 && d.startsWith("977")); };
  function back() {
    asking++; scanning?.abort(); scanning = null;
    status = null; step = "find";
    if (!touch()) tick().then(() => findInput?.focus());
  }
  async function lookUp(how = "") {
    const token = ++asking, typed = id.trim();
    if (!typed) { status = { text: "Type the 13 digits printed above the barcode." }; return; }
    status = { spin: true, text: "Asking Open Library…" };
    let d;
    try {
      d = await api("GET", `/api/lookup?id=${encodeURIComponent(typed)}`);
    } catch (e) {
      if (token !== asking) return;
      if (e.status === 409) status = { text: `${e.message} `, show: e.data.book };
      else if (e.status === 404) status = { text: "Open Library and Google Books don’t know this ISBN.", byHand: true };
      else status = { text: e.message, byHand: true };
      return;
    }
    if (token !== asking) return;
    if (d.periodical) return startDraft({ identifier: d.identifier }, "Periodicals can’t be looked up. Enter the issue’s details; only the title is required.");
    startDraft({ ...d, identifier: d.isbn }, `Found on ${d.source}${how ? ", " + how : ""}. Check the details, then add it.`);
  }
  async function startDraft(src, note) {
    status = null;
    draft = {
      title: src.title || "", author: src.author || "", translator: src.translator || "", publisher: src.publisher || "",
      published: src.published ?? "", pages: src.pages ?? "", binding: src.binding || "paperback",
      identifier: src.identifier || "", issue: "", visible: true, coverUrl: src.coverUrl || "", file: null, preview: src.coverUrl || "",
      heading: src.title ? "Is this the one?" : "Add it by hand", note,
    };
    err = ""; step = "confirm";
    await tick();
    card.scrollTop = 0;
    if (src.title) addForm.querySelector("[type=submit]").focus({ preventScroll: true });
    else if (!touch()) addForm.elements.title.focus({ preventScroll: true });
  }
  const byHand = () => startDraft({ identifier: id.trim() }, "Only the title is required.");
  async function showIt(bookId) {
    await hidePanel(wrap);
    openShelved(data.books.find((b) => b.id === bookId));
  }
  // The camera reads the barcode by itself; the number then goes through the same lookup.
  async function startScan() {
    const problem = cameraProblem();
    if (problem) { status = { text: problem }; return; }
    scanning?.abort();
    const session = new AbortController();
    scanning = session;
    step = "scan";
    await tick();
    if (scanning !== session || session.signal.aborted) return;
    try {
      const code = await scan(video, session.signal);
      if (scanning !== session || step !== "scan") return;
      step = "find"; id = code;
      lookUp("read from the barcode");
    } catch (e) {
      if (e.name === "AbortError" || scanning !== session) return;
      step = "find";
      status = { text: e.name === "BarcodeError" ? "The barcode reader couldn’t read the camera. Try scanning again, or type the ISBN."
        : e.name === "NotAllowedError" ? "Bookshelf wasn’t allowed to use the camera. Type the ISBN instead." : "The camera couldn’t be started. Type the ISBN instead." };
    } finally { if (scanning === session) scanning = null; }
  }
  function pickCover(e) {
    const file = e.target.files[0]; e.target.value = "";
    if (!file) return;
    if (draft.preview.startsWith("blob:")) URL.revokeObjectURL(draft.preview);
    draft.file = file; draft.preview = URL.createObjectURL(file);
  }
  async function add(e) {
    e.preventDefault();
    const v = (n) => String(draft[n] ?? "").trim();
    err = !v("title") ? "A book needs a title."
      : v("published") && !/^\d{1,4}$/.test(v("published")) ? "The year should be a number, like 1962."
      : v("pages") && !(/^\d{1,5}$/.test(v("pages")) && +v("pages") > 0) ? "Pages should be a whole number." : "";
    if (err) return;
    busy = true;
    let b;
    try {
      b = await api("POST", "/api/books", {
        title: v("title"), author: v("author"), translator: v("translator"), publisher: v("publisher"),
        published: v("published") ? +v("published") : null, pages: v("pages") ? +v("pages") : null, binding: draft.binding,
        identifier: v("identifier"), issue: v("issue"), hidden: !draft.visible, coverUrl: draft.file ? "" : draft.coverUrl,
      });
      if (draft.file) await api("PUT", `/api/books/${b.id}/cover`, draft.file).catch((e) => toast(`The book is added, but its cover wasn’t: ${e.message}`));
    } catch (e) {
      err = e.message; busy = false;
      return;
    }
    busy = false;
    await hidePanel(wrap);
    if (ui.query) { setQuery(""); document.getElementById("q").value = ""; }
    await showAdded(b.id);
  }
</script>

<div class="modal" id="adder" hidden bind:this={wrap} onpanelhide={() => { asking++; scanning?.abort(); }}>
  <div class="modal-scrim" data-close></div>
  <section class="card wide" role="dialog" aria-modal="true" aria-labelledby="add-h" bind:this={card}>
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    <div class="stack" hidden={step !== "find"}>
      <h2 id="add-h">Add a book</h2>
      <form class="stack" novalidate onsubmit={(e) => { e.preventDefault(); lookUp(); }}>
        <label class="lab">ISBN or ISSN<input class="inp isbn" id="isbn" inputmode="numeric" autocomplete="off" placeholder="978…" spellcheck="false" bind:value={id} bind:this={findInput}></label>
        <div class="row-btns">
          <button type="submit" class="btn primary"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.6-3.6"/></svg>Look up</button>
          <button type="button" class="btn" onclick={startScan}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 8V5h3M17 5h3v3M20 16v3h-3M7 19H4v-3M8 9v6M11 9v6M14 9v6M17 9v6"/></svg>Scan barcode</button>
        </div>
      </form>
      <p class="lookup-status" class:quiet={!status} role="status">
        {#if status}
          {#if status.spin}<i class="spin" aria-hidden="true"></i>{/if}<span>{status.text}{#if status.show}<button type="button" class="link" onclick={() => showIt(status.show)}>Show it</button>{/if}</span>
        {/if}
      </p>
      <p class="fine">{#if !status?.byHand}No ISBN?{" "}{/if}<button type="button" class="link" onclick={byHand}>Enter the details by hand</button>.</p>
    </div>
    <div class="stack" hidden={step !== "scan"}>
      <h2>Scan the barcode</h2>
      <div class="viewfinder"><video bind:this={video} muted playsinline></video><span class="scan-line" aria-hidden="true"></span></div>
      <div class="row-btns"><button type="button" class="btn" onclick={back}>Type it instead</button></div>
    </div>
    {#if draft}
      <form class="stack" hidden={step !== "confirm"} novalidate bind:this={addForm} onsubmit={add}>
        <div><h2>{draft.heading}</h2><p class="source">{draft.note}</p></div>
        <div class="found">
          <label class="mini" title="Replace the cover" style:--c="#8a7a66">{#if draft.preview}<img alt="" src={draft.preview}>{/if}<span>{draft.preview ? "Replace cover" : "Add a cover"}</span><input type="file" accept="image/*" hidden onchange={pickCover}></label>
          <div class="fields">
            <label class="lab full">Title<input class="inp" name="title" autocomplete="off" bind:value={draft.title}></label>
            <label class="lab full">Author<input class="inp" name="author" autocomplete="off" bind:value={draft.author}></label>
            <label class="lab full">Translator<input class="inp" name="translator" autocomplete="off" bind:value={draft.translator}></label>
            <label class="lab">Publisher<input class="inp" name="publisher" autocomplete="off" bind:value={draft.publisher}></label>
            <label class="lab">Published<input class="inp" name="published" inputmode="numeric" autocomplete="off" bind:value={draft.published}></label>
            <label class="lab">Pages<input class="inp" name="pages" inputmode="numeric" autocomplete="off" bind:value={draft.pages}></label>
            <label class="lab">Binding<select class="inp" name="binding" bind:value={draft.binding}><option value="paperback">Paperback</option><option value="hardcover">Hardcover</option><option value="cloth">Cloth</option><option value="leather">Leather</option></select></label>
            <label class="lab full">ISBN or ISSN<input class="inp" name="identifier" autocomplete="off" spellcheck="false" bind:value={draft.identifier}></label>
            {#if issnLike(draft.identifier) || draft.issue}<label class="lab full">Issue<input class="inp" name="issue" autocomplete="off" placeholder="70–71" bind:value={draft.issue}></label>{/if}
            <label class="check2 full"><input type="checkbox" name="visible" bind:checked={draft.visible}>Show on the website</label>
          </div>
        </div>
        <p class="err" role="alert">{err}</p>
        <div class="row-btns end"><button type="button" class="btn" onclick={back}>Back</button><button type="submit" class="btn primary" disabled={busy}>Add to the shelf</button></div>
      </form>
    {/if}
  </section>
</div>
