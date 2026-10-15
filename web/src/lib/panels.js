// Cards: how they come and go, which one Esc closes, and how typing works on a touch
// screen. One set of rules for every card. A card is pinned over the page, which is locked beneath it.
import { DIM, EASE, curve, dimIn, dimOut, isPhone, motionOK, settled, touch } from "../engine/model.js";

// How far a card at the foot of a phone's screen moves to be out of sight: from its top to the screen's foot,
// and a little more for its shadow (it floats clear of the bottom edge, so its own height isn't enough).
const below = (card) => innerHeight - card.getBoundingClientRect().top + 8;

// Where the page stood when the first card opened. To show a field above the keyboard Safari scrolls the page
// beneath a card, locked or not; nothing interferes while typing, and the page goes back as the card leaves.
let pageAt = null;
function putPageBack() {
  if (pageAt === null || openPanel()) return;
  if (scrollY !== pageAt) scrollTo({ top: pageAt, behavior: "instant" });
}
export function showPanel(wrap, focus) {
  if (!wrap.hidden && !wrap.closing) return;
  if (!openPanel()) pageAt = scrollY;
  wrap.hidden = false;
  const card = wrap.querySelector(".card");
  if (motionOK()) {
    const from = isPhone() ? `translateY(${below(card)}px)` : null;
    card.animate(from ? [{ transform: from }, { transform: "none" }] : [{ opacity: 0, transform: "translateY(12px) scale(.985)" }, { opacity: 1, transform: "none" }], { duration: from ? 400 : 260, easing: from ? curve : EASE });
  }
  dimIn(wrap.querySelector(".modal-scrim"));
  // On a touch screen the keyboard should only open from the reader's own tap, once the card has settled
  // (focusing a field as the card slides in makes iOS raise the keyboard over it); the card itself takes focus.
  const target = focus && wrap.querySelector(focus);
  if (target && touch() && target.matches("input, textarea, select")) { card.tabIndex = -1; card.focus({ preventScroll: true }); }
  else target?.focus({ preventScroll: true });
}
export async function hidePanel(wrap) {
  if (!wrap || wrap.hidden || wrap.closing) return;
  wrap.closing = true;
  wrap.dispatchEvent(new Event("panelhide"));   // it has begun to close
  if (wrap.contains(document.activeElement)) document.activeElement.blur();   // the keyboard goes as the card does
  putPageBack();   // under the dimming, as the card leaves
  const card = wrap.querySelector(".card");
  if (motionOK()) {
    const to = isPhone() ? `translateY(${below(card)}px)` : null;
    const fade = dimOut(wrap.querySelector(".modal-scrim"));
    await settled(card.animate(to ? [{ transform: "none" }, { transform: to }] : [{ opacity: 1 }, { opacity: 0, transform: "translateY(8px) scale(.985)" }], { duration: to ? 260 : 160, easing: "ease-in", fill: "forwards" }), 260);
    await settled(fade, DIM.out);   // the dimming finishes lifting after the card has gone
  }
  wrap.hidden = true; wrap.closing = false;
  putPageBack();   // in case the keyboard's going moved it again
  if (!openPanel()) pageAt = null;
  wrap.querySelectorAll("*").forEach((el) => el.getAnimations().forEach((a) => a.cancel()));
  wrap.dispatchEvent(new Event("panelclose"));   // anything the panel had running (the camera) stops
}
// The panel on top, if any (the last one opened that is still open).
export const openPanel = () => [...document.querySelectorAll(".modal")].filter((p) => !p.hidden && !p.closing).pop() || null;

/* Typing on a touch screen:
   1. nothing focuses a field for the reader (see showPanel, Edit and the Add-a-book steps);
   2. tapping a field's caption doesn't select it; only the field itself does (the search box, whose
      caption is its own magnifying glass, opens on a tap as it should);
   3. tapping anything that isn't a field puts the keyboard away.
   Nothing else: Safari brings the field being typed in into view itself, in a card or on the open book's page,
   and anything of ours that moved a card or a field for the keyboard fought it (tested on the owner's iPhone,
   2026-10-13). What Safari scrolls beneath goes back when the layer closes (see putPageBack, closeBook). */
// Every field (a dropdown opens the phone's own picker, a date its own).
const TEXT = "input:not([type=checkbox]):not([type=radio]):not([type=range]):not([type=color]):not([type=file]), textarea, select";
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
  // Anything marked data-close closes the panel it is in.
  document.addEventListener("click", (e) => {
    const x = e.target.closest("[data-close]");
    if (x) hidePanel(x.closest(".modal"));
  });
}
