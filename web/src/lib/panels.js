// Cards and sheets (Settings among them): how they come and go, which one Esc closes, and how typing works in them
// on a touch screen. One set of rules for every layer.
import { EASE, curve, isPhone, motionOK, root, settled, touch } from "../engine/model.js";

export function showPanel(wrap, focus) {
  if (!wrap.hidden && !wrap.closing) return;
  wrap.hidden = false;
  const card = wrap.querySelector(".card");
  if (motionOK()) {
    const from = isPhone() ? "translateY(100%)" : null;
    card.animate(from ? [{ transform: from }, { transform: "none" }] : [{ opacity: 0, transform: "translateY(12px) scale(.985)" }, { opacity: 1, transform: "none" }], { duration: from ? 400 : 260, easing: from ? curve : EASE });
    wrap.querySelector(".modal-scrim").animate([{ opacity: 0 }, { opacity: 1 }], { duration: 240 });
  }
  // On a touch screen the keyboard should only open from the reader's own tap, once the card has settled
  // (focusing a field as the card slides in makes iOS raise the keyboard over it); the card itself takes focus.
  const target = focus && wrap.querySelector(focus);
  if (target && touch() && target.matches("input, textarea, select")) { card.tabIndex = -1; card.focus({ preventScroll: true }); }
  else target?.focus({ preventScroll: true });
}
export async function hidePanel(wrap) {
  if (!wrap || wrap.hidden || wrap.closing) return;
  wrap.closing = true;
  const card = wrap.querySelector(".card");
  if (motionOK()) {
    const to = isPhone() ? "translateY(100%)" : null;
    wrap.querySelector(".modal-scrim").animate([{ opacity: 1 }, { opacity: 0 }], { duration: 200, fill: "forwards" });
    await settled(card.animate(to ? [{ transform: "none" }, { transform: to }] : [{ opacity: 1 }, { opacity: 0, transform: "translateY(8px) scale(.985)" }], { duration: to ? 260 : 160, easing: "ease-in", fill: "forwards" }), 260);
  }
  wrap.hidden = true; wrap.closing = false;
  wrap.querySelectorAll("*").forEach((el) => el.getAnimations().forEach((a) => a.cancel()));
  wrap.dispatchEvent(new Event("panelclose"));   // anything the panel had running (the camera) stops
}
// The panel on top, if any (the last one opened that is still open).
export const openPanel = () => [...document.querySelectorAll(".modal")].filter((p) => !p.hidden && !p.closing).pop() || null;

/* Typing on a touch screen:
   1. nothing focuses a field for the reader (see showPanel, Edit and the Add-a-book steps);
   2. tapping a field's caption doesn't select it; only the field itself does (the search box, whose
      caption is its own magnifying glass, opens on a tap as it should);
   3. tapping anything that isn't a field puts the keyboard away;
   4. the field in use is brought to the top of the layer it is in (the book's page, the settings; other cards are short),
      so the keyboard never has anything to cover and iOS has no reason to move the screen. */
const TEXT = "input:not([type=checkbox]):not([type=radio]):not([type=range]):not([type=color]):not([type=file]), textarea, select";
const layerOf = (el) => el.closest(".t-page, .spane");
export function typingRules() {
  document.addEventListener("click", (e) => {
    if (!touch()) return;
    const lab = e.target.closest("label");
    if (lab && !lab.matches(".search .field") && !e.target.closest(TEXT) && lab.querySelector(TEXT)) e.preventDefault();
  }, true);
  document.addEventListener("pointerdown", (e) => {
    const a = document.activeElement;
    if (!touch() || !a?.matches?.(TEXT) || e.target.closest(TEXT + ", button, a")) return;
    a.blur();
  });
  document.addEventListener("focusin", (e) => {
    const f = e.target, layer = touch() && f.matches?.(TEXT) && layerOf(f);
    if (!layer) return;
    layer.classList.add("typing-room");
    layer.scrollTop += f.getBoundingClientRect().top - layer.getBoundingClientRect().top - 56;
  });
  document.addEventListener("focusout", (e) => {
    const layer = e.target.matches?.(TEXT) && layerOf(e.target);
    // The room stays until the tap that put the keyboard away has finished, so the page can't slide a field
    // under the finger and have that same tap select it.
    if (layer) setTimeout(() => { if (!layer.contains(document.activeElement) || !document.activeElement.matches(TEXT)) layer.classList.remove("typing-room"); }, 700);
  });
  // The on-screen keyboard covers the bottom of the layout without resizing it; its height is how far the
  // visual viewport falls short, and the cards lift by that much.
  if (window.visualViewport) {
    const vv = visualViewport;
    const kb = () => root.style.setProperty("--kb", Math.max(0, Math.round(innerHeight - vv.height - vv.offsetTop)) + "px");
    vv.addEventListener("resize", kb); vv.addEventListener("scroll", kb);
    addEventListener("focusin", () => [60, 250, 500].forEach((ms) => setTimeout(kb, ms)));
    addEventListener("focusout", () => setTimeout(kb, 250));
  }
  // Anything marked data-close closes the panel it is in.
  document.addEventListener("click", (e) => {
    const x = e.target.closest("[data-close]");
    if (x) hidePanel(x.closest(".modal"));
  });
}
