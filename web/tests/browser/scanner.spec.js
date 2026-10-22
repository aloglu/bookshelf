import { readFile } from "node:fs/promises";
import { test, expect } from "@playwright/test";

async function signIn(page) {
  const login = await page.request.post("/api/session", {
    headers: { Origin: "http://127.0.0.1:18323" }, data: { user: "admin", password: "review-browser-only" },
  });
  expect(login.ok()).toBeTruthy();
  await page.goto("/");
  await page.getByRole("button", { name: "Add book", exact: true }).click();
  await page.getByRole("button", { name: "Scan barcode", exact: true }).click();
}

test("the bundled WASM reader decodes a real video frame and releases the track", async ({ page }) => {
  // Synthetic image/camera: this validates the production integration, not physical-phone accuracy.
  const image = await readFile(new URL("../fixtures/isbn-9780140328721.png", import.meta.url));
  await page.route("**/scanner-fixture.png", (route) => route.fulfill({ body: image, contentType: "image/png" }));
  await page.route("**/api/lookup?*", (route) => route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ message: "Fixture has no catalogue entry" }) }));
  await page.addInitScript(() => {
    Object.defineProperty(window, "BarcodeDetector", { configurable: true, value: undefined });
    navigator.mediaDevices.getUserMedia = async () => {
      const img = new Image(); img.src = "/scanner-fixture.png"; await img.decode();
      const canvas = document.createElement("canvas"); canvas.width = img.width; canvas.height = img.height;
      const ctx = canvas.getContext("2d"); ctx.drawImage(img, 0, 0);
      const stream = canvas.captureStream(10);
      window.scannerTracks = stream.getTracks();
      const timer = setInterval(() => {
        if (window.scannerTracks.every((track) => track.readyState === "ended")) clearInterval(timer);
        else ctx.drawImage(img, 0, 0);
      }, 100);
      return stream;
    };
  });
  await signIn(page);
  await expect(page.locator("#isbn")).toHaveValue("9780140328721");
  await expect.poll(() => page.evaluate(() => window.scannerTracks?.every((track) => track.readyState === "ended"))).toBe(true);
  expect(await page.locator("#adder video").evaluate((video) => video.srcObject)).toBeNull();
});

test("close and reopen during permission does not let the old scan own the new session", async ({ page }) => {
  await page.addInitScript(() => {
    window.scannerTracks = []; window.cameraRequests = 0; window.decodeCalls = 0;
    Object.defineProperty(window, "BarcodeDetector", { configurable: true, value: class {
      static async getSupportedFormats() { return ["ean_13"]; }
      detect() { window.decodeCalls++; return new Promise(() => {}); }
    } });
    navigator.mediaDevices.getUserMedia = () => {
      window.cameraRequests++;
      const canvas = document.createElement("canvas"); canvas.width = 640; canvas.height = 480;
      const ctx = canvas.getContext("2d"); ctx.fillRect(0, 0, 640, 480);
      const stream = canvas.captureStream(10), track = stream.getVideoTracks()[0];
      window.scannerTracks.push(track);
      const timer = setInterval(() => {
        if (track.readyState === "ended") clearInterval(timer); else ctx.fillRect(0, 0, 640, 480);
      }, 100);
      if (window.cameraRequests === 1) return new Promise((resolve) => { window.grantFirst = () => resolve(stream); });
      return Promise.resolve(stream);
    };
  });
  await signIn(page);
  await expect.poll(() => page.evaluate(() => window.cameraRequests)).toBe(1);
  await page.getByRole("button", { name: "Type it instead", exact: true }).click();
  await page.getByRole("button", { name: "Scan barcode", exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.decodeCalls)).toBe(1);
  await page.evaluate(() => window.grantFirst());
  await expect.poll(() => page.evaluate(() => window.scannerTracks[0].readyState)).toBe("ended");
  expect(await page.evaluate(() => window.scannerTracks[1].readyState)).toBe("live");
  await page.locator("#adder .x").click();
  await expect.poll(() => page.evaluate(() => window.scannerTracks.every((track) => track.readyState === "ended"))).toBe(true);
  await expect(page.locator("#adder")).toBeHidden();
});
