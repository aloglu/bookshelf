<script>
  // Author, Title, Year. The active one shows which way it runs; choosing it again turns it round. Every
  // button keeps room for the arrow, so changing the sort never moves the tools beside it.
  import { ui, setSort } from "../lib/app.svelte.js";
  import { SORT_WAYS } from "../engine/model.js";

  let { picked = () => {} } = $props();
  const NAMES = [["author", "Author"], ["title", "Title"], ["year", "Year"]];
</script>

{#each NAMES as [k, name]}
  {@const on = ui.sort === k}
  <button type="button" aria-pressed={on} aria-label="{name}, {SORT_WAYS[k][on && ui.desc ? 1 : 0]}"
    title={on ? `${SORT_WAYS[k][ui.desc ? 1 : 0]}; again for ${SORT_WAYS[k][ui.desc ? 0 : 1]}` : `Sort by ${name.toLowerCase()}`}
    onclick={() => { setSort(k); picked(); }}>{name}<svg class="dir" class:down={on && ui.desc} class:shown={on} viewBox="0 0 24 24" aria-hidden="true"><path d="M12 19V5M6 11l6-6 6 6"/></svg></button>
{/each}
