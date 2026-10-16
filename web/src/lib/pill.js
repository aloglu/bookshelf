// The segmented controls' highlight: one pill behind the buttons that slides to the chosen one. It is put in
// place without motion when the control first shows or changes size; only a new choice slides. It moves by a
// transform (across, and stretched to the chosen button's width from the width it was given when it was last
// put in place), so the slide runs on the compositor and keeps its pace while the page is busy, as when the
// shelf re-sorts at the same moment.
// A sort's arrow (.dir) has room on the chosen button only, so a new choice changes the buttons' widths. They
// take them at once, and the words in each glide from where they stood to where they stand now, by a transform
// too (a width growing frame by frame would be laid out on the busy page, and stutter), as the pill slides.
import { EASE, motionOK } from "../engine/model.js";

export function pill(seg) {
  const el = document.createElement("i");
  el.className = "pill"; el.setAttribute("aria-hidden", "true");
  seg.prepend(el);
  const chosen = () => seg.querySelector(':scope > button[aria-pressed="true"], :scope > button[aria-selected="true"]');
  const buttons = () => seg.querySelectorAll(":scope > button");
  // Where a button's words stand on screen (its first piece of text), and where each stood when last placed.
  const wordsAt = (btn) => {
    const t = [...btn.childNodes].find((n) => n.nodeType === Node.TEXT_NODE && n.textContent.trim());
    if (!t) return null;
    const r = document.createRange();
    r.selectNodeContents(t);
    return r.getBoundingClientRect().left;
  };
  let stood = new Map();
  function place(still) {
    const b = chosen();
    el.classList.toggle("none", !b);
    if (!b || !b.offsetWidth) return;
    // Exact positions (offsetLeft and offsetWidth round to whole pixels), measured on screen and taken back
    // to the control's own scale, since a card can still be scaling in as it opens.
    const box = b.getBoundingClientRect(), own = seg.getBoundingClientRect(), k = seg.offsetWidth / own.width;
    const x = (box.left - own.left) * k - seg.clientLeft, w = box.width * k;
    if (still) { el.classList.remove("ready"); el.style.setProperty("--w", w + "px"); }
    el.style.setProperty("--x", x + "px");
    el.style.setProperty("--sx", w / parseFloat(el.style.getPropertyValue("--w")));
    if (still) { el.offsetWidth; el.classList.add("ready"); }
    // A glide still running (a quick second choice) starts the next from where the words are on screen now.
    const gliding = new Map([...buttons()].map((btn) => {
      const off = parseFloat(getComputedStyle(btn).translate) || 0;
      btn.getAnimations().forEach((a) => a.id === "glide" && a.cancel());   // only the glide: the color's fade goes on
      return [btn, off];
    }));
    const now = new Map([...buttons()].map((btn) => [btn, wordsAt(btn)]));
    if (!still && motionOK()) {
      for (const [btn, at] of now) {
        const was = stood.get(btn), from = was == null || at == null ? 0 : (was - at) * k + gliding.get(btn);
        if (Math.abs(from) < .5) continue;
        btn.animate([{ translate: `${from}px 0` }, { translate: "0 0" }], { id: "glide", duration: 280, easing: EASE });
      }
    }
    stood = now;
  }
  const choice = new MutationObserver(() => place(false));
  choice.observe(seg, { subtree: true, childList: true, attributes: true, attributeFilter: ["aria-pressed", "aria-selected"] });
  const size = new ResizeObserver(() => place(true));
  size.observe(seg);
  place(true);
  return { destroy() { choice.disconnect(); size.disconnect(); el.remove(); } };
}
