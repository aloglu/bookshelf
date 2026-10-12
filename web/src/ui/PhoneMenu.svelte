<script>
  // The phone menu stands in for the tools a phone has no room for; each row closes it and does the same thing.
  import { ui, setSort, signOut, setSelecting } from "../lib/app.svelte.js";
  import { root } from "../engine/model.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast } from "../lib/toast.js";

  let { setView, onSignIn, onSettings } = $props();
  let wrap, dark = $state(false);
  export function open() {
    dark = root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    showPanel(wrap);
  }
  const then = (run) => async () => { await hidePanel(wrap); run(); };
  const flip = () => { root.dataset.theme = dark ? "light" : "dark"; };
  const out = async () => { try { await signOut(); } catch (err) { toast(err.message); } };
</script>

<div class="modal" id="phone-menu" hidden bind:this={wrap}>
  <div class="modal-scrim" data-close></div>
  <section class="card menu-sheet" role="dialog" aria-modal="true" aria-label="Menu">
    <div class="lab">Sort by<div class="seg">
      {#each [["author", "Author"], ["title", "Title"], ["year", "Year"]] as [k, name]}
        <button type="button" aria-pressed={ui.sort === k} onclick={() => { if (ui.sort !== k) setSort(k); hidePanel(wrap); }}>{name}</button>
      {/each}
    </div></div>
    <div class="pm-list">
      <button type="button" onclick={then(flip)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z"/></svg><span>{dark ? "Switch to day" : "Switch to evening"}</span></button>
      <button type="button" class="stats-btn" onclick={then(() => setView(ui.view === "stats" ? "shelf" : "stats"))}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 20V11M12 20V5M19 20v-6M3 20h18"/></svg><span>{ui.view === "stats" ? "Back to the shelves" : "Statistics"}</span></button>
      <button type="button" class="owner-only" onclick={then(() => setSelecting(!ui.selecting))}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="m8.5 12 2.5 2.5 4.5-5"/></svg><span>Select books</span></button>
      <button type="button" class="owner-only" class:dot={ui.update?.newer} onclick={then(onSettings)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/></svg><span>Settings</span></button>
      <button type="button" class="owner-only" onclick={then(out)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10"/></svg><span>Sign out</span></button>
      <button type="button" class="visitor-only" onclick={then(onSignIn)}><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M10 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4M14 16l4-4-4-4M18 12H8"/></svg><span>Sign in</span></button>
    </div>
  </section>
</div>
