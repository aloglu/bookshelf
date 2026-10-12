<script>
  // Marking a book as read (it was read before Bookshelf, or without "Reading now"), or correcting a past
  // reading: when it ended as precisely as the owner remembers (a day, a year, or not at all).
  import { markRead, correctReading } from "../lib/app.svelte.js";
  import { pill } from "../lib/pill.js";
  import { when } from "../lib/api.js";
  import { isoDay, today } from "../lib/dates.js";
  import { showPanel, hidePanel } from "../lib/panels.js";

  let wrap, form;
  let book = $state(null), reading = $state(null), after = null;
  let outcome = $state("finished"), prec = $state("day"), start = $state(""), day = $state(""), year = $state("");
  let note = $state(""), isPublic = $state(false), err = $state(""), busy = $state(false);
  const editing = $derived(!!reading);

  export function open(b, r = null, then = null) {
    book = b; reading = r; after = then; err = "";
    outcome = r?.outcome === "set_aside" ? "set_aside" : "finished";
    prec = r && r.ended.precision !== "instant" ? r.ended.precision : "day";
    start = r ? isoDay(r.started) : "";
    day = r ? isoDay(r.ended) : "";
    year = r?.ended.precision === "year" ? r.ended.time.slice(0, 4) : "";
    note = ""; isPublic = false;
    showPanel(wrap, r ? undefined : 'input[name="day"]');
  }
  async function submit(e) {
    e.preventDefault();
    const now = today();
    err = "";
    if (prec === "day" && !day) err = "Choose the day, or another option.";
    else if (prec === "year" && (!/^\d{4}$/.test(year) || +year > new Date().getFullYear())) err = "Enter a year, like 2019.";
    else if (prec === "day" && day > now) err = "That’s in the future.";
    else if (editing && start && start > now) err = "That’s in the future.";
    else if (editing && start && prec === "day" && start > day) err = "It can’t end before it started.";
    else if (editing && outcome === "set_aside" && reading.note) err = "A reading with a note stays finished. Remove the note first.";
    if (err) return;
    const ended = prec === "day" ? when("day", day) : prec === "year" ? when("year", year) : when("unknown");
    busy = true;
    try {
      if (editing) await correctReading(reading, start ? when("day", start) : when("unknown"), ended, outcome);
      else await markRead(book, ended, { text: note.trim(), public: isPublic && !!note.trim() });
    } catch (e) {
      err = e.message; busy = false;
      return;
    }
    busy = false;
    await hidePanel(wrap);
    after?.();
  }
</script>

<div class="modal" id="marker" hidden bind:this={wrap}>
  <div class="modal-scrim" data-close></div>
  <form class="card stack" role="dialog" aria-modal="true" aria-labelledby="mk-h" novalidate bind:this={form} onsubmit={submit}>
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    <div><p class="wordmark">{book?.title}</p><h2 id="mk-h">{editing ? "Correct this reading" : book?.readCount ? "Mark as read again" : "Mark as read"}</h2></div>
    <div class="lab" hidden={!editing}>How it ended<div class="seg" use:pill>
      <button type="button" aria-pressed={outcome === "finished"} onclick={() => (outcome = "finished")}>Finished</button>
      <button type="button" aria-pressed={outcome === "set_aside"} onclick={() => (outcome = "set_aside")}>Set aside</button>
    </div></div>
    <label class="lab" hidden={!editing}><span>Started <span class="opt">· if you know</span></span><input class="inp" type="date" name="start" max={today()} bind:value={start}></label>
    <div class="lab"><span>{outcome === "set_aside" ? "Set aside" : "Finished"}</span><div class="seg" use:pill>
      <button type="button" aria-pressed={prec === "day"} onclick={() => (prec = "day")}>On a day</button>
      <button type="button" aria-pressed={prec === "year"} onclick={() => (prec = "year")}>In a year</button>
      <button type="button" aria-pressed={prec === "unknown"} onclick={() => (prec = "unknown")}>I don’t remember</button>
    </div></div>
    <label class="lab" hidden={prec !== "day"}>Date<input class="inp" type="date" name="day" max={today()} bind:value={day}></label>
    <label class="lab" hidden={prec !== "year"}>Year<input class="inp" name="year" inputmode="numeric" maxlength="4" placeholder="2019" bind:value={year}></label>
    <label class="lab" hidden={editing}><span>Note <span class="opt">· optional</span></span><textarea class="inp" name="note" rows="3" bind:value={note}></textarea></label>
    <label class="toggle" hidden={editing}><span><b>Public note</b><small>Visitors can read it on the book’s page.</small></span><input type="checkbox" name="public" bind:checked={isPublic}></label>
    <p class="err" role="alert">{err}</p>
    <button type="submit" class="btn primary" disabled={busy}>{editing ? "Save" : "Mark as read"}</button>
  </form>
</div>
