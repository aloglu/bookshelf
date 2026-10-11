// Reading a book's barcode with the camera: the browser's own barcode reader where it has one,
// otherwise ZXing, loaded only when scanning starts and served by Bookshelf itself (never a CDN).
async function detector() {
  if ("BarcodeDetector" in window) {
    const formats = await window.BarcodeDetector.getSupportedFormats?.().catch(() => []);
    if (formats?.includes("ean_13")) return new window.BarcodeDetector({ formats: ["ean_13"] });
  }
  const [{ BarcodeDetector, prepareZXingModule }, { default: wasm }] = await Promise.all([
    import("barcode-detector/ponyfill"),
    import("zxing-wasm/reader/zxing_reader.wasm?url"),
  ]);
  prepareZXingModule({ overrides: { locateFile: (path, prefix) => (path.endsWith(".wasm") ? wasm : prefix + path) } });
  return new BarcodeDetector({ formats: ["ean_13"] });
}

// Why the camera can't be used here, if it can't.
export function cameraProblem() {
  if (!window.isSecureContext) return "The camera only works when Bookshelf is opened over https (or on this computer). Type the ISBN instead.";
  if (!navigator.mediaDevices?.getUserMedia) return "This browser can’t use the camera. Type the ISBN instead.";
  return "";
}

// Shows the camera in `video` and resolves with the first book barcode it reads: an ISBN (978, 979) or
// a periodical's ISSN barcode (977). Stops when `signal` aborts.
export async function scan(video, signal) {
  const [found, stream] = await Promise.all([detector(), navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" }, audio: false })]);
  const stop = () => stream.getTracks().forEach((t) => t.stop());
  if (signal.aborted) { stop(); throw new DOMException("Stopped", "AbortError"); }
  signal.addEventListener("abort", stop);
  video.srcObject = stream;
  await video.play();
  try {
    while (!signal.aborted) {
      const codes = await found.detect(video).catch(() => []);
      const hit = codes.find((c) => /^97[789]\d{10}$/.test(c.rawValue));
      if (hit) return hit.rawValue;
      await new Promise((r) => setTimeout(r, 180));
    }
    throw new DOMException("Stopped", "AbortError");
  } finally {
    stop();
    video.srcObject = null;
  }
}
