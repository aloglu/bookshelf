// The server's API. Every answer is JSON; a problem comes back as { error } with a sentence for the owner.
class APIError extends Error {
  constructor(message, status, data) { super(message); this.status = status; this.data = data; }
}

export async function api(method, path, body) {
  const opts = { method, headers: {}, credentials: "same-origin" };
  if (body instanceof Blob || body instanceof FormData) opts.body = body;
  else if (body !== undefined) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
  let res;
  try {
    res = await fetch(path, opts);
  } catch {
    throw new APIError("Bookshelf can’t be reached. Check the connection and try again.", 0, null);
  }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new APIError(data?.error || `Something went wrong (${res.status}).`, res.status, data);
  return data;
}

// A plain-text answer (a Markdown export).
export async function apiText(path) {
  let res;
  try { res = await fetch(path, { credentials: "same-origin" }); } catch { throw new APIError("Bookshelf can’t be reached. Check the connection and try again.", 0, null); }
  if (!res.ok) throw new APIError(`Something went wrong (${res.status}).`, res.status, null);
  return res.text();
}

// A date as the server takes it: { date: "2015-06-01" | "2015" | "", precision }.
export const when = (precision, date = "") => ({ precision, date });
