#!/usr/bin/env node

import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createServer } from "node:http";
import {
  accessSync,
  constants,
  createReadStream,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, extname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const requireBrowser = process.argv.includes("--require-browser");
const temporaryRoot = mkdtempSync(join(tmpdir(), "bookshelf-browser-"));
let browser;
let server;

const fail = (message) => {
  throw new Error(message);
};

const waitWithTimeout = async (promise, milliseconds) => {
  let timer;
  const timeout = new Promise((resolveTimeout) => {
    timer = setTimeout(resolveTimeout, milliseconds);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    clearTimeout(timer);
  }
};

const findBrowser = () => {
  const candidates = [
    process.env.CHROME_BIN,
    "chromium",
    "chromium-browser",
    "google-chrome",
    "google-chrome-stable",
  ].filter(Boolean);
  for (const candidate of candidates) {
    if (candidate.includes(sep)) {
      try {
        accessSync(candidate, constants.X_OK);
        return candidate;
      } catch {
        continue;
      }
    }
    const found = spawnSync("sh", ["-c", `command -v "$1"`, "sh", candidate], {
      encoding: "utf8",
    });
    if (found.status === 0 && found.stdout.trim()) {
      return found.stdout.trim();
    }
  }
  return "";
};

const run = (command, args, options = {}) =>
  execFileSync(command, args, {
    cwd: repoRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    ...options,
  });

const buildFixture = () => {
  const binary = join(temporaryRoot, "bookshelf");
  const libraryRoot = join(temporaryRoot, "library");
  const emptyLibraryRoot = join(temporaryRoot, "empty-library");
  const input = join(temporaryRoot, "books.json");
  const environment = {
    ...process.env,
    BOOKSHELF_INSTALL_DIR: libraryRoot,
    GOCACHE: process.env.GOCACHE || join(temporaryRoot, "go-cache"),
  };
  run("go", ["build", "-o", binary, "./cmd/bookshelf"], { env: environment });
  run(binary, ["_init"], { env: environment });
  writeFileSync(
    input,
    `${JSON.stringify(
      [
        {
          title: "NASA and iPhone · İstanbul",
          author: "e.e. cummings",
          isbn: "978-0-00-000000-0",
        },
        {
          title: "A Portrait of the Artist as a Young Man",
          author: "James Joyce",
          isbn: "978-0143108245",
        },
        {
          title: "Mars: The Pristine Beauty of the Red Planet",
          author: "Alfred S. McEwen",
        },
      ],
      null,
      2,
    )}\n`,
  );
  run(binary, ["import", input, "--no-build"], { env: environment });
  run(binary, ["build"], { env: environment });
  const emptyEnvironment = {
    ...environment,
    BOOKSHELF_INSTALL_DIR: emptyLibraryRoot,
  };
  run(binary, ["_init"], { env: emptyEnvironment });
  run(binary, ["build"], { env: emptyEnvironment });
  return {
    populated: join(libraryRoot, "public"),
    empty: join(emptyLibraryRoot, "public"),
  };
};

const contentTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".webp": "image/webp",
  ".woff2": "font/woff2",
};

const serve = (publicRoot) =>
  new Promise((resolveServer, reject) => {
    const canonicalRoot = realpathSync(publicRoot);
    const instance = createServer((request, response) => {
      try {
        const pathname = decodeURIComponent(new URL(request.url, "http://localhost").pathname);
        const requested = resolve(canonicalRoot, `.${pathname === "/" ? "/index.html" : pathname}`);
        if (requested !== canonicalRoot && !requested.startsWith(canonicalRoot + sep)) {
          response.writeHead(404).end();
          return;
        }
        if (!statSync(requested).isFile()) {
          response.writeHead(404).end();
          return;
        }
        response.setHeader("Content-Type", contentTypes[extname(requested)] || "application/octet-stream");
        createReadStream(requested).pipe(response);
      } catch {
        response.writeHead(404).end();
      }
    });
    instance.once("error", reject);
    instance.listen(0, "127.0.0.1", () => resolveServer(instance));
  });

const launchBrowser = (executable) =>
  new Promise((resolveBrowser, reject) => {
    const profile = join(temporaryRoot, "browser-profile");
    const process = spawn(
      executable,
      [
        "--headless=new",
        "--no-sandbox",
        "--disable-gpu",
        "--disable-breakpad",
        "--remote-debugging-port=0",
        `--user-data-dir=${profile}`,
        "about:blank",
      ],
      { stdio: ["ignore", "ignore", "pipe"] },
    );
    let stderr = "";
    const timeout = setTimeout(() => {
      process.kill("SIGKILL");
      reject(new Error(`browser did not expose its debugging endpoint:\n${stderr}`));
    }, 15000);
    process.stderr.setEncoding("utf8");
    process.stderr.on("data", (chunk) => {
      stderr += chunk;
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timeout);
        resolveBrowser({ process, browserWebSocket: match[1] });
      }
    });
    process.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    process.once("exit", (code) => {
      if (!stderr.includes("DevTools listening on")) {
        clearTimeout(timeout);
        reject(new Error(`browser exited with status ${code}:\n${stderr}`));
      }
    });
  });

const stopBrowser = async (instance) => {
  const hasExited = () =>
    instance.process.exitCode !== null || instance.process.signalCode !== null;
  if (!instance?.process || hasExited()) return;

  const exited = new Promise((resolveExit) => instance.process.once("exit", resolveExit));
  try {
    const devtools = new DevTools(instance.browserWebSocket);
    await devtools.open();
    await waitWithTimeout(devtools.send("Browser.close"), 1000);
  } catch {
    // Fall through to the process-level shutdown below.
  }

  await waitWithTimeout(exited, 5000);
  if (!hasExited()) {
    instance.process.kill("SIGKILL");
    await exited;
  }
};

class DevTools {
  constructor(url) {
    this.nextID = 1;
    this.pending = new Map();
    this.events = new Map();
    this.socket = new WebSocket(url);
  }

  async open() {
    await new Promise((resolveSocket, reject) => {
      this.socket.addEventListener("open", resolveSocket, { once: true });
      this.socket.addEventListener("error", reject, { once: true });
    });
    this.socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.id) {
        const pending = this.pending.get(message.id);
        if (!pending) return;
        this.pending.delete(message.id);
        if (message.error) pending.reject(new Error(message.error.message));
        else pending.resolve(message.result);
        return;
      }
      for (const listener of this.events.get(message.method) || []) {
        listener(message.params);
      }
    });
  }

  send(method, params = {}) {
    const id = this.nextID++;
    return new Promise((resolveCommand, reject) => {
      this.pending.set(id, { resolve: resolveCommand, reject });
      this.socket.send(JSON.stringify({ id, method, params }));
    });
  }

  on(method, listener) {
    const listeners = this.events.get(method) || [];
    listeners.push(listener);
    this.events.set(method, listeners);
  }

  waitFor(method, timeoutMilliseconds = 10000) {
    return new Promise((resolveEvent, reject) => {
      const timeout = setTimeout(() => reject(new Error(`timed out waiting for ${method}`)), timeoutMilliseconds);
      this.on(method, (params) => {
        clearTimeout(timeout);
        resolveEvent(params);
      });
    });
  }

  close() {
    this.socket.close();
  }
}

const pageWebSocket = async (browserWebSocket) => {
  const endpoint = new URL(browserWebSocket);
  const targets = await fetch(`http://${endpoint.host}/json/list`).then((response) => response.json());
  const page = targets.find((target) => target.type === "page");
  if (!page?.webSocketDebuggerUrl) fail("browser did not create a page target");
  return page.webSocketDebuggerUrl;
};

const evaluate = async (devtools, expression) => {
  const result = await devtools.send("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    fail(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
  }
  return result.result.value;
};

const waitUntil = async (devtools, expression, description) => {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    if (await evaluate(devtools, expression)) return;
    await new Promise((resolveDelay) => setTimeout(resolveDelay, 50));
  }
  fail(`timed out waiting for ${description}`);
};

const runChecks = async (url, browserWebSocket) => {
  const devtools = new DevTools(await pageWebSocket(browserWebSocket));
  await devtools.open();
  const errors = [];
  devtools.on("Runtime.exceptionThrown", ({ exceptionDetails }) => {
    errors.push(exceptionDetails.exception?.description || exceptionDetails.text);
  });
  devtools.on("Runtime.consoleAPICalled", ({ type, args }) => {
    if (type === "error") errors.push(args.map((argument) => argument.value || argument.description).join(" "));
  });
  await devtools.send("Runtime.enable");
  await devtools.send("Page.enable");
  const loaded = devtools.waitFor("Page.loadEventFired");
  await devtools.send("Page.navigate", { url });
  await loaded;

  await waitUntil(devtools, `document.querySelectorAll(".book").length === 3`, "initial book rendering");
  const initial = await evaluate(
    devtools,
    `({
      titles: [...document.querySelectorAll(".book .title")].map((element) => element.textContent),
      shelf: document.querySelector("#bookshelf").classList.contains("force-shelf")
    })`,
  );
  if (!initial.titles.includes("NASA and iPhone · İstanbul")) fail("title capitalization changed in the browser");
  if (!initial.shelf) fail("desktop Shelf layout was not initialized");

  const shelfInteraction = await evaluate(
    devtools,
    `(() => {
      const shelf = document.querySelector("#bookshelf");
      shelf.style.width = "260px";
      shelf.style.maxWidth = "260px";
      const books = [...document.querySelectorAll(".book")];
      books[1].click();
      return new Promise((resolve) => setTimeout(() => {
        const bookCenter = books[1].offsetLeft + books[1].offsetWidth / 2;
        const viewportCenter = shelf.scrollLeft + shelf.clientWidth / 2;
        const centered = Math.abs(bookCenter - viewportCenter) < 3;
        window.stopShelfScroll?.();
        shelf.scrollLeft = shelf.scrollWidth - shelf.clientWidth;
        const beforeWheel = shelf.scrollLeft;
        shelf.dispatchEvent(new WheelEvent("wheel", {
          deltaY: -300,
          bubbles: true,
          cancelable: true,
        }));
        setTimeout(() => {
          const tilted = [...books].some((book) =>
            Math.abs(parseFloat(book.style.getPropertyValue("--tilt")) || 0) > 0.05
          );
          const afterWheel = shelf.scrollLeft;
          const shelfAnimation = { ...window.shelfState };
          const nativePosition = shelf.scrollWidth - shelf.clientWidth;
          shelf.scrollLeft = nativePosition;
          shelf.dispatchEvent(new Event("scroll"));
          setTimeout(() => {
            const nativeScrollPreserved = Math.abs(shelf.scrollLeft - nativePosition) < 3;
            window.stopShelfScroll?.();
            shelf.style.width = "";
            shelf.style.maxWidth = "";
            resolve({
              centered,
              nativeScrollPreserved,
              tilted,
              reducedMotion: matchMedia("(prefers-reduced-motion: reduce)").matches,
              tiltValues: books.map((book) => book.style.getPropertyValue("--tilt")),
              beforeWheel,
              afterWheel,
              shelfAnimation,
            });
          }, 250);
        }, 30);
      }, 700));
    })()`,
  );
  if (!shelfInteraction.centered || !shelfInteraction.nativeScrollPreserved ||
      !shelfInteraction.tilted) {
    fail(`Shelf interaction regressed: ${JSON.stringify(shelfInteraction)}`);
  }

  await evaluate(
    devtools,
    `(() => {
      const input = document.querySelector(".search-input");
      input.value = "portrait of";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    })()`,
  );
  await waitUntil(devtools, `document.querySelectorAll(".book").length === 1`, "search filtering");
  const searchTitle = await evaluate(devtools, `document.querySelector(".book .title").textContent`);
  if (searchTitle !== "A Portrait of the Artist as a Young Man") fail(`unexpected search result: ${searchTitle}`);

  await evaluate(
    devtools,
    `(() => {
      const input = document.querySelector(".search-input");
      input.value = "";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    })()`,
  );
  await waitUntil(devtools, `document.querySelectorAll(".book").length === 3`, "cleared search");
  const detailMatches = await evaluate(
    devtools,
    `(() => {
      const book = document.querySelector(".book");
      const title = book.querySelector(".title").textContent;
      book.click();
      return new Promise((resolve) => setTimeout(() => resolve(
        !document.querySelector("#details").hidden &&
        document.querySelector(".details-title").textContent === title &&
        location.hash.length > 1
      ), 50));
    })()`,
  );
  if (!detailMatches) fail("book details or permalink did not open correctly");

  for (const view of ["stack", "coverflow", "shelf"]) {
    await evaluate(devtools, `document.querySelector('#view-control [data-view="${view}"]').click()`);
    await waitUntil(devtools, `document.body.classList.contains("view-${view}")`, `${view} view`);
    if (view === "stack") {
      const animated = await evaluate(devtools, `document.querySelector("#bookshelf").classList.contains("view-animate")`);
      if (animated) fail("Stack view retained the view-change animation");
    }
    if (view === "coverflow") {
      await waitUntil(
        devtools,
        `window.cfState && !window.cfState.isAnimating &&
          window.cfState.cacheVersion === window.bookshelfRenderVersion`,
        "idle Coverflow",
      );
      const initialIndex = await evaluate(devtools, `window.cfState.index`);
      await evaluate(
        devtools,
        `document.querySelector("#bookshelf").dispatchEvent(
          new WheelEvent("wheel", { deltaY: 240, bubbles: true, cancelable: true })
        )`,
      );
      await waitUntil(
        devtools,
        `window.cfState.index > ${JSON.stringify(initialIndex)}`,
        "Coverflow wheel movement",
      );
      await waitUntil(
        devtools,
        `!window.cfState.isAnimating && window.cfState.targetIndex === null`,
        "Coverflow returning to idle",
      );
      await waitUntil(
        devtools,
        `document.querySelector(".coverflow-details")?.classList.contains("visible") &&
          document.querySelector(".coverflow-details").textContent.trim().length > 0`,
        "Coverflow delayed details",
      );
      const stableCoverflowLink = await evaluate(
        devtools,
        `(() => {
          const active = document.querySelector(".book.is-3d-active");
          active.click();
          const firstHash = location.hash;
          active.click();
          return firstHash.length > 1 && location.hash === firstHash;
        })()`,
      );
      if (!stableCoverflowLink) fail("clicking the active Coverflow book cleared its permalink");
      await evaluate(
        devtools,
        `(() => {
          window.stackTransformTransitions = 0;
          document.querySelectorAll(".book").forEach((book) => {
            book.addEventListener("transitionrun", (event) => {
              if (event.propertyName === "transform") window.stackTransformTransitions++;
            });
          });
          document.querySelector('#view-control [data-view="stack"]').click();
        })()`,
      );
      await waitUntil(devtools, `document.body.classList.contains("view-stack")`, "Coverflow to Stack view");
      await new Promise((resolveDelay) => setTimeout(resolveDelay, 100));
      const stackTransitions = await evaluate(devtools, `window.stackTransformTransitions`);
      if (stackTransitions !== 0) {
        fail(`Coverflow-to-Stack transform transition count: ${stackTransitions}`);
      }
    }
  }

  const historyFixture = await evaluate(
    devtools,
    `(() => {
      window.setActive(null, false, "none");
      history.replaceState(null, "", location.pathname + location.search);
      const books = [...document.querySelectorAll(".book")];
      books[0].click();
      const first = { id: books[0].dataset.id, hash: location.hash };
      books[1].click();
      return { first, second: { id: books[1].dataset.id, hash: location.hash } };
    })()`,
  );
  if (!historyFixture.first.hash || !historyFixture.second.hash ||
      historyFixture.first.hash === historyFixture.second.hash) {
    fail(`book history entries were not created: ${JSON.stringify(historyFixture)}`);
  }
  await evaluate(devtools, "history.back()");
  await waitUntil(
    devtools,
    `window.activeId === ${JSON.stringify(historyFixture.first.id)} &&
      location.hash === ${JSON.stringify(historyFixture.first.hash)}`,
    "Back navigation to the previous book",
  );
  await evaluate(devtools, "history.back()");
  await waitUntil(devtools, `window.activeId === null && location.hash === ""`, "Back navigation to the library");
  await evaluate(devtools, "history.forward()");
  await waitUntil(
    devtools,
    `window.activeId === ${JSON.stringify(historyFixture.first.id)} &&
      location.hash === ${JSON.stringify(historyFixture.first.hash)}`,
    "Forward navigation to a book",
  );
  await evaluate(
    devtools,
    `(() => {
      const input = document.querySelector(".search-input");
      input.value = "nasa";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    })()`,
  );
  await waitUntil(devtools, `document.querySelectorAll(".book").length === 1`, "history filter setup");
  await evaluate(devtools, "history.forward()");
  await waitUntil(
    devtools,
    `window.activeId === ${JSON.stringify(historyFixture.second.id)} &&
      location.hash === ${JSON.stringify(historyFixture.second.hash)} &&
      document.querySelector(".search-input").value === "" &&
      document.querySelectorAll(".book").length === 3`,
    "Forward navigation to a filtered-out book",
  );

  await evaluate(devtools, `document.querySelector('#view-control [data-view="stack"]').click()`);
  await waitUntil(devtools, `document.body.classList.contains("view-stack")`, "Stack view for ISBN popup");
  const stackIsbn = await evaluate(
    devtools,
    `(() => {
      const book = document.querySelector('.book[data-id="9780000000000"]');
      window.setActive(book.dataset.id, false, "none");
      book.scrollIntoView({ behavior: "auto", block: "center" });
      const trigger = book.querySelector(".stack-isbn-btn");
      trigger.click();
      const popup = document.querySelector("#isbn-popup");
      const rect = popup.getBoundingClientRect();
      return {
        active: window.activeId,
        expanded: trigger.getAttribute("aria-expanded"),
        visible: popup.classList.contains("is-visible"),
        insideDetails: Boolean(popup.closest("#details")),
        positioned: rect.top >= 0 && rect.left >= 0 &&
          rect.bottom <= window.innerHeight && rect.right <= window.innerWidth,
        links: [...popup.querySelectorAll(".isbn-link:not([hidden])")].every((link) => Boolean(link.href)),
        background: getComputedStyle(trigger).backgroundColor,
      };
    })()`,
  );
  if (!stackIsbn.visible || stackIsbn.expanded !== "true" || stackIsbn.insideDetails ||
      !stackIsbn.positioned || !stackIsbn.links ||
      !["rgba(0, 0, 0, 0)", "transparent"].includes(stackIsbn.background)) {
    fail(`Stack ISBN popup was not usable: ${JSON.stringify(stackIsbn)}`);
  }
  await evaluate(
    devtools,
    `document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }))`,
  );
  await waitUntil(
    devtools,
    `!document.querySelector("#isbn-popup").classList.contains("is-visible") &&
      window.activeId === ${JSON.stringify(stackIsbn.active)}`,
    "Escape closing only the ISBN popup",
  );
  await evaluate(
    devtools,
    `document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }))`,
  );
  await waitUntil(devtools, `window.activeId === null`, "second Escape closing book details");

  const overlayEscape = await evaluate(
    devtools,
    `(() => {
      const book = document.querySelector(".book");
      window.setActive(book.dataset.id, false, "none");
      document.querySelector(".stats-toggle").click();
      document.dispatchEvent(new KeyboardEvent("keydown", {
        key: "Escape", bubbles: true, cancelable: true
      }));
      return {
        statsHidden: document.querySelector("#stats-overlay").hidden,
        active: window.activeId,
        expected: book.dataset.id,
      };
    })()`,
  );
  if (!overlayEscape.statsHidden || overlayEscape.active !== overlayEscape.expected) {
    fail(`Escape closed more than the statistics overlay: ${JSON.stringify(overlayEscape)}`);
  }

  const controlEscape = await evaluate(
    devtools,
    `(() => {
      const activeBefore = window.activeId;
      const trigger = document.querySelector("#view-control .select-trigger");
      trigger.click();
      document.dispatchEvent(new KeyboardEvent("keydown", {
        key: "Escape", bubbles: true, cancelable: true
      }));
      const dropdownClosed = document.querySelector("#view-list").hidden;
      document.querySelector(".search-toggle").click();
      document.dispatchEvent(new KeyboardEvent("keydown", {
        key: "Escape", bubbles: true, cancelable: true
      }));
      return {
        dropdownClosed,
        searchClosed: !document.querySelector("#search-control").classList.contains("is-active"),
        detailPreserved: window.activeId === activeBefore,
      };
    })()`,
  );
  if (!controlEscape.dropdownClosed || !controlEscape.searchClosed || !controlEscape.detailPreserved) {
    fail(`Escape did not close one browser control at a time: ${JSON.stringify(controlEscape)}`);
  }
  await evaluate(devtools, `window.setActive(null, false, "none")`);

  await evaluate(devtools, `document.querySelector('#view-control [data-view="shelf"]').click()`);
  await waitUntil(devtools, `document.body.classList.contains("view-shelf")`, "Shelf view for keyboard checks");
  await waitUntil(devtools, `window.shelfState && !window.shelfState.isAnimating`, "idle Shelf scheduler");
  const schedulerStarted = await evaluate(
    devtools,
    `(() => {
      window.dispatchEvent(new KeyboardEvent("keydown", {
        key: "ArrowRight", bubbles: true, cancelable: true
      }));
      return window.shelfState.isAnimating;
    })()`,
  );
  if (!schedulerStarted) fail("Shelf keyboard input did not wake its animation scheduler");
  await new Promise((resolveDelay) => setTimeout(resolveDelay, 50));
  await evaluate(
    devtools,
    `window.dispatchEvent(new KeyboardEvent("keyup", { key: "ArrowRight", bubbles: true }))`,
  );
  await waitUntil(
    devtools,
    `!window.shelfState.isAnimating && window.shelfState.animationFrame === 0`,
    "Shelf scheduler returning to idle",
  );

  const searchArrow = await evaluate(
    devtools,
    `(() => {
      const toggle = document.querySelector(".search-toggle");
      const input = document.querySelector(".search-input");
      toggle.click();
      input.focus();
      const before = document.querySelector("#bookshelf").scrollLeft;
      input.dispatchEvent(new KeyboardEvent("keydown", {
        key: "ArrowRight", bubbles: true, cancelable: true
      }));
      input.dispatchEvent(new KeyboardEvent("keyup", { key: "ArrowRight", bubbles: true }));
      return {
        before,
        after: document.querySelector("#bookshelf").scrollLeft,
        held: window.shelfState.keys.right,
        animating: window.shelfState.isAnimating,
      };
    })()`,
  );
  if (searchArrow.held || searchArrow.before !== searchArrow.after || searchArrow.animating) {
    fail(`search-field arrows moved the Shelf: ${JSON.stringify(searchArrow)}`);
  }

  await evaluate(
    devtools,
    `(() => {
      window.setActive(null);
      location.hash = "978-0-00-000000-0";
    })()`,
  );
  await waitUntil(
    devtools,
    `document.querySelector('.book[data-id="9780000000000"]')?.classList.contains("is-active")`,
    "ISBN deep link",
  );
  await new Promise((resolveDelay) => setTimeout(resolveDelay, 100));

  await devtools.send("Emulation.setEmulatedMedia", {
    media: "",
    features: [{ name: "prefers-reduced-motion", value: "reduce" }],
  });
  const reducedMotionLoaded = devtools.waitFor("Page.loadEventFired");
  await devtools.send("Page.reload", { ignoreCache: true });
  await reducedMotionLoaded;
  await waitUntil(devtools, `document.querySelectorAll(".book").length === 3`, "reduced-motion rendering");
  await evaluate(devtools, `document.querySelector('#view-control [data-view="coverflow"]').click()`);
  await waitUntil(devtools, `document.body.classList.contains("view-coverflow")`, "reduced-motion Coverflow");
  const reducedMotion = await evaluate(
    devtools,
    `(() => {
      const before = window.cfState.index;
      document.querySelector("#bookshelf").dispatchEvent(
        new WheelEvent("wheel", { deltaY: 240, bubbles: true, cancelable: true })
      );
      return {
        matches: matchMedia("(prefers-reduced-motion: reduce)").matches,
        before,
        after: window.cfState.index,
        animating: window.cfState.isAnimating,
        viewAnimation: document.querySelector("#bookshelf").classList.contains("view-animate"),
        controlsTransition: parseFloat(getComputedStyle(document.querySelector(".controls")).transitionDuration),
        scrollBehavior: getComputedStyle(document.documentElement).scrollBehavior,
      };
    })()`,
  );
  if (!reducedMotion.matches) fail("reduced-motion media emulation did not apply");
  if (reducedMotion.after !== reducedMotion.before + 1 || reducedMotion.animating) {
    fail(`reduced-motion Coverflow was not immediate: ${JSON.stringify(reducedMotion)}`);
  }
  if (reducedMotion.viewAnimation || reducedMotion.controlsTransition > 0.001) {
    fail(`reduced-motion transitions remained active: ${JSON.stringify(reducedMotion)}`);
  }
  if (reducedMotion.scrollBehavior !== "auto") {
    fail(`reduced-motion scroll behavior was ${reducedMotion.scrollBehavior}`);
  }

  devtools.close();
  if (errors.length) fail(`browser reported errors:\n${errors.join("\n")}`);
};

const runEmptyLibraryChecks = async (url, browserWebSocket) => {
  const devtools = new DevTools(await pageWebSocket(browserWebSocket));
  await devtools.open();
  const errors = [];
  devtools.on("Runtime.exceptionThrown", ({ exceptionDetails }) => {
    errors.push(exceptionDetails.exception?.description || exceptionDetails.text);
  });
  devtools.on("Runtime.consoleAPICalled", ({ type, args }) => {
    if (type === "error") errors.push(args.map((argument) => argument.value || argument.description).join(" "));
  });
  await devtools.send("Runtime.enable");
  await devtools.send("Page.enable");
  const loaded = devtools.waitFor("Page.loadEventFired");
  await devtools.send("Page.navigate", { url });
  await loaded;
  await waitUntil(
    devtools,
    `document.querySelector("#no-results")?.hidden === false`,
    "empty-library message",
  );
  for (const view of ["stack", "coverflow", "shelf"]) {
    await evaluate(devtools, `document.querySelector('#view-control [data-view="${view}"]').click()`);
    await waitUntil(devtools, `document.body.classList.contains("view-${view}")`, `empty ${view} view`);
  }
  await new Promise((resolveDelay) => setTimeout(resolveDelay, 100));
  devtools.close();
  if (errors.length) fail(`empty-library browser reported errors:\n${errors.join("\n")}`);
};

try {
  const browserExecutable = findBrowser();
  if (!browserExecutable) {
    if (requireBrowser) fail("Chromium or Chrome is required for the browser smoke test");
    console.log("Browser smoke test skipped: Chromium or Chrome was not found.");
    process.exitCode = 0;
  } else {
    if (typeof WebSocket === "undefined") {
      fail("Node.js 22 or newer is required for the browser smoke test");
    }
    const publicRoots = buildFixture();
    server = await serve(publicRoots.populated);
    const address = server.address();
    browser = await launchBrowser(browserExecutable);
    await runChecks(`http://127.0.0.1:${address.port}/`, browser.browserWebSocket);
    await new Promise((resolveClose) => server.close(resolveClose));
    server = await serve(publicRoots.empty);
    const emptyAddress = server.address();
    await runEmptyLibraryChecks(`http://127.0.0.1:${emptyAddress.port}/`, browser.browserWebSocket);
    console.log("Browser smoke test passed.");
  }
} finally {
  await stopBrowser(browser);
  if (server) await new Promise((resolveClose) => server.close(resolveClose));
  rmSync(temporaryRoot, { recursive: true, force: true, maxRetries: 20, retryDelay: 100 });
}
