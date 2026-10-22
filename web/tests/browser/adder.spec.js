import { test, expect } from "@playwright/test";

for (const status of [404, 503]) test(`lookup ${status} leaves one clear manual-entry action`, async ({ page }, testInfo) => {
  const login = await page.request.post("/api/session", {
    headers: { Origin: "http://127.0.0.1:18323" }, data: { user: "admin", password: "review-browser-only" },
  });
  expect(login.ok()).toBeTruthy();
  const message = status === 404 ? "Open Library and Google Books don’t know this ISBN." : "The book sources are busy. Try again in a minute.";
  await page.route("**/api/lookup?*", (route) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify({ error: message }) }));
  await page.goto("/");
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.locator("#isbn").fill("9786256999190");
  await page.getByRole("button", { name: "Look up", exact: true }).click();
  await expect(page.locator('#adder [role="status"]')).toHaveText(message);
  const manual = page.getByRole("button", { name: /enter the details by hand/i });
  await expect(manual).toHaveCount(1);
  await page.screenshot({ path: testInfo.outputPath("lookup-message.png") });
  await manual.click();
  await expect(page.locator('#adder input[name="identifier"]')).toHaveValue("9786256999190");
  await expect(page.locator('#adder input[name="title"]')).toBeVisible();
});
