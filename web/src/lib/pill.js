// The segmented controls' highlight: one pill behind the buttons that slides to the chosen one. It is put in
// place without motion when the control first shows or changes size; only a new choice slides. It moves by a
// transform (across, and stretched to the chosen button's width from the width it was given when it was last
// put in place), so the slide runs on the compositor and keeps its pace while the page is busy, as when the
// shelf re-sorts at the same moment.
export function pill(seg) {
  const el = document.createElement("i");
  el.className = "pill"; el.setAttribute("aria-hidden", "true");
  seg.prepend(el);
  const chosen = () => seg.querySelector(':scope > button[aria-pressed="true"], :scope > button[aria-selected="true"]');
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
  }
  const choice = new MutationObserver(() => place(false));
  choice.observe(seg, { subtree: true, childList: true, attributes: true, attributeFilter: ["aria-pressed", "aria-selected"] });
  const size = new ResizeObserver(() => place(true));
  size.observe(seg);
  place(true);
  return { destroy() { choice.disconnect(); size.disconnect(); el.remove(); } };
}
