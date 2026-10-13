// Inline Markdown for the library's name and its footer: links, **bold**, *italic*, `code`, and (in the
// footer) line breaks. Everything else is shown as typed: the text is escaped and only these become HTML,
// and a link goes only to a web address, an email address, or somewhere on this site.
const esc = (s) => s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
// Emphasis neither opens nor closes beside a space, so "2 * 3 * 4" stays as typed (as the server reads it).
const TOKENS = /`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|<((?:https?:\/\/|mailto:)[^>\s]+)>|\*\*([^\s*](?:.*?[^\s*])?)\*\*|__([^\s_](?:.*?[^\s_])?)__|\*([^\s*](?:.*?[^\s*])?)\*|(?<![\p{L}\p{N}_])_([^\s_](?:.*?[^\s_])?)_(?![\p{L}\p{N}_])/gu;
const safeURL = (u) => /^(https?:\/\/|mailto:|\/(?!\/)|#)/i.test(u);

function render(s) {
  let out = "", last = 0;
  for (const m of s.matchAll(TOKENS)) {
    out += esc(s.slice(last, m.index));
    last = m.index + m[0].length;
    const [, code, text, url, auto, strong1, strong2, em1, em2] = m;
    if (code) out += `<code>${esc(code)}</code>`;
    else if (text) out += safeURL(url) ? `<a href="${esc(url)}" rel="noopener">${render(text)}</a>` : render(text);
    else if (auto) out += `<a href="${esc(auto)}" rel="noopener">${esc(auto.replace(/^mailto:/, ""))}</a>`;
    else if (strong1 || strong2) out += `<strong>${render(strong1 || strong2)}</strong>`;
    else out += `<em>${render(em1 || em2)}</em>`;
  }
  return out + esc(s.slice(last));
}

// The name: one line.
export const inline = (md) => render(String(md || "").replace(/\s+/g, " ").trim());
// The footer: its lines kept.
export const lines = (md) => String(md || "").trim().split(/\r?\n/).map(render).join("<br>");
// As plain words, for the page title and anywhere a link can't go.
export function plain(md) {
  const d = document.createElement("div");
  d.innerHTML = inline(md);
  return d.textContent;
}
// Whether it holds a link of its own (the name then isn't wrapped in the link home).
export const hasLink = (md) => /<a /.test(inline(md));
