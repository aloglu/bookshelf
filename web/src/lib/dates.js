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
