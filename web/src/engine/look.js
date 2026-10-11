// How the library looks, from its settings: the walls, the wood, the bookcase, the ribbon, day or evening.
import { root } from "./model.js";

export const WOODS = { oak: "Oak", walnut: "Walnut", maple: "Maple", ash: "Ash", pine: "Pine", cherry: "Cherry", teak: "Teak", mahogany: "Mahogany", ebony: "Ebony" };
// The named wall pairs have hand-tuned palettes; linen is the default and needs no attribute.
export const PRESETS = { linen: ["#e9e0d0", "#17110d"], tobacco: ["#cdb08a", "#2a1c12"], chestnut: ["#94633f", "#3a2416"], umber: ["#5b4130", "#1e1510"], forest: ["#3d4c3e", "#152018"] };
export const presetOf = (day, night) => Object.keys(PRESETS).find((k) => PRESETS[k][0] === day && PRESETS[k][1] === night) || null;

export function applyLook(st) {
  applyTheme(st.theme);
  applyWalls(st.wallDay, st.wallNight);
  applyWoods(st.woodDay, st.woodNight);
  const parts = st.bookcase ? Object.keys(st.caseParts).filter((k) => st.caseParts[k]) : [];
  if (parts.length) root.dataset.case = parts.join(" "); else delete root.dataset.case;
  root.style.setProperty("--ribbon", st.ribbon);
  root.toggleAttribute("data-plain", !st.bindings);
}

export function applyTheme(theme) { if (theme === "system") delete root.dataset.theme; else root.dataset.theme = theme; }

/* The day wall and the evening wall can each be any color. For a color of your own, the lettering and
   panels on it flip between dark and light with how light the wall is, so the header stays readable on
   any wall. Worked in OKLCH so steps look even. */
const WALL_TOKENS = ["wall", "wall2", "ink", "muted", "surface", "line", "accent", "accent-ink"];
function oklch(hex) {
  const lin = (v) => (v /= 255) <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4;
  const [r, g, b] = [1, 3, 5].map((i) => lin(parseInt(hex.slice(i, i + 2), 16)));
  const l = Math.cbrt(.4122214708 * r + .5363325363 * g + .0514459929 * b), m = Math.cbrt(.2119034982 * r + .6806995451 * g + .1073969566 * b), s = Math.cbrt(.0883024619 * r + .2817188376 * g + .6299787005 * b);
  const L = .2104542553 * l + .793617785 * m - .0040720468 * s, A = 1.9779984951 * l - 2.428592205 * m + .4505937099 * s, B = .0259040371 * l + .7827717662 * m - .808675766 * s;
  return { l: L, c: Math.hypot(A, B), h: Math.atan2(B, A) };
}
const lch = (l, c, h) => `oklch(${(Math.min(1, Math.max(0, l)) * 100).toFixed(1)}% ${c.toFixed(3)} ${(h * 180 / Math.PI).toFixed(1)})`;
// One wall, by day ("l") or by evening ("d"): its lettering and panels follow from how light it is.
function wallSet(hex, m) {
  const { l, c, h } = oklch(hex), light = l > .64, k = (t) => `${t}-${m}`;
  const set = { [k("wall")]: hex, [k("wall2")]: lch(l - (light ? .045 : .035), c, h) };
  if (light) Object.assign(set, { [k("ink")]: lch(.26, Math.min(c, .03), h), [k("muted")]: lch(.46, Math.min(c, .05), h), [k("surface")]: lch(Math.min(l + .05, .97), c * .6, h), [k("line")]: "rgba(43, 31, 22, .17)" });
  else Object.assign(set, { [k("ink")]: lch(.95, Math.min(c, .022), h), [k("muted")]: lch(.8, Math.min(c, .04), h), [k("surface")]: lch(l + (l < .3 ? .05 : -.07), Math.min(c, .06), h), [k("line")]: "rgba(255, 245, 230, .18)" });
  if (m === "l") Object.assign(set, light ? { "accent-l": "#2f4f3c", "accent-ink-l": "#f3ead8" } : { "accent-l": "#e3c27c", "accent-ink-l": "#2a1c10" });
  return set;
}
function applyWalls(day, night) {
  WALL_TOKENS.forEach((t) => ["l", "d"].forEach((m) => root.style.removeProperty(`--${t}-${m}`)));
  const preset = presetOf(day, night);
  if (preset === "linen") { delete root.dataset.wall; return; }
  if (preset) { root.dataset.wall = preset; return; }
  root.dataset.wall = "custom";
  [[day, "l"], [night, "d"]].forEach(([hex, m]) => Object.entries(wallSet(hex, m)).forEach(([k, v]) => root.style.setProperty("--" + k, v)));
}

const chosenWood = {};
function applyWoods(day, night) {
  root.style.setProperty("--wood-day", `var(--w-${day})`);
  root.style.setProperty("--wood-night", `var(--w-${night})`);
  Object.assign(chosenWood, { day, night });
  for (const [m, name] of [["day", day], ["night", night]]) woodFaces(name).then((f) => {
    if (chosenWood[m] !== name) return;   // changed again while this one was drawn
    root.style.setProperty(`--wood-${m}-v`, f.v); root.style.setProperty(`--wood-${m}-back`, f.back);
  });
}
/* The sides and back of the bookcase come from the same strip of wood as the planks, drawn once per wood:
   - a side is the strip turned on end, so its grain runs up the upright;
   - the back is nine narrow boards side by side, each cut from a different stretch of the strip, turned on end and
     widened, with a dark seam between boards, so no two neighbouring boards repeat. */
const woodFaceCache = new Map();
function woodFaces(name) {
  if (!woodFaceCache.has(name)) woodFaceCache.set(name, (async () => {
    const url = getComputedStyle(root).getPropertyValue(`--w-${name}`).trim().replace(/^url\(["']?/, "").replace(/["']?\)$/, "");
    const im = new Image(); im.src = url; await im.decode();
    const W = im.width, H = im.height;
    const v = Object.assign(document.createElement("canvas"), { width: H, height: W });
    let g = v.getContext("2d"); g.translate(H, 0); g.rotate(Math.PI / 2); g.drawImage(im, 0, 0);
    const B = 72, n = 9, back = Object.assign(document.createElement("canvas"), { width: B * n, height: W });
    g = back.getContext("2d");
    for (let i = 0; i < n; i++) {
      const off = (i * 397) % W;
      g.save(); g.translate(B * i + B, 0); g.rotate(Math.PI / 2); g.scale(1, B / H);
      g.drawImage(im, -off, 0); g.drawImage(im, W - off, 0);
      g.restore();
      g.fillStyle = "rgba(0,0,0,.4)"; g.fillRect(B * i, 0, 1.5, W);
      g.fillStyle = "rgba(255,255,255,.08)"; g.fillRect(B * i + 1.5, 0, 1, W);
    }
    return { v: `url(${v.toDataURL("image/jpeg", .86)})`, back: `url(${back.toDataURL("image/jpeg", .82)})` };
  })());
  return woodFaceCache.get(name);
}

// Spine cloth: a fine noise drawn once, laid over every cloth binding.
export function cloth() {
  const c = document.createElement("canvas"); c.width = c.height = 96;
  const ctx = c.getContext("2d"), img = ctx.createImageData(96, 96), d = img.data;
  for (let i = 0; i < d.length; i += 4) { const v = Math.random(); d[i] = d[i + 1] = d[i + 2] = v > .5 ? 255 : 0; d[i + 3] = Math.abs(v - .5) * 34; }
  ctx.putImageData(img, 0, 0);
  root.style.setProperty("--cloth", `url("${c.toDataURL()}")`);
}
