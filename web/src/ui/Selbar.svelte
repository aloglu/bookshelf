<script>
  // Choosing several books: hide, show or remove them together.
  import { ui, setHidden, removeBooks, setSelecting } from "../lib/app.svelte.js";
  import { picked } from "../engine/shelf.js";
  import { toast } from "../lib/toast.js";

  let asking = $state(false), no = $state();
  $effect(() => { ui.chosen; asking = false; });
  $effect(() => { if (asking) no?.focus(); });
  const run = async (f) => { try { await f(); } catch (err) { toast(err.message); } };
  const hide = (hidden) => run(() => setHidden([...picked], hidden));
  const remove = () => run(async () => { const list = [...picked]; setSelecting(false); await removeBooks(list); });
</script>

<div class="selbar" id="selbar" hidden={!ui.selecting}>
  <span class="count">{ui.chosen ? `${ui.chosen} chosen` : "Choose books"}</span>
  <span class="sel-acts">
    {#if asking}
      <span class="ask">Remove {ui.chosen === 1 ? "this book" : `${ui.chosen} books`} from your library?</span>
      <button type="button" class="btn sm danger solid" onclick={remove}>Remove</button>
      <button type="button" class="btn sm" bind:this={no} onclick={() => (asking = false)}>Cancel</button>
    {:else}
      <button type="button" class="btn sm" disabled={!ui.chosen} onclick={() => hide(true)}>Hide</button>
      <button type="button" class="btn sm" disabled={!ui.chosen} onclick={() => hide(false)}>Show</button>
      <button type="button" class="btn sm danger" disabled={!ui.chosen} onclick={() => (asking = true)}>Remove</button>
    {/if}
  </span>
  <button type="button" class="btn sm" onclick={() => setSelecting(false)}>Done</button>
</div>
