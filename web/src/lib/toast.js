// A short note at the foot of the screen, with at most one action (Undo, Add a note).
import { EASE, motionOK, settled } from "../engine/model.js";

export function toast(text, action) {
  document.querySelector(".toast")?.remove();
  const t = document.createElement("div");
  t.className = "toast"; t.setAttribute("role", "status");
  t.append(Object.assign(document.createElement("span"), { textContent: text }));
  if (action) {
    const b = Object.assign(document.createElement("button"), { type: "button", textContent: action.label });
    b.addEventListener("click", () => { t.remove(); action.run(); });
    t.append(b);
  }
  document.body.append(t);
  if (motionOK()) t.animate([{ opacity: 0, transform: "translateY(8px)" }, { opacity: 1, transform: "none" }], { duration: 260, easing: EASE });
  setTimeout(() => t.remove(), action ? 6000 : 2200);
}

// The quiet "Settings saved" pill: one at a time, kept up while the owner keeps changing things.
let savedTimer = 0;
export function savedToast(note = "Settings saved") {
  let t = document.querySelector(".toast.saved");
  if (!t) {
    document.querySelector(".toast")?.remove();
    t = document.createElement("div"); t.className = "toast saved"; t.setAttribute("role", "status");
    t.innerHTML = `<svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12.5 4.5 4.5L19 7.5"/></svg><span></span>`;
    document.body.append(t);
    if (motionOK()) t.animate([{ opacity: 0, transform: "translateY(6px)" }, { opacity: 1, transform: "none" }], { duration: 220, easing: EASE });
  }
  t.querySelector("span").textContent = note;
  clearTimeout(savedTimer);
  savedTimer = setTimeout(async () => {
    if (motionOK()) await settled(t.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 260, fill: "forwards" }), 260);
    t.remove();
  }, 1700);
}
