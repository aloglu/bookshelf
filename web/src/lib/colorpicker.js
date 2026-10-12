/* Any color: a small panel of the page's own under the swatch that opened it, with a field of shades, a hue
   bar and the hex code. (The browser's own picker opens wherever the window system puts it, which can be a
   corner of the screen.) While it is open, `input` hears every color as it is picked; `done` hears the
   last one once the panel closes, if it changed. One panel at a time; Esc, a tap elsewhere or a resize close it. */
let open = null;
export function pickColor(anchor, value, { input, done } = {}) {
  if (open?.anchor === anchor) { open.close(); return; }
  open?.close();
  let [h, s, v] = toHSV(value), last = value;
  const el = document.createElement("div");
  el.className = "picker"; el.setAttribute("role", "dialog"); el.setAttribute("aria-label", "Choose a color");
  el.innerHTML = `<div class="pk-sv"><i class="pk-knob"></i></div><div class="pk-hue"><i class="pk-knob"></i></div>
    <div class="pk-foot"><span class="pk-chip"></span><input class="inp" maxlength="7" spellcheck="false" autocomplete="off" aria-label="Hex code"></div>`;
  document.body.append(el);
  const [svEl, hueEl] = el.querySelectorAll(".pk-sv, .pk-hue"), field = el.querySelector("input");
  const show = () => {
    const c = toHex(h, s, v);
    el.style.setProperty("--hue", h); el.style.setProperty("--k", c);
    svEl.firstChild.style.cssText = `left:${s * 100}%;top:${(1 - v) * 100}%`;
    hueEl.firstChild.style.cssText = `left:${h / 3.6}%;--k:hsl(${h} 100% 50%)`;
    if (document.activeElement !== field) field.value = c;
  };
  const pick = () => { last = toHex(h, s, v); show(); input?.(last); };
  const drag = (area, move) => area.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    area.setPointerCapture(e.pointerId);
    const at = (ev) => { const r = area.getBoundingClientRect(); move(clamp((ev.clientX - r.left) / r.width), clamp((ev.clientY - r.top) / r.height)); pick(); };
    at(e);
    area.onpointermove = at;
    area.onpointerup = area.onpointercancel = () => { area.onpointermove = null; };
  });
  drag(svEl, (x, y) => { s = x; v = 1 - y; });
  drag(hueEl, (x) => { h = Math.min(359.9, x * 360); });
  field.addEventListener("input", () => {
    let t = field.value.trim().toLowerCase(); if (!t.startsWith("#")) t = "#" + t;
    if (/^#[0-9a-f]{3}$/.test(t)) t = "#" + [...t.slice(1)].map((x) => x + x).join("");
    if (/^#[0-9a-f]{6}$/.test(t)) { [h, s, v] = toHSV(t); pick(); }
  });
  // Under the swatch, kept on screen; above it when there's no room below.
  const r = anchor.getBoundingClientRect(), W = el.offsetWidth, H = el.offsetHeight;
  el.style.left = Math.max(12, Math.min(innerWidth - W - 12, r.left + r.width / 2 - W / 2)) + "px";
  el.style.top = (r.bottom + 8 + H > innerHeight - 12 ? Math.max(12, r.top - 8 - H) : r.bottom + 8) + "px";
  show();
  const away = (e) => { if (!el.contains(e.target) && !anchor.contains(e.target)) close(); };
  const esc = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopImmediatePropagation(); close(); anchor.focus(); } };
  function close() {
    if (open?.el !== el) return;
    open = null; el.remove();
    removeEventListener("pointerdown", away, true); removeEventListener("keydown", esc, true); removeEventListener("resize", close);
    if (last !== value) done?.(last);
  }
  addEventListener("pointerdown", away, true); addEventListener("keydown", esc, true); addEventListener("resize", close);
  open = { el, anchor, close };
}
const clamp = (x) => Math.min(1, Math.max(0, x));
function toHSV(hex) {
  const n = parseInt(hex.slice(1), 16), r = (n >> 16) / 255, g = ((n >> 8) & 255) / 255, b = (n & 255) / 255;
  const mx = Math.max(r, g, b), d = mx - Math.min(r, g, b);
  const h = d ? 60 * (mx === r ? ((g - b) / d + 6) % 6 : mx === g ? (b - r) / d + 2 : (r - g) / d + 4) : 0;
  return [h, mx ? d / mx : 0, mx];
}
const toHex = (h, s, v) => "#" + [5, 3, 1].map((n) => {
  const k = (n + h / 60) % 6;
  return Math.round(255 * (v - v * s * Math.max(0, Math.min(k, 4 - k, 1)))).toString(16).padStart(2, "0");
}).join("");
