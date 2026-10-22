import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { createContext, SourceTextModule, SyntheticModule } from "node:vm";

const source = await readFile(process.env.BOOKSHELF_SCAN_SOURCE || new URL("../src/lib/scan.js", import.meta.url), "utf8");
const valid = "9780140328721";
const result = [{ rawValue: valid }];
const flush = () => new Promise((r) => setImmediate(r));
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }

async function harness(options = {}) {
  const state = { requests: [], nativeCalls: 0, fallbackCalls: 0, stopped: 0 };
  const controller = new AbortController();
  const track = { readyState: "live", stop() { this.readyState = "ended"; state.stopped++; }, getSettings: () => ({ width: 1280, height: 720, frameRate: 30 }) };
  const stream = { getTracks: () => [track], getVideoTracks: () => [track] };
  const video = { srcObject: null, play: options.play || (async () => {}) };
  class Native {
    static async getSupportedFormats() { if (options.formatsError) throw options.formatsError; return ["ean_13"]; }
    constructor() { if (options.constructorError) throw options.constructorError; }
    async detect() { state.nativeCalls++; return options.nativeDetect ? options.nativeDetect(state, controller) : result; }
  }
  class Fallback {
    async detect() { state.fallbackCalls++; return options.fallbackDetect ? options.fallbackDetect(state, controller) : result; }
  }
  const context = createContext({
    window: { isSecureContext: options.secure !== false, ...(options.native === false ? {} : { BarcodeDetector: Native }) },
    navigator: { mediaDevices: { getUserMedia: async (request) => {
      state.requests.push(request);
      return options.camera ? options.camera(request, stream, state) : stream;
    } } },
    DOMException, console: { debug() {} },
    setTimeout: (fn, ms) => setTimeout(fn, Math.min(ms, 1)), clearTimeout,
  });
  const module = new SourceTextModule(source, {
    context,
    importModuleDynamically: async (specifier) => {
      if (options.moduleError) throw options.moduleError;
      let child;
      if (specifier === "barcode-detector/ponyfill") {
        child = new SyntheticModule(["BarcodeDetector", "prepareZXingModule"], function () {
          this.setExport("BarcodeDetector", Fallback); this.setExport("prepareZXingModule", () => {});
        }, { context });
      } else if (specifier === "zxing-wasm/reader/zxing_reader.wasm?url") {
        child = new SyntheticModule(["default"], function () { this.setExport("default", "/test.wasm"); }, { context });
      } else throw new Error(`Unexpected import ${specifier}`);
      await child.link(() => {}); await child.evaluate(); return child;
    },
  });
  await module.link(() => {}); await module.evaluate();
  return { ...module.namespace, state, controller, stream, track, video, run: () => module.namespace.scan(video, controller.signal) };
}

function released(h) { assert.equal(h.track.readyState, "ended"); assert.equal(h.video.srcObject, null); }

test("prefers HD without making it mandatory, and releases a successful scan", async () => {
  const h = await harness();
  assert.equal(await h.run(), valid);
  const request = JSON.parse(JSON.stringify(h.state.requests[0]));
  assert.deepEqual(request, { video: { facingMode: "environment", width: { ideal: 1920 }, height: { ideal: 1080 } }, audio: false });
  released(h);
});

test("an unsupported resolution request can fall back, permission denial cannot", async () => {
  const h = await harness({ camera: async (_, stream, state) => {
    if (state.requests.length === 1) throw new DOMException("Unsupported", "OverconstrainedError");
    return stream;
  } });
  assert.equal(await h.run(), valid);
  assert.equal(h.state.requests.length, 2);
  assert.equal(h.state.requests[1].video.width, undefined);
  released(h);
  const denied = await harness({ camera: async () => { throw new DOMException("Denied", "NotAllowedError"); } });
  await assert.rejects(denied.run(), { name: "NotAllowedError" });
  assert.equal(denied.state.requests.length, 1);
});

test("playback failure releases the camera and detaches the video", async () => {
  const h = await harness({ play: async () => { throw new DOMException("Playback failed", "NotSupportedError"); } });
  await assert.rejects(h.run(), { name: "NotSupportedError" }); released(h);
});

test("a failed fallback module is reported before requesting a camera", async () => {
  const h = await harness({ native: false, moduleError: new Error("WASM unavailable") });
  await assert.rejects(h.run(), { name: "BarcodeError" });
  assert.equal(h.state.requests.length, 0);
});

for (const cause of ["constructorError", "formatsError"]) test(`native ${cause} uses the bundled reader`, async () => {
  const h = await harness({ [cause]: new Error("Native unavailable") });
  assert.equal(await h.run(), valid); assert.equal(h.state.fallbackCalls, 1); released(h);
});

test("a native decode error switches backend without restarting the camera", async () => {
  const h = await harness({ nativeDetect: async () => { throw new Error("Native failure"); } });
  assert.equal(await h.run(), valid);
  assert.equal(h.state.nativeCalls, 1); assert.equal(h.state.fallbackCalls, 1); assert.equal(h.state.requests.length, 1); released(h);
});

test("sustained native misses get one fallback attempt", async () => {
  const h = await harness({ nativeDetect: async () => [] });
  assert.equal(await h.run(), valid);
  assert.equal(h.state.nativeCalls, 12); assert.equal(h.state.fallbackCalls, 1); released(h);
});

test("repeated fallback exceptions surface instead of scanning forever", { timeout: 1000 }, async () => {
  const h = await harness({ native: false, fallbackDetect: async () => { throw new Error("Broken decoder"); } });
  await assert.rejects(h.run(), { name: "BarcodeError" });
  assert.equal(h.state.fallbackCalls, 3); released(h);
});

test("ordinary misses continue until cancelled", async () => {
  const h = await harness({ native: false, fallbackDetect: async (state, controller) => {
    if (state.fallbackCalls === 4) controller.abort(); return [];
  } });
  await assert.rejects(h.run(), { name: "AbortError" });
  assert.equal(h.state.fallbackCalls, 4); released(h);
});

test("a camera granted after cancellation is stopped without attaching it", async () => {
  const permission = deferred();
  const h = await harness({ camera: () => permission.promise });
  const running = h.run();
  await flush(); assert.equal(h.state.requests.length, 1);
  h.controller.abort();
  await assert.rejects(running, { name: "AbortError" });
  permission.resolve(h.stream); await flush(); released(h);
});

test("cancelling a pending decoder releases the camera promptly and ignores its late result", async () => {
  const reading = deferred();
  const h = await harness({ nativeDetect: () => reading.promise });
  const running = h.run(); await flush(); assert.equal(h.state.nativeCalls, 1);
  h.controller.abort(); await assert.rejects(running, { name: "AbortError" }); released(h);
  reading.resolve(result); await flush(); released(h);
});

test("an old session does not detach a newer stream", async () => {
  const playback = deferred();
  const h = await harness({ play: () => playback.promise });
  const running = h.run(); await flush();
  const newer = {}; h.video.srcObject = newer;
  h.controller.abort(); await assert.rejects(running, { name: "AbortError" });
  assert.equal(h.video.srcObject, newer); assert.equal(h.track.readyState, "ended");
  playback.resolve(); await flush(); assert.equal(h.video.srcObject, newer);
});

test("valid book/periodical prefixes and checksum are both required", async () => {
  const h = await harness({ nativeDetect: async () => [{ rawValue: "9780140328722" }, { rawValue: "4006381333931" }, ...result] });
  assert.equal(await h.run(), valid); released(h);
  const periodical = "9771300288009";
  const p = await harness({ nativeDetect: async () => [{ rawValue: periodical }] });
  assert.equal(await p.run(), periodical); released(p);
});

test("insecure origins remain blocked", async () => {
  const h = await harness({ secure: false });
  assert.match(h.cameraProblem(), /https/);
});
