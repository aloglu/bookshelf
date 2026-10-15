<script>
  // Signing in: its own page (/signin), and the page a private library shows visitors at any address. The
  // user name and password from the server's settings; a week on this device. The fields are only in the page
  // while it shows: a password field left hidden in it makes password managers offer to fill or save a
  // password whenever anything else on the page changes (opening Settings, say). On a touch screen nothing
  // focuses a field for the reader; the keyboard opens from their own tap.
  import { tick } from "svelte";
  import { ui, signIn } from "../lib/app.svelte.js";
  import { touch } from "../engine/model.js";
  import { toast } from "../lib/toast.js";

  let { active, onDone } = $props();
  let form = $state(), err = $state(""), user = $state(""), pass = $state(""), busy = $state(false);
  $effect(() => {
    if (!active) return;
    err = "";
    tick().then(() => { if (!touch()) form?.elements.user.focus({ preventScroll: true }); });
  });
  async function submit(e) {
    e.preventDefault();
    err = !user.trim() ? "Enter your user name." : !pass ? "Enter your password." : "";
    if (err) return form.elements[user.trim() ? "pass" : "user"].focus();
    busy = true;
    try {
      await signIn(user.trim(), pass);
    } catch (e) {
      err = e.message; busy = false;
      return form.elements.pass.select();
    }
    busy = false; pass = "";
    onDone();
    toast(`Signed in as ${ui.user}`);
  }
</script>

<section class="signin" id="signin" hidden={!active} aria-labelledby="signin-h">
  {#if active}
    {#if ui.gated}
      <svg class="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>
      <h2 id="signin-h">This library is private</h2>
    {:else}
      <h2 id="signin-h">Sign in</h2>
    {/if}
    <form class="stack" novalidate bind:this={form} onsubmit={submit}>
      <label class="lab">User name<input class="inp" name="user" autocomplete="username" autocapitalize="none" spellcheck="false" bind:value={user}></label>
      <label class="lab">Password<input class="inp" name="pass" type="password" autocomplete="current-password" bind:value={pass}></label>
      <p class="err" role="alert">{err}</p>
      <button type="submit" class="btn primary" disabled={busy}>Sign in</button>
      <p class="fine">You stay signed in for a week on this device.</p>
    </form>
  {/if}
</section>
