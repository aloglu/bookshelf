<script>
  // The stats page (/stats): the books being read, then the library in numbers.
  import { tick } from "svelte";
  import { ui, data, setPage } from "../lib/app.svelte.js";
  import { openBook, hooks } from "../engine/reader.js";
  import { toast } from "../lib/toast.js";
  import { renderStats } from "./stats.js";

  let { active } = $props();
  let box;
  // A book shown here opens right here, rising from its cover.
  const openFromStats = (b, el) => openBook(b, el);
  async function page(b, n) {
    try { await setPage(b, n); } catch (err) { toast(err.message); return; }
    if (n >= b.pages) toast(`Finished “${b.title}”?`, { label: "Mark finished", run: () => hooks.finishReading(b) });
    else toast(`On page ${n.toLocaleString("en-US")}`);
  }
  function draw() {
    if (!box || !active) return;
    const list = data.books, hidden = list.filter((b) => b.hidden).length;
    renderStats(box, list, { owner: ui.owner, progress: ui.owner || !!data.library?.settings.showProgress, hidden, openFromStats, setPage: page });
  }
  $effect(() => { data.books; ui.owner; if (active) tick().then(draw); });
  function table(e) {
    const card = e.currentTarget.closest(".chart"), on = card.classList.toggle("as-table");
    e.currentTarget.textContent = on ? "Chart" : "Table";
    draw();
  }
</script>

<section class="stats" id="stats" hidden={!active} aria-labelledby="stats-h" bind:this={box}>
  <div class="reading-block" hidden>
    <h2 class="stats-h">Now reading</h2>
    <div class="reading-cards"></div>
  </div>
  <h2 class="stats-h" id="stats-h">The library in numbers</h2>
  <div class="kpis"></div>
  <div class="charts">
    <article class="chart wide" data-chart="decades"><header><h3>When they were written</h3><p>Books by the decade they first appeared, for the decades that have any</p><button type="button" class="link tbl" onclick={table}>Table</button></header><div class="plot"></div></article>
    <article class="chart tall" data-chart="lengths"><header><h3>How long they run</h3><p>Books by page count{#if data.books.some((b) => !b.pages)}, for those that have one{/if}</p><button type="button" class="link tbl" onclick={table}>Table</button></header><div class="plot"></div></article>
    <article class="chart" data-chart="bindings"><header><h3>How they are bound</h3><p>Books by binding</p><button type="button" class="link tbl" onclick={table}>Table</button></header><div class="plot"></div></article>
    <article class="chart" data-chart="translation"><header><h3>In translation</h3><p>Books with a translator named</p></header><div class="plot"></div></article>
    <article class="chart" data-chart="authors"><header><h3>Authors you return to</h3><p>Two or more books on the shelves</p><button type="button" class="link tbl" onclick={table}>Table</button></header><div class="plot"></div></article>
    <article class="chart" data-chart="publishers"><header><h3>Publishers</h3><p>The six with the most books here</p><button type="button" class="link tbl" onclick={table}>Table</button></header><div class="plot"></div></article>
    <article class="chart wide" data-chart="extremes"><header><h3>At the edges</h3><p>The oldest, newest, longest and shortest</p></header><div class="plot"></div></article>
  </div>
</section>
