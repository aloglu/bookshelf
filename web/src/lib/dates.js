// Dates as readings keep them: an instant (when "Reading now" was chosen), a day, a year, or unknown.
// A day or a year is shown as it was entered, whatever the time zone.
const local = new Intl.DateTimeFormat("en-US", { month: "long", day: "numeric", year: "numeric" });
const utc = new Intl.DateTimeFormat("en-US", { month: "long", day: "numeric", year: "numeric", timeZone: "UTC" });
export const DAY = 864e5;

export function whenText(w) {
  if (!w || w.precision === "unknown" || !w.precision) return "";
  if (w.precision === "year") return w.time.slice(0, 4);
  return (w.precision === "day" ? utc : local).format(Date.parse(w.time));
}
export const known = (w) => !!w && (w.precision === "day" || w.precision === "instant");
export const ms = (w) => (w && w.precision !== "unknown" ? Date.parse(w.time) : null);
// The day a date falls on, as a date field shows it.
export function isoDay(w) {
  if (!known(w)) return "";
  if (w.precision === "day") return w.time.slice(0, 10);
  const d = new Date(Date.parse(w.time));
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
export const today = () => isoDay({ precision: "instant", time: new Date().toISOString() });
export const longDate = (t) => local.format(t);
// A short date ("Jul 27, 2025"): for an instant, the day it fell on here.
const shortLocal = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric" });
const shortUTC = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "UTC" });
export const shortDate = (t) => shortLocal.format(t);
export function shortWhen(w) {
  if (!known(w)) return whenText(w);
  return (w.precision === "day" ? shortUTC : shortLocal).format(Date.parse(w.time));
}
// Days read, counting the first and the last: begun and finished on one day is one day.
export function spanDays(a, z) {
  const noon = (w) => { const d = new Date(Date.parse(w.time)); return w.precision === "day" ? Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()) : Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()); };
  return Math.round((noon(z) - noon(a)) / DAY) + 1;
}
