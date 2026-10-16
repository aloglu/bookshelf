// Copying to the clipboard. Browsers give a page the clipboard only at a secure address (https, or the computer
// it runs on); at a plain http one, as on a home network (http://192.168.1.10:8080), the older way still copies:
// the text selected in a hidden field. It says whether the text was copied.
export async function copyText(text) {
  try {
    if (navigator.clipboard) { await navigator.clipboard.writeText(text); return true; }
  } catch { /* refused: the older way may still work */ }
  const field = document.createElement("textarea"), had = document.activeElement;
  field.value = text;
  field.readOnly = true;   // no keyboard on a phone
  field.style.cssText = "position: fixed; top: 0; left: 0; opacity: 0; pointer-events: none";
  document.body.append(field);
  field.select();
  field.setSelectionRange(0, text.length);   // iOS selects only this way
  let done = false;
  try { done = document.execCommand("copy"); } catch { done = false; }
  field.remove();
  had?.focus?.({ preventScroll: true });
  return done;
}
