// The segmented controls' highlight: one pill behind the buttons that slides to the chosen one. It is put in
// place without motion when the control first shows or changes size; only a new choice slides. It moves by a
// transform (across, and stretched to the chosen button's width from the width it was given when it was last
// put in place), so the slide runs on the compositor and keeps its pace while the page is busy, as when the
// shelf re-sorts at the same moment. A sort's arrow (.dir) takes its room on the chosen button only, opening
// over the same time as the slide while the last one's closes; the pill goes straight to where the chosen
// button will be once they have, worked out from how far each arrow has yet to grow or shrink.
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
    const own = seg.getBoundingClientRect(), k = seg.offsetWidth / own.width;
    let shift = 0, x = 0, w = 0;
    for (const btn of seg.querySelectorAll(":scope > button")) {
      const box = btn.getBoundingClientRect(), grow = arrowGrowth(btn);
      if (btn === b) { x = (box.left - own.left) * k - seg.clientLeft + shift; w = box.width * k + grow; }
      shift += grow;
    }
    if (still) { el.classList.remove("ready"); el.style.setProperty("--w", w + "px"); }
    el.style.setProperty("--x", x + "px");
    el.style.setProperty("--sx", w / parseFloat(el.style.getPropertyValue("--w")));
    if (still) { el.offsetWidth; el.classList.add("ready"); }
  }
  // How much wider a button's arrow is about to be (narrower, below 0): its room shown, less its room now.
  function arrowGrowth(btn) {
    const d = btn.querySelector(":scope > .dir");
    if (!d) return 0;
    const cs = getComputedStyle(d), now = parseFloat(cs.width) + parseFloat(cs.marginLeft);
    return (d.classList.contains("shown") ? parseFloat(getComputedStyle(seg).getPropertyValue("--dir-room")) || 0 : 0) - now;
  }
  const choice = new MutationObserver(() => place(false));
  choice.observe(seg, { subtree: true, childList: true, attributes: true, attributeFilter: ["aria-pressed", "aria-selected"] });
  // A real change of size (the window, the fonts arriving) puts it in place at once; the fraction of a pixel the
  // control wavers by while one arrow's room closes and another's opens doesn't, or it would cut the slide short.
  let width = 0;
  const size = new ResizeObserver(() => {
    const w = seg.getBoundingClientRect().width;
    if (Math.abs(w - width) < .5) return;
    width = w;
    place(true);
  });
  size.observe(seg);
  place(true);
  return { destroy() { choice.disconnect(); size.disconnect(); el.remove(); } };
}
