// The segmented controls' highlight: one pill behind the buttons that slides to the chosen one. It is put in
// place without motion when the control first shows or changes size; only a new choice slides.
export function pill(seg) {
  const el = document.createElement("i");
  el.className = "pill"; el.setAttribute("aria-hidden", "true");
  seg.prepend(el);
  const chosen = () => seg.querySelector(':scope > button[aria-pressed="true"], :scope > button[aria-selected="true"]');
  function place(still) {
    const b = chosen();
    el.classList.toggle("none", !b);
    if (!b || !b.offsetWidth) return;
    if (still) el.classList.remove("ready");
    el.style.setProperty("--x", b.offsetLeft + "px");
    el.style.setProperty("--w", b.offsetWidth + "px");
    if (still) { el.offsetWidth; el.classList.add("ready"); }
  }
  const choice = new MutationObserver(() => place(false));
  choice.observe(seg, { subtree: true, childList: true, attributes: true, attributeFilter: ["aria-pressed", "aria-selected"] });
  const size = new ResizeObserver(() => place(true));
  size.observe(seg);
  place(true);
  return { destroy() { choice.disconnect(); size.disconnect(); el.remove(); } };
}
