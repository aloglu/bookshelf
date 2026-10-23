// Reading a book's barcode with the camera: the browser's own barcode reader where it has one,
// otherwise ZXing, loaded only when scanning starts and served by Bookshelf itself (never a CDN).
// ZXing compares its settings by identity and drops its compiled reader when they differ, so they are
// given once a visit: a second scan reuses the reader instead of compiling its megabyte again.
let zxing = null;
async function fallbackDetector() {
  try {
    zxing ??= Promise.all([import("barcode-detector/ponyfill"), import("zxing-wasm/reader/zxing_reader.wasm?url")]).then(([lib, { default: wasm }]) => {
      lib.prepareZXingModule({ overrides: { locateFile: (path, prefix) => (path.endsWith(".wasm") ? wasm : prefix + path) } });
      return lib.BarcodeDetector;
    });
    const BarcodeDetector = await zxing.catch((error) => { zxing = null; throw error; });   // a failed download can be tried again
    return { reader: new BarcodeDetector({ formats: ["ean_13"] }), native: false };
  } catch {
    throw barcodeError();
  }
}
async function detector() {
  try {
    const Native = window.BarcodeDetector;
    const formats = await Native?.getSupportedFormats?.();
    if (formats?.includes("ean_13")) return { reader: new Native({ formats: ["ean_13"] }), native: true };
  } catch { /* A broken native reader still has the bundled reader to fall back to. */ }
  return fallbackDetector();
}
const stopped = () => new DOMException("Stopped", "AbortError");
function barcodeError() {
  const error = new Error("The barcode reader could not read the camera.");
  error.name = "BarcodeError";
  return error;
}
// Camera permission, playback and decoding can all settle after the card closes.
function active(promise, signal) {
  return new Promise((resolve, reject) => {
    const abort = () => { signal.removeEventListener("abort", abort); reject(stopped()); };
    const finish = (fn, value) => { signal.removeEventListener("abort", abort); fn(value); };
    signal.addEventListener("abort", abort, { once: true });
    promise.then((value) => finish(resolve, value), (error) => finish(reject, error));
    if (signal.aborted) abort();
  });
}
function pause(signal) {
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(stopped()); };
    const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, 180);
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) { signal.removeEventListener("abort", abort); abort(); }
  });
}
function bookCode(value) {
  if (!/^97[789]\d{10}$/.test(value)) return false;
  return [...value].reduce((sum, n, i) => sum + Number(n) * (i % 2 ? 3 : 1), 0) % 10 === 0;
}

// Why the camera can't be used here, if it can't.
export function cameraProblem() {
  if (!window.isSecureContext) return "The camera only works when Bookshelf is opened over https (or on this computer). Type the ISBN instead.";
  if (!navigator.mediaDevices?.getUserMedia) return "This browser can’t use the camera. Type the ISBN instead.";
  return "";
}

// Prefer more captured detail for narrow bars. These are ideals: the browser can supply a smaller
// stream. Full intrinsic video frames still go to the decoder; the guide does not resample them.
export async function scan(video, signal) {
  if (signal.aborted) throw stopped();
  let backend = await active(detector(), signal);
  if (signal.aborted) throw stopped();
  let stream;
  const stopTracks = (s) => s.getTracks().forEach((t) => t.stop());
  const stop = () => { if (stream) stopTracks(stream); };
  const camera = async (constraints) => {
    const opening = navigator.mediaDevices.getUserMedia({ video: constraints, audio: false });
    // A late permission grant must be cleaned up even though scan() has already been cancelled.
    opening.then((s) => { if (signal.aborted) stopTracks(s); }, () => {});
    return active(opening, signal);
  };
  try {
    try {
      stream = await camera({ facingMode: "environment", width: { ideal: 1920 }, height: { ideal: 1080 } });
    } catch (error) {
      if (signal.aborted || error.name !== "OverconstrainedError") throw error;
      stream = await camera({ facingMode: "environment" });
    }
    if (signal.aborted) throw stopped();
    signal.addEventListener("abort", stop, { once: true });
    video.srcObject = stream;
    await active(video.play(), signal);
    const { width, height, frameRate } = stream.getVideoTracks()[0].getSettings();
    // No images or identifiers: useful when comparing a test device's actual capture settings.
    console.debug("Bookshelf scanner", { backend: backend.native ? "native" : "ZXing", width, height, frameRate });
    let errors = 0, misses = 0;
    while (!signal.aborted) {
      let codes;
      try {
        codes = await active(backend.reader.detect(video), signal);
        errors = 0;
      } catch (error) {
        if (signal.aborted) throw stopped();
        if (!backend.native && ++errors >= 3) throw barcodeError();
      }
      if (signal.aborted) throw stopped();
      const hit = codes?.find((c) => bookCode(c.rawValue));
      if (hit) return hit.rawValue;
      // Retry a native exception immediately, or sustained misses once, with the bundled reader.
      // A failing WASM reader surfaces an error; an ordinary miss keeps scanning until cancelled.
      if (backend.native && (!codes || ++misses >= 12)) {
        backend = await active(fallbackDetector(), signal);
        errors = 0;
        console.debug("Bookshelf scanner", { backend: "ZXing", reason: codes ? "native misses" : "native error" });
      }
      await pause(signal);
    }
    throw stopped();
  } finally {
    signal.removeEventListener("abort", stop);
    stop();
    if (stream && video.srcObject === stream) video.srcObject = null;
  }
}
