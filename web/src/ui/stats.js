// The stats page's figures and charts, drawn into the page's own markup.
import { BINDING_NAMES, bindingOf, fmt, isPhone, plural } from "../engine/model.js";

const $ = (box, sel) => box.querySelector(sel);
const plot = (box, name) => box.querySelector(`[data-chart="${name}"] .plot`);

export function renderStats(box, list, { owner, hidden, openFromStats, setPage }) {
  const n = list.length;
  const pages = list.reduce((t, b) => t + (b.pages || 0), 0);
  const authors = new Set(list.map((b) => b.author)).size;
  const years = list.map((b) => b.published).filter(Boolean);
  // Books without a page count are left out of the page figures, and the tiles say so.
  const counted = list.filter((b) => b.pages).length, partly = counted < n;
  // Every tile has the same three lines: what it counts, the number, and a word on what the number means.
  const kpi = (label, value, note) => `<div class="kpi"><span>${label}</span><b>${value}</b><small>${note}</small></div>`;
  $(box, ".kpis").innerHTML = [
    kpi("Books", fmt(n), owner && hidden ? `${hidden} hidden from visitors` : "on the shelves"),
    kpi("Pages", fmt(pages), partly ? `in the ${fmt(counted)} with a page count` : `${fmt(Math.round(pages / 30 / 365 * 10) / 10)} years at 30 pages a day`),
    kpi("Authors", fmt(authors), `${fmt(Math.round(n / Math.max(1, authors) * 10) / 10)} books each on average`),
    kpi("Average length", fmt(Math.round(pages / Math.max(1, counted))), partly ? "pages, of those with a count" : "pages a book"),
    kpi("Written", years.length ? `${Math.min(...years)}–${Math.max(...years)}` : "—", years.length ? `${Math.max(...years) - Math.min(...years)} years from first to last` : "no dates yet"),
  ].join("");

  renderReadingCards(box, list, owner, openFromStats, setPage);
  // When they were written: the decades that have books, in order.
  const decades = new Map();
  [...years].sort((a, z) => a - z).forEach((y) => { const d = Math.floor(y / 10) * 10; decades.set(d, (decades.get(d) || 0) + 1); });
  columns(box, "decades", [...decades].map(([d, v]) => [`${d}s`, v]), "book", "fit");
  // How long they run: ordered page bands.
  const bands = [[1, 149], [150, 299], [300, 449], [450, 599], [600, 749], [750, Infinity]];
  columns(box, "lengths", bands.map(([a, z]) => [z === Infinity ? `${a}+` : `${a === 1 ? 0 : a}–${z}`, list.filter((b) => b.pages >= a && b.pages <= z).length, z === Infinity ? `${a}+` : String(a === 1 ? 0 : a)]), "book", () => true);
  // How they are bound.
  bars(box, "bindings", Object.keys(BINDING_NAMES).map((k) => [BINDING_NAMES[k], list.filter((b) => bindingOf(b) === k).length]).filter(([, v]) => v), "book");
  // Authors with two or more books, and the six biggest publishers.
  const count = (key) => [...list.reduce((m, b) => m.set(b[key], (m.get(b[key]) || 0) + 1), new Map())].filter(([k]) => k).sort((a, z) => z[1] - a[1] || a[0].localeCompare(z[0]));
  const many = count("author").filter(([, v]) => v > 1);
  bars(box, "authors", many.slice(0, 8), "book", many.length ? "" : "No author has more than one book here yet.");
  bars(box, "publishers", count("publisher").slice(0, 6), "book");
  // In translation.
  const tr = list.filter((b) => b.translator).length, share = n ? tr / n : 0;
  plot(box, "translation").innerHTML = `<div class="meter"><div class="big">${Math.round(share * 100)}%<small>${fmt(tr)} of ${fmt(n)} books</small></div><div class="bar" role="img" aria-label="${Math.round(share * 100)} percent"><i style="--v: ${share * 100}%"></i></div><p>The rest are in their original language, or their translator isn’t recorded.</p></div>`;
  // At the edges.
  const by = (f, dir) => list.filter((b) => f(b)).sort((a, z) => dir * (f(a) - f(z)))[0];
  const edges = [["Oldest", by((b) => b.published, 1), (b) => `First published ${b.published}`], ["Newest", by((b) => b.published, -1), (b) => `First published ${b.published}`],
    ["Longest", by((b) => b.pages, -1), (b) => `${fmt(b.pages)} pages`], ["Shortest", by((b) => b.pages, 1), (b) => `${fmt(b.pages)} pages`]].filter(([, b]) => b);
  const ex = plot(box, "extremes"); ex.innerHTML = `<div class="edges"></div>`;
  ex.firstChild.append(...edges.map(([label, b, fact]) => {
    const e = document.createElement("button"); e.type = "button"; e.className = "edge";
    e.innerHTML = `<img alt=""><small></small><b></b><span></span>`;
    e.querySelector("img").src = b.thumb; e.querySelector("small").textContent = label; e.querySelector("b").textContent = b.title; e.querySelector("span").textContent = fact(b);
    e.addEventListener("click", () => openFromStats(b, e));
    return e;
  }));
}

// The books being read: how far in (the owner keeps the page up to date here), when each was started,
// and where it stands among the rest of the library by length and by age.
function renderReadingCards(box, list, owner, openFromStats, setPage) {
  const now = list.filter((x) => x.reading).sort((a, b) => b.readingSince - a.readingSince);
  $(box, ".reading-block").hidden = !now.length;
  $(box, ".reading-cards").replaceChildren(...now.map((b) => readingCard(b, list, owner, openFromStats, setPage)));
}
function readingCard(b, list, owner, openFromStats, setPage) {
  const card = document.createElement("article");
  card.className = "chart reading-card";
  const share = (f) => Math.round(list.filter((x) => x !== b && f(x)).length / Math.max(1, list.length - 1) * 100);
  const page = Math.min(b.page || 0, b.pages || 0), pct = b.pages ? Math.round(page / b.pages * 100) : 0;
  const days = b.readingSince ? Math.max(0, Math.round((Date.now() - b.readingSince) / 864e5)) : null;
  const left = Math.max(0, (b.pages || 0) - page), plural2 = (n, w) => `${fmt(n)} ${w}${n === 1 ? "" : "s"}`;
  card.innerHTML = `<img alt=""><div class="body"><div class="who"><b></b><span></span></div>
    <div class="progress"><div class="line"><div class="big">${pct}%<small>${b.pages ? `page ${fmt(page)} of ${fmt(b.pages)}` : "page count unknown"}</small></div>
      ${owner && b.pages ? `<label class="page">On page <input type="number" min="0" max="${b.pages}" value="${page}" inputmode="numeric"></label>` : ""}</div>
      <div class="meter"><div class="bar" role="img" aria-label="${pct} percent read"><i style="--v: ${pct}%"></i></div></div></div>
    <ul class="facts">
      ${days !== null ? `<li><b>${days === 0 ? "Today" : plural2(days, "day")}</b>${days === 0 ? "started" : "since starting"}</li>` : ""}
      ${b.pages ? `<li><b>${fmt(left)}</b>pages to go${left ? ` · about ${plural2(Math.max(1, Math.round(left / 30)), "day")}` : ""}</li>
      <li><b>${share((x) => x.pages && x.pages < b.pages)}%</b>of the library is shorter</li>` : ""}
      ${b.published ? `<li><b>${share((x) => x.published && x.published > b.published)}%</b>of the library is older</li>` : ""}
    </ul></div>`;
  const img = card.querySelector("img"); img.src = b.thumb; img.addEventListener("click", () => openFromStats(b, img));
  card.querySelector(".who b").textContent = b.title; card.querySelector(".who span").textContent = b.author;
  card.querySelector(".page input")?.addEventListener("change", (e) => setPage(b, Math.max(0, Math.min(b.pages, Math.round(+e.target.value || 0)))));
  return card;
}

// A clean axis: steps of 1, 2, 5 or 10 so there are at most five gridlines.
function niceMax(v) { const step = [1, 2, 5, 10, 20, 50, 100, 200, 500].find((s) => v / s <= 5) || 1000; return { max: Math.max(step, Math.ceil(v / step) * step), step }; }
function columns(root, name, rows, unit, showTick) {
  const top = Math.max(0, ...rows.map(([, v]) => v)), { max, step } = niceMax(top), steps = max / step;
  const box = plot(root, name), peak = rows.findIndex(([, v]) => v === top && top > 0);
  box.dataset.table = JSON.stringify(rows);
  box.innerHTML = `<div class="cols" style="--steps: ${steps}"><div class="yax">${Array.from({ length: steps + 1 }, (_, i) => `<span>${fmt(i * step)}</span>`).join("")}</div><div class="area"></div><div class="xax"></div></div>`;
  const area = box.querySelector(".area"), xax = box.querySelector(".xax");
  rows.forEach(([label, v, short], i) => {
    const c = document.createElement("div"); c.className = "col"; c.tabIndex = 0;
    c.style.setProperty("--v", `${v / max * 100}%`);
    c.setAttribute("aria-label", `${label}: ${plural(v, unit)}`);
    c.innerHTML = `<i></i>`;
    if (i === peak) c.append(Object.assign(document.createElement("b"), { textContent: fmt(v) }));   // only the tallest is labelled
    tipOn(c, label, plural(v, unit));
    area.append(c);
    xax.append(Object.assign(document.createElement("span"), { textContent: showTick === "fit" || showTick(i) ? (isPhone() && short ? short : label) : "" }));   // a phone shows where each band starts
  });
  // "fit": label as many columns as there is room for. Every column is labelled when each label fits its
  // column; otherwise every second, third… so the labels that show never touch.
  if (showTick === "fit") {
    const spans = [...xax.children], colW = area.clientWidth / Math.max(1, rows.length);
    const probe = Object.assign(document.createElement("span"), { textContent: rows.reduce((w, [l]) => l.length > w.length ? l : w, "") });
    probe.style.cssText = "position:absolute;visibility:hidden;flex:none"; xax.append(probe);
    const every = Math.max(1, Math.ceil((probe.offsetWidth + 8) / Math.max(1, colW))); probe.remove();
    spans.forEach((sp, i) => { sp.textContent = i % every === 0 ? rows[i][0] : ""; });
  }
  if (box.closest(".chart").classList.contains("as-table")) asTable(box);
}
function bars(root, name, rows, unit, emptyNote = "") {
  const box = plot(root, name), max = Math.max(1, ...rows.map(([, v]) => v));
  box.dataset.table = JSON.stringify(rows);
  if (!rows.length) { box.innerHTML = `<p class="fine">${emptyNote || "Nothing to show yet."}</p>`; return; }
  box.innerHTML = `<div class="bars"></div>`;
  box.firstChild.append(...rows.flatMap(([label, v]) => {
    const nm = Object.assign(document.createElement("span"), { className: "name", textContent: label });
    const t = document.createElement("span"); t.className = "track"; t.innerHTML = `<i></i><b></b>`;
    t.querySelector("i").style.setProperty("--v", `${v / max * 100}%`); t.querySelector("b").textContent = fmt(v);
    return [nm, t];
  }));
  if (box.closest(".chart").classList.contains("as-table")) asTable(box);
}
function asTable(box) {
  const rows = JSON.parse(box.dataset.table || "[]"), t = document.createElement("table");
  t.innerHTML = `<thead><tr><th></th><th>Books</th></tr></thead><tbody></tbody>`;
  const tb = t.querySelector("tbody");
  rows.forEach(([label, v]) => { const tr = tb.insertRow(); tr.insertCell().textContent = label; tr.insertCell().textContent = fmt(v); });
  box.replaceChildren(t);
}
// One tooltip for every column: the value leads, the label follows. It only repeats what the table holds.
function tipOn(el, label, value) {
  const tip = document.getElementById("tip");
  const show = (x, y) => { tip.innerHTML = ""; tip.append(Object.assign(document.createElement("b"), { textContent: value }), document.createTextNode(label)); tip.hidden = false;
    const w = tip.offsetWidth; tip.style.left = Math.min(innerWidth - w - 8, Math.max(8, x - w / 2)) + "px"; tip.style.top = y - tip.offsetHeight - 10 + "px"; };
  el.addEventListener("pointermove", (e) => show(e.clientX, e.clientY));
  el.addEventListener("pointerleave", () => { tip.hidden = true; });
  el.addEventListener("focus", () => { const r = el.getBoundingClientRect(); show(r.left + r.width / 2, r.top); });
  el.addEventListener("blur", () => { tip.hidden = true; });
}
