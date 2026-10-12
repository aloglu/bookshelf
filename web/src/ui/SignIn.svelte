<script>
  // Signing in: the user name and password from the server's settings; a week on this device.
  import { ui, signIn } from "../lib/app.svelte.js";
  import { showPanel, hidePanel } from "../lib/panels.js";
  import { toast } from "../lib/toast.js";

  let wrap, form, err = $state(""), user = $state(""), pass = $state(""), busy = $state(false);
  export function open() { err = ""; showPanel(wrap, "[name=user]"); }
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
    await hidePanel(wrap);
    toast(`Signed in as ${ui.user}`);
  }
</script>

<div class="modal" id="signin" hidden bind:this={wrap}>
  <div class="modal-scrim" data-close></div>
  <form class="card stack" role="dialog" aria-modal="true" aria-labelledby="signin-h" novalidate bind:this={form} onsubmit={submit}>
    <button type="button" class="btn icon x" data-close aria-label="Close"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
    <div><p class="wordmark">{ui.name}</p><h2 id="signin-h">Sign in</h2></div>
    <label class="lab">User name<input class="inp" name="user" autocomplete="username" bind:value={user}></label>
    <label class="lab">Password<input class="inp" name="pass" type="password" autocomplete="current-password" bind:value={pass}></label>
    <p class="err" role="alert">{err}</p>
    <button type="submit" class="btn primary" disabled={busy}>Sign in</button>
    <p class="fine">You stay signed in for a week on this device.</p>
  </form>
</div>
