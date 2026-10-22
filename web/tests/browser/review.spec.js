import { test, expect } from "@playwright/test";
import { fileURLToPath } from "node:url";
const origin = "http://127.0.0.1:18323";
const headers = { Origin: origin };

test.beforeEach(async ({ page }) => {
  const login = await page.request.post("/api/session", { headers, data: { user: "admin", password: "review-browser-only" } });
  expect(login.ok()).toBeTruthy();
  await page.request.patch("/api/settings", { headers, data: { checkUpdates: false, backupSchedule: "never", sort: "author", sortDesc: false } });
  const library = await (await page.request.get("/api/library")).json();
  if (library.books.length < 300) {
    const books = Array.from({ length: 300 }, (_, i) => ({ title: `Book ${String(i).padStart(3, "0")} with a full title`, author: `Author ${String(i).padStart(3, "0")}`, pages: 200 + i % 300, binding: "paperback" }));
    const imported = await page.request.post("/api/import?name=review.json", { headers, data: books });
    expect(imported.ok()).toBeTruthy();
  }
  await page.goto("/");
  await expect(page.locator(".slot").nth(299)).toBeAttached();
  await page.evaluate(() => document.fonts.ready);
});

async function trace(page, title) {
  await page.evaluate((title) => {
    window.reviewTrace = { active: true, frames: [], visibility: [] };
    const reader = document.querySelector("#reader");
    window.reviewTrace.observer = new MutationObserver((changes) => {
      for (const change of changes) window.reviewTrace.visibility.push(change.oldValue === null ? "hide" : "show");
    });
    window.reviewTrace.observer.observe(reader, { attributes: true, attributeFilter: ["hidden"], attributeOldValue: true });
    const frame = () => {
      const trace = window.reviewTrace;
      if (!trace.active) return;
      const slot = title ? [...document.querySelectorAll(".slot")].find((el) => el.book?.title === title) : document.querySelector(".slot.out");
      const rect = slot?.getBoundingClientRect();
      const cover = document.querySelector("#lid-out img");
      trace.frames.push({ y: scrollY, hidden: document.querySelector("#reader").hidden, top: rect?.top, bottom: rect?.bottom, height: innerHeight, coverReady: !!cover?.complete && cover.naturalWidth > 0 });
      requestAnimationFrame(frame);
    };
    requestAnimationFrame(frame);
  }, title);
}
async function stopTrace(page, testInfo, name) {
  const { frames, visibility } = await page.evaluate(() => {
    window.reviewTrace.active = false; window.reviewTrace.observer.disconnect();
    return { frames: window.reviewTrace.frames, visibility: window.reviewTrace.visibility };
  });
  await testInfo.attach(`${name}-visibility`, { body: JSON.stringify(visibility), contentType: "application/json" });
  await testInfo.attach(name, { body: JSON.stringify(frames), contentType: "application/json" });
  return frames;
}
async function openNearBottom(page) {
  await page.locator(".slot").nth(240).click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  expect(await page.evaluate(() => scrollY)).toBeGreaterThan(1000);
}

test("closing holds the viewport on every frame; the overlay still locks background scrolling", async ({ page }, testInfo) => {
  for (const method of ["button", "escape", "backdrop"]) {
    await openNearBottom(page);
    const y = await page.evaluate(() => scrollY);
    expect(await page.evaluate(() => getComputedStyle(document.documentElement).overflowY)).toBe("hidden");
    await page.mouse.move(8, 60);
    await page.mouse.wheel(0, 800);
    await page.waitForTimeout(150);
    expect(await page.evaluate(() => scrollY)).toBeCloseTo(y, 0);
    await trace(page);
    if (method === "button") await page.locator("#close").click();
    else if (method === "escape") await page.keyboard.press("Escape");
    else await page.locator("#scrim").click({ position: { x: 4, y: 60 } });
    await expect(page.locator("#reader")).toBeHidden();
    await expect(page).toHaveURL(origin + "/");
    await page.waitForTimeout(200);
    const frames = await stopTrace(page, testInfo, `close-${method}`);
    expect(frames.length).toBeGreaterThan(5);
    expect(Math.max(...frames.map((f) => Math.abs(f.y - y)))).toBeLessThanOrEqual(1);
  }
});

for (const path of ["/", "/stats", "/notes", "/settings/library"]) test(`adding from ${path} settles the destination and opens the saved book`, async ({ page }, testInfo) => {
  await page.goto(path);
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.getByRole("button", { name: "Enter the details by hand", exact: true }).click();
  const title = `ZZZ Added ${testInfo.project.name} ${path}`;
  await page.locator('#adder input[name="title"]').fill(title);
  await page.locator('#adder input[name="author"]').fill("ZZZZ Last");
  await trace(page, title);
  await page.getByRole("button", { name: "Add to the shelf", exact: true }).click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  await expect(page.locator("#d-title")).toHaveText(title);
  await expect(page.locator("#reader")).not.toHaveClass(/solo/);
  const book = (await (await page.request.get("/api/library")).json()).books.find((b) => b.title === title);
  await expect(page).toHaveURL(origin + "/" + book.address);
  // A steady destination alone misses a closed-book preview that lands and opens a second time.
  expect(await page.evaluate(() => window.reviewTrace.visibility)).toEqual(["show"]);
  const frames = await stopTrace(page, testInfo, "arrival");
  const flying = frames.filter((f) => !f.hidden && f.top !== undefined);
  expect(flying.length).toBeGreaterThan(5);
  expect(flying.every((f) => f.bottom > 0 && f.top < f.height)).toBeTruthy();
  expect(Math.max(...flying.map((f) => f.top)) - Math.min(...flying.map((f) => f.top))).toBeLessThanOrEqual(1);
  await page.locator("#close").click();
  await expect(page.locator("#reader")).toBeHidden();
  await expect(page).toHaveURL(origin + "/");
  await page.goForward();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  await expect(page.locator("#d-title")).toHaveText(title);
  await page.goBack();
  await expect(page.locator("#reader")).toBeHidden();
});

test("a new book's cover is ready when its single opening starts", async ({ page }, testInfo) => {
  // The book is near the top, so a long scroll cannot accidentally mask an unloaded thumbnail.
  await page.route("**/covers/**", async (route) => {
    await new Promise((done) => setTimeout(done, 500));
    await route.continue();
  });
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.getByRole("button", { name: "Enter the details by hand", exact: true }).click();
  const title = `Cover ready ${testInfo.project.name}`;
  await page.locator('#adder input[name="title"]').fill(title);
  await page.locator('#adder input[name="author"]').fill("000 First");
  await page.locator('#adder input[type="file"]').setInputFiles(fileURLToPath(new URL("../fixtures/isbn-9780140328721.png", import.meta.url)));
  await trace(page, title);
  await page.getByRole("button", { name: "Add to the shelf", exact: true }).click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  expect(await page.evaluate(() => window.reviewTrace.visibility)).toEqual(["show"]);
  const frames = await stopTrace(page, testInfo, "opening-with-cover");
  const visible = frames.filter((frame) => !frame.hidden);
  expect(visible.length).toBeGreaterThan(5);
  expect(visible.every((frame) => frame.coverReady)).toBe(true);
});

test("addition clears a phone-width search and opens correctly with reduced motion", async ({ page }, testInfo) => {
  // Desktop search deliberately covers the other tools. The phone layout keeps Add visible.
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.locator('#search label').click();
  await page.locator("#q").fill("Book 001");
  await page.locator("#q").press("Tab");
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.getByRole("button", { name: "Enter the details by hand", exact: true }).click();
  const title = `AAA Reduced ${testInfo.project.name}`;
  await page.locator('#adder input[name="title"]').fill(title);
  await page.locator('#adder input[name="author"]').fill("AAAA First");
  await page.getByRole("button", { name: "Add to the shelf", exact: true }).click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  await expect(page.locator("#d-title")).toHaveText(title);
  await expect(page.locator("#q")).toHaveValue("");
  await expect(page.locator(".slot").nth(299)).toBeAttached();
});

test("Turkish letters remain distinct through adding and editing", async ({ page }, testInfo) => {
  const letters = "İstanbul İzmir Iğdır ışık Çığ ŞİİR ıİ iI çÇ ğĞ öÖ şŞ üÜ";
  const title = `${letters} ${testInfo.project.name}`;
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.getByRole("button", { name: "Enter the details by hand", exact: true }).click();
  for (const name of ["title", "author", "translator", "publisher"]) await page.locator(`#adder input[name="${name}"]`).fill(name === "title" ? title : letters);
  await page.getByRole("button", { name: "Add to the shelf", exact: true }).click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  await page.getByRole("button", { name: "More", exact: true }).click();
  await page.locator("#edit").click();
  for (const name of ["title", "author", "translator", "publisher"]) await expect(page.locator(`#pform [name="${name}"]`)).toHaveValue(name === "title" ? title : letters);
  await testInfo.attach("turkish-editor", { body: await page.screenshot(), contentType: "image/png" });
  const edited = letters + " — Iİıi";
  for (const name of ["title", "author", "translator", "publisher"]) await page.locator(`#pform [name="${name}"]`).fill(name === "title" ? title + " — Iİıi" : edited);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.locator("#reader")).not.toHaveClass(/editing/);
  const book = (await (await page.request.get("/api/library")).json()).books.find((b) => b.title === title + " — Iİıi");
  expect(book.author).toBe(edited); expect(book.translator).toBe(edited); expect(book.publisher).toBe(edited);
  // A changed title can change its address. Reopen the saved book in a fresh page load.
  await page.goto("/" + book.address);
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  await page.getByRole("button", { name: "More", exact: true }).click();
  await page.locator("#edit").click();
  for (const name of ["title", "author", "translator", "publisher"]) await expect(page.locator(`#pform [name="${name}"]`)).toHaveValue(name === "title" ? title + " — Iİıi" : edited);
});

// Previous wraps to the far end in one step; the long run also covers bounded browser history.
for (const { steps, direction } of [{ steps: 1, direction: "Previous" }, { steps: 55, direction: "Next" }]) test(`putting back after ${steps} ${direction} steps keeps the landing row visible`, async ({ page }, testInfo) => {
  const titles = await page.locator(".slot").evaluateAll((els) => els.map((el) => el.book.title));
  await page.locator(".slot").first().click();
  await expect(page.locator("#reader")).toHaveClass(/flat/);
  for (let i = 0; i < steps; i++) await page.getByRole("button", { name: `${direction} book`, exact: true }).click();
  await expect(page.locator("#d-title")).toHaveText(direction === "Previous" ? titles.at(-1) : titles[steps]);
  const title = await page.locator("#d-title").textContent();
  await trace(page, title);
  await page.locator("#close").click();
  await expect(page.locator("#reader")).toBeHidden();
  await expect(page).toHaveURL(origin + "/");
  await page.waitForTimeout(200);
  const frames = await stopTrace(page, testInfo, "close-after-stepping");
  const landing = frames.filter((f) => !f.hidden).at(-1);
  expect(landing).toBeTruthy();
  const after = frames.filter((f) => f.hidden);
  expect(after.length).toBeGreaterThan(2);
  expect(Math.max(...after.map((f) => Math.abs(f.y - landing.y)))).toBeLessThanOrEqual(1);
  expect(after.at(-1).bottom).toBeGreaterThan(0);
  expect(after.at(-1).top).toBeLessThan(after.at(-1).height);
});
