(function () {
  const allBooks = Array.isArray(window.booksData) ? window.booksData : [];
  const permalinkStyle = window.bookshelfConfig?.permalinkStyle || "formatted-isbn";
  const configuredSort = window.bookshelfConfig?.defaultSort;
  const defaultSort = ["title", "author", "year"].includes(configuredSort)
    ? configuredSort
    : "title";
  const configuredSortOrder = window.bookshelfConfig?.defaultSortOrder;
  const defaultSortOrder = ["ascending", "descending"].includes(configuredSortOrder)
    ? configuredSortOrder
    : "ascending";
  const shelfSpeedFactors = Object.freeze({ slow: 0.65, normal: 1, fast: 1.6 });
  const shelfKeyboardSpeedFactor =
    shelfSpeedFactors[window.bookshelfConfig?.shelfScrollSpeed] || shelfSpeedFactors.normal;
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const preferredScrollBehavior = reduceMotion ? 'auto' : 'smooth';
  let viewableBooks = [...allBooks]; // Base list for current view (filtered or not)
  window.viewableBookCount = viewableBooks.length;
  let currentFilter = ""; // Search query
  let currentSort = defaultSort;
  const bookshelf = document.getElementById("bookshelf");

  // Custom Dropdown Elements
  const sortControl = document.getElementById("sort-control");
  const sortTrigger = sortControl ? sortControl.querySelector(".select-trigger") : null;
  const sortList = sortControl ? sortControl.querySelector(".select-options") : null;
  const sortOptions = sortControl ? sortControl.querySelectorAll(".option") : [];
  const triggerText = sortTrigger ? sortTrigger.querySelector(".trigger-text") : null;
  const detailCard = document.querySelector(".details-card");
  const detailCover = detailCard ? detailCard.querySelector(".details-cover") : null;
  const detailTitle = detailCard ? detailCard.querySelector(".details-title") : null;
  const detailAuthor = detailCard ? detailCard.querySelector(".details-author") : null;
  const detailFields = document.querySelectorAll(".details [data-field]");
  const isbnTrigger = detailCard ? detailCard.querySelector(".isbn-trigger[data-field='isbn']") : null;
  const isbnPopup = document.getElementById("isbn-popup");
  const isbnLinks = isbnPopup ? Array.from(isbnPopup.querySelectorAll(".isbn-link")) : [];
  let activeIsbnTrigger = null;

  const appendTextElement = (parent, tagName, className, value) => {
    const element = document.createElement(tagName);
    if (className) element.className = className;
    element.textContent = String(value);
    parent.appendChild(element);
    return element;
  };

  const safeAssetPath = (value) =>
    String(value || "")
      .split("/")
      .map((part) => encodeURIComponent(part))
      .join("/");

  const siteTitle = String(window.bookshelfConfig?.siteTitle || "Bookshelf").trim() || "Bookshelf";
  const siteSubtitle = String(
    window.bookshelfConfig?.siteSubtitle ?? "Click on a book spine to see its details"
  ).trim();
  const appendInlineMarkdown = (parent, markdown) => {
    const pattern = /(\[[^\]]+\]\([^)]+\)|\*\*[^*]+\*\*|`[^`]+`|\*[^*]+\*|\n)/g;
    let offset = 0;
    for (const match of markdown.matchAll(pattern)) {
      if (match.index > offset) {
        parent.appendChild(document.createTextNode(markdown.slice(offset, match.index)));
      }
      const token = match[0];
      if (token === "\n") {
        parent.appendChild(document.createElement("br"));
      } else if (token.startsWith("[")) {
        const parts = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
        const href = parts ? parts[2].trim() : "";
        const safeHref = /^(https?:|mailto:|\/|#|\.\.?\/)/i.test(href);
        if (parts && safeHref) {
          const link = document.createElement("a");
          link.href = href;
          if (/^https?:/i.test(href)) {
            link.target = "_blank";
            link.rel = "noopener noreferrer";
          }
          appendInlineMarkdown(link, parts[1]);
          parent.appendChild(link);
        } else {
          parent.appendChild(document.createTextNode(token));
        }
      } else if (token.startsWith("**")) {
        const strong = document.createElement("strong");
        strong.textContent = token.slice(2, -2);
        parent.appendChild(strong);
      } else if (token.startsWith("`")) {
        const code = document.createElement("code");
        code.textContent = token.slice(1, -1);
        parent.appendChild(code);
      } else {
        const emphasis = document.createElement("em");
        emphasis.textContent = token.slice(1, -1);
        parent.appendChild(emphasis);
      }
      offset = match.index + token.length;
    }
    if (offset < markdown.length) {
      parent.appendChild(document.createTextNode(markdown.slice(offset)));
    }
  };
  const heading = document.querySelector(".hero-heading h1");
  const instruction = document.getElementById("instruction-text");
  const footer = document.querySelector(".site-footer");
  if (heading) heading.textContent = siteTitle;
  document.title = siteTitle;
  if (instruction) {
    instruction.textContent = siteSubtitle;
    instruction.hidden = !siteSubtitle;
  }
  if (footer) {
    footer.hidden = window.bookshelfConfig?.showFooter === false;
    const footerText = String(window.bookshelfConfig?.footerText || "").trim();
    if (footerText) {
      footer.replaceChildren();
      appendInlineMarkdown(footer, footerText);
    }
  }
  if (isbnLinks) {
    const sources = window.bookshelfConfig?.isbnLinkSources || "both";
    isbnLinks.forEach((link) => {
      const source = link.dataset.source;
      link.hidden = sources !== "both" && source !== sources;
    });
  }

  // Search Elements
  const searchControl = document.getElementById("search-control");
  const searchToggle = searchControl ? searchControl.querySelector(".search-toggle") : null;
  const searchInput = searchControl ? searchControl.querySelector(".search-input") : null;

  if (!bookshelf) return;

  let bookElements = [];
  window.bookshelfRenderVersion = 0;
  const detailsSection = document.getElementById("details");
  let activeId = null;
  window.activeId = null;
  let layoutObserver = null;

  const ANIMATION_BUFFER = 280;
  const ISBN_SOURCES = {
    wikipedia: {
      buildUrl: (isbn) =>
        `https://en.wikipedia.org/wiki/Special:BookSources?isbn=${encodeURIComponent(isbn)}`,
    },
    goodreads: {
      buildUrl: (isbn) =>
        `https://www.goodreads.com/search?utf8=%E2%9C%93&query=${encodeURIComponent(isbn)}`,
    },
  };

  const colorProbe = document.createElement("span");
  colorProbe.style.cssText = "position:absolute;left:-9999px;top:-9999px;";
  document.body.appendChild(colorProbe);

  const clamp = (value, min, max) => Math.max(min, Math.min(max, value));

  const parseColor = (value) => {
    if (!value) return null;
    colorProbe.style.color = "";
    colorProbe.style.color = value;
    const computed = getComputedStyle(colorProbe).color;
    const match = computed.match(/rgba?\(([^)]+)\)/);
    if (!match) return null;
    const parts = match[1].split(",").map((part) => parseFloat(part.trim()));
    return { r: parts[0], g: parts[1], b: parts[2] };
  };

  const toLinear = (channel) => {
    const c = channel / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };

  const luminance = (rgb) => {
    if (!rgb) return 0;
    return (
      0.2126 * toLinear(rgb.r) +
      0.7152 * toLinear(rgb.g) +
      0.0722 * toLinear(rgb.b)
    );
  };

  const contrastRatio = (colorA, colorB) => {
    if (!colorA || !colorB) return 1;
    const lumA = luminance(colorA);
    const lumB = luminance(colorB);
    const lighter = Math.max(lumA, lumB);
    const darker = Math.min(lumA, lumB);
    return (lighter + 0.05) / (darker + 0.05);
  };

  const bestHeadingColor = (background) => {
    const bg = parseColor(background);
    const dark = { r: 17, g: 17, b: 17 };
    const light = { r: 253, g: 253, b: 253 };
    const darkRatio = contrastRatio(bg, dark);
    const lightRatio = contrastRatio(bg, light);
    return darkRatio >= lightRatio ? "#111111" : "#fdfdfd";
  };

  const hashCode = (str) => {
    let hash = 0;
    if (!str) {
      return hash;
    }
    for (let i = 0; i < str.length; i += 1) {
      hash = (hash << 5) - hash + str.charCodeAt(i);
      hash |= 0;
    }
    return Math.abs(hash);
  };

  const makePalette = (book) => {
    const seed = hashCode(`${book.id}-${book.title}`);
    const hue = seed % 360;
    const saturation = 30 + (seed % 25);
    const lightness = 38 + (Math.floor(seed / 3) % 32);
    const background = `hsl(${hue}, ${saturation}%, ${lightness}%)`;
    const foreground = lightness > 60 ? "#1f1f25" : "#fdfdfd";
    return { background, foreground };
  };

  const pickPalette = (book) => {
    if (book.spineColor && book.spineTextColor) {
      return {
        background: book.spineColor,
        foreground: book.spineTextColor,
      };
    }
    return makePalette(book);
  };

  const normalizeCoverPath = (cover) => {
    if (!cover || typeof cover !== "string") {
      return null;
    }
    // Fix relative path since this script is now in /js/
    return cover.replace(/\\/g, "/");
  };

  const sanitizeIsbn = (value) => {
    if (value === null || value === undefined) {
      return "";
    }
    return String(value)
      .replace(/[^0-9xX]/g, "")
      .toUpperCase();
  };

  const permalinkToken = (book) => {
    if (!book) return "";
    if (book.permalink) return String(book.permalink).trim();
    if (book.slug) return String(book.slug).trim();
    if (permalinkStyle === "title-slug" && book.titleSlug) {
      return String(book.titleSlug).trim();
    }
    if (book.isbn) {
      if (permalinkStyle === "compact-isbn") return sanitizeIsbn(book.isbn);
      return String(book.isbn).trim();
    }
    return String(book.titleSlug || book.id || "").trim();
  };

  const closeIsbnPopup = (restoreFocus = false) => {
    if (!isbnPopup) return;
    const trigger = activeIsbnTrigger;
    isbnPopup.classList.remove("is-visible");
    isbnPopup.setAttribute("aria-hidden", "true");
    if (trigger) trigger.setAttribute("aria-expanded", "false");
    activeIsbnTrigger = null;
    if (restoreFocus && trigger) trigger.focus();
  };
  window.closeIsbnPopup = closeIsbnPopup;

  const positionIsbnPopup = (trigger) => {
    if (!isbnPopup || !trigger) return;
    const rect = trigger.getBoundingClientRect();
    const gap = 7;
    const popupHeight = isbnPopup.offsetHeight;
    const popupWidth = isbnPopup.offsetWidth;
    const opensUpward = window.innerHeight - rect.bottom < popupHeight + gap && rect.top > popupHeight + gap;
    const top = opensUpward ? rect.top - popupHeight - gap : rect.bottom + gap;
    const left = Math.max(8, Math.min(rect.left, window.innerWidth - popupWidth - 8));
    isbnPopup.style.top = `${Math.max(8, top)}px`;
    isbnPopup.style.left = `${left}px`;
    if (opensUpward) {
      isbnPopup.classList.add("is-upward");
    } else {
      isbnPopup.classList.remove("is-upward");
    }
  };

  const updateIsbnLinks = (value) => {
    const cleanIsbn = sanitizeIsbn(value);
    isbnLinks.forEach((link) => {
      const source = ISBN_SOURCES[link.dataset.source];
      if (!source || !cleanIsbn) {
        link.removeAttribute("href");
        return;
      }
      link.href = source.buildUrl(cleanIsbn);
    });
    return cleanIsbn;
  };

  const openIsbnPopup = (trigger = isbnTrigger) => {
    if (!isbnPopup || !trigger || trigger.disabled) return;
    const cleanIsbn = updateIsbnLinks(trigger.dataset.isbn || trigger.textContent);
    if (!cleanIsbn) return;
    if (activeIsbnTrigger && activeIsbnTrigger !== trigger) {
      activeIsbnTrigger.setAttribute("aria-expanded", "false");
    }
    activeIsbnTrigger = trigger;
    isbnPopup.classList.add("is-visible");
    isbnPopup.setAttribute("aria-hidden", "false");
    trigger.setAttribute("aria-expanded", "true");
    positionIsbnPopup(trigger);
  };

  const toggleIsbnPopup = (trigger = isbnTrigger) => {
    if (!isbnPopup) return;
    if (isbnPopup.classList.contains("is-visible") && activeIsbnTrigger === trigger) {
      closeIsbnPopup();
    } else {
      openIsbnPopup(trigger);
    }
  };

  const updateIsbnField = (value) => {
    if (!isbnTrigger) return;
    const cleanIsbn = sanitizeIsbn(value);
    const hasIsbn = Boolean(cleanIsbn);
    let displayValue = "-";
    if (value !== null && value !== undefined && value !== "") {
      const candidate = String(value).trim();
      displayValue = candidate.length ? candidate : cleanIsbn || "-";
    } else if (hasIsbn) {
      displayValue = cleanIsbn;
    }
    isbnTrigger.textContent = displayValue;
    isbnTrigger.disabled = !hasIsbn;
    isbnTrigger.dataset.isbn = hasIsbn ? cleanIsbn : "";
    isbnTrigger.setAttribute(
      "aria-label",
      hasIsbn ? `Open ISBN links for ${cleanIsbn}` : "No ISBN available",
    );
    if (activeIsbnTrigger === isbnTrigger) closeIsbnPopup();
    updateIsbnLinks(cleanIsbn);
  };

  const formatTitle = (title) =>
    typeof title === "string" ? title.trim() : "";

  const cardHeight = (book) => {
    const title = typeof book.title === "string" ? book.title.trim() : "";
    const author = typeof book.author === "string" ? book.author.trim() : "";

    // Combine lengths: Title (weight 1.0) + Author (weight 1.15)
    // Authors take up significant space when wrapped, often more than title due to listing multiple names.
    const textLoad = title.length + (author.length * 1.15);

    if (textLoad < 1) {
      return 324;
    }

    // Scale more aggressively for longer content
    const length = clamp(textLoad, 8, 300);
    const ratio = (length - 8) / (300 - 8);

    // Base 288, max spread to 495 * 1.5 = ~742px
    const base = 288 + ratio * 207;
    return Math.round(base * 1.5);
  };

  const paddingFromIndex = (book, index) => ({
    left: 14 + (hashCode(book.id) % 8),
    right: 16 + ((index + 2) % 5) * 4,
  });

  const coverTilt = (book, index) => {
    const wobble = (hashCode(book.author || book.title) % 5) - 2;
    return clamp(wobble * 0.3 + (index % 3) * 0.15, -3, 3);
  };

  const decorateBook = (book, index) => {
    const palette = pickPalette(book);
    const height = cardHeight(book);
    const coverPath = normalizeCoverPath(book.thumbnail || book.cover);
    const coverHeight = clamp(Math.round(height * 0.35), 99, 135);
    const coverWidth = clamp(Math.round(coverHeight * 0.65), 63, 81);
    const cardWidth = clamp(coverWidth + 40, 94, 126);
    const padding = paddingFromIndex(book, index);

    return {
      palette,
      height,
      cardWidth,
      paddingLeft: padding.left,
      paddingRight: padding.right,
      coverHeight,
      coverWidth,
      coverPath,
      tilt: coverTilt(book, index),
    };
  };



  const measureBookPositions = () => {
    if (!window.bookElements?.length && !bookElements?.length) return;
    const elements = window.bookElements || bookElements;
    // Cache container width to avoid layout thrashing in animation loop
    window.shelfWidth = bookshelf.clientWidth;

    elements.forEach((entry) => {
      if (!entry || !entry.el) return;
      entry.left = entry.el.offsetLeft;
      entry.width = entry.el.offsetWidth;
    });
  };
  window.measureBookPositions = measureBookPositions;

  // --- Shelf + Stack Rendering ---
  // --- Shelf + Stack Rendering ---

  // Lazy Loading State
  let nextBookIndex = 0;
  const BATCH_SIZE = 50;
  const sentinel = document.createElement("div");
  sentinel.className = "sentinel";
  // Ensure sentinel has dimension so it is tracked correctly in flex layouts
  sentinel.style.cssText = "width:10px;height:10px;flex-shrink:0;opacity:0;pointer-events:none;";
  let batchObserver = null;

  const createBookElement = (book, index) => {
    const metrics = decorateBook(book, index);
    const displayTitle = formatTitle(book.title);
    const el = document.createElement("button");
    el.type = "button";
    el.className = "book";
    el.dataset.id = book.id;
    el.setAttribute("aria-pressed", "false");
    el.setAttribute(
      "aria-label",
      `${displayTitle}${book.author ? ` by ${book.author}` : ""}`,
    );
    el.style.setProperty("--card-height", `${metrics.height}px`);
    el.style.setProperty("--card-width", `${metrics.cardWidth}px`);
    el.style.setProperty("--pad-left", `${metrics.paddingLeft}px`);
    el.style.setProperty("--pad-right", `${metrics.paddingRight}px`);
    el.style.setProperty("--cover-height", `${metrics.coverHeight}px`);
    el.style.setProperty("--cover-width", `${metrics.coverWidth}px`);
    el.style.setProperty("--cover-tilt", `${metrics.tilt}deg`);
    el.style.setProperty("--bg", metrics.palette.background);
    el.style.setProperty("--fg", metrics.palette.foreground);

    // Prepare Cover URL
    // CSS Variable needs to be relative to the CSS file (../data/...)
    // Inline styles need to be relative to the Document (data/...)
    const encodedCoverPath = safeAssetPath(metrics.coverPath);
    const cssCoverUrl = encodedCoverPath ? `url("../${encodedCoverPath}")` : "";
    const docCoverUrl = encodedCoverPath ? `url("${encodedCoverPath}")` : "";

    if (metrics.coverPath) {
      el.classList.add("has-cover");
      el.style.setProperty("--cover-image", cssCoverUrl);
    } else {
      el.classList.remove("has-cover");
      el.style.removeProperty("--cover-image");
    }

    // --- Shelf View Elements (Direct children, no wrapper) ---
    const shelfCover = appendTextElement(el, "span", "shelf-cover", "");
    shelfCover.setAttribute("aria-hidden", "true");
    const shelfText = appendTextElement(el, "div", "shelf-text", "");
    if (book.author) appendTextElement(shelfText, "small", "author", book.author);
    appendTextElement(shelfText, "span", "title", displayTitle);

    // --- Stack View Content (Details Card Look) ---
    const stackContent = appendTextElement(el, "div", "stack-content", "");
    const stackCover = appendTextElement(stackContent, "div", "stack-cover", "");
    if (docCoverUrl) stackCover.style.backgroundImage = docCoverUrl;
    const stackBody = appendTextElement(stackContent, "div", "stack-body", "");
    const stackHeading = appendTextElement(stackBody, "div", "stack-heading", "");
    appendTextElement(stackHeading, "p", "stack-title", displayTitle);
    if (book.author) appendTextElement(stackHeading, "p", "stack-author", book.author);
    const stackGrid = appendTextElement(stackBody, "dl", "stack-grid", "");
    let stackItemCount = 0;
    const appendStackItem = (label, value) => {
      if (!value) return;
      const row = appendTextElement(stackGrid, "div", "", "");
      appendTextElement(row, "dt", "", label);
      appendTextElement(row, "dd", "", value);
      stackItemCount++;
    };
    appendStackItem("Publisher", book.publisher);
    appendStackItem("Published", book.published);
    appendStackItem("Binding", book.binding);
    appendStackItem("Translator", book.translator);
    if (book.isbn) {
      const row = appendTextElement(stackGrid, "div", "", "");
      appendTextElement(row, "dt", "", "ISBN");
      const field = appendTextElement(row, "dd", "isbn-field", "");
      const trigger = appendTextElement(field, "span", "isbn-trigger stack-isbn-btn", sanitizeIsbn(book.isbn));
      trigger.setAttribute("role", "button");
      trigger.tabIndex = 0;
      trigger.setAttribute("aria-label", `Open ISBN links for ${book.isbn}`);
      trigger.setAttribute("aria-haspopup", "true");
      trigger.setAttribute("aria-controls", "isbn-popup");
      trigger.setAttribute("aria-expanded", "false");
      trigger.dataset.isbn = book.isbn;
      stackItemCount++;
    }
    stackGrid.dataset.count = String(stackItemCount);

    // Attach handler for Stack View ISBN trigger
    const stackTrigger = el.querySelector(".stack-content .isbn-trigger");
    if (stackTrigger) {
      // Shared logic
      const triggerAction = (e) => {
        e.stopPropagation();
        e.preventDefault();
        toggleIsbnPopup(stackTrigger);
      };

      stackTrigger.addEventListener("click", triggerAction);
      stackTrigger.addEventListener("keydown", (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          triggerAction(e);
        }
      });
    }

    el.addEventListener("click", () => {
      if (activeId === book.id) {
        setActive(null);
      } else {
        setActive(book.id, true);
      }
    });
    el.bookData = book;
    return { book, el, left: 0, width: 0 };
  };

  window.loadMoreBooks = (skipMeasure = false) => {
    if (nextBookIndex >= viewableBooks.length) {
      return;
    }

    // Load a batch
    const batch = viewableBooks.slice(nextBookIndex, nextBookIndex + BATCH_SIZE);
    const startNext = nextBookIndex;

    const newItems = batch.map((book, i) => {
      const item = createBookElement(book, startNext + i);
      // Insert before sentinel so sentinel stays at the end
      bookshelf.insertBefore(item.el, sentinel);
      return item;
    });

    bookElements = bookElements.concat(newItems);
    nextBookIndex += batch.length;
    window.bookshelfRenderVersion += 1;

    if (!skipMeasure) {
      measureBookPositions();
    }
  };

  const loadMoreBooks = (observer) => {
    if (nextBookIndex >= viewableBooks.length) {
      if (observer) observer.disconnect();
      if (sentinel.parentNode) sentinel.parentNode.removeChild(sentinel);
      return;
    }
    window.loadMoreBooks(false);
  };

  const maybeLoadMoreForShelf = () => {
    if (document.body.classList.contains("view-coverflow")) return;
    const isHorizontalShelf = bookshelf.classList.contains("force-shelf")
      || document.body.classList.contains("view-shelf");
    if (!isHorizontalShelf || nextBookIndex >= viewableBooks.length) return;

    const currentScroll = bookshelf.scrollLeft;
    const remaining = bookshelf.scrollWidth - (currentScroll + bookshelf.clientWidth);
    if (remaining <= 400) {
      loadMoreBooks(batchObserver);
    }
  };

  bookshelf.addEventListener("scroll", maybeLoadMoreForShelf, { passive: true });

  const initObserver = () => {
    if (batchObserver) batchObserver.disconnect();
    const options = {
      root: null, // viewport
      rootMargin: "200px", // Reduced margin to ensure lazy loading is felt
      threshold: 0.1 // Require at least a pixel to be visible
    };

    batchObserver = new IntersectionObserver((entries) => {
      // If the sentinel is visible, load more
      if (entries.some(entry => entry.isIntersecting)) {
        loadMoreBooks(batchObserver);
      }
    }, options);

    batchObserver.observe(sentinel);
    maybeLoadMoreForShelf();
  };

  const applySort = () => {
    const direction = defaultSortOrder === "descending" ? -1 : 1;
    // Sort viewableBooks in place based on currentSort
    if (currentSort === "author") {
      viewableBooks.sort((a, b) => {
        const authorA = (a.author || "").toLowerCase();
        const authorB = (b.author || "").toLowerCase();
        return direction * authorA.localeCompare(authorB);
      });
    } else if (currentSort === "title") {
      viewableBooks.sort((a, b) => {
        const titleA = (a.title || "").toLowerCase();
        const titleB = (b.title || "").toLowerCase();
        return direction * titleA.localeCompare(titleB);
      });
    } else if (currentSort === "year") {
      viewableBooks.sort((a, b) => {
        return direction * ((a.published || 0) - (b.published || 0));
      });
    }
  };

  const normalizeText = (text) => {
    if (!text) return "";
    return text
      .toLowerCase()
      .replace(/ğ/g, "g")
      .replace(/ü/g, "u")
      .replace(/ş/g, "s")
      .replace(/ı/g, "i")
      .replace(/İ/g, "i")
      .replace(/ö/g, "o")
      .replace(/ç/g, "c")
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "");
  };

  const getLevenshteinDistance = (a, b) => {
    if (a.length === 0) return b.length;
    if (b.length === 0) return a.length;

    const matrix = [];

    // Increment along the first column of each row
    for (let i = 0; i <= b.length; i++) {
      matrix[i] = [i];
    }

    // Increment each column in the first row
    for (let j = 0; j <= a.length; j++) {
      matrix[0][j] = j;
    }

    // Fill in the rest of the matrix
    for (let i = 1; i <= b.length; i++) {
      for (let j = 1; j <= a.length; j++) {
        if (b.charAt(i - 1) === a.charAt(j - 1)) {
          matrix[i][j] = matrix[i - 1][j - 1];
        } else {
          matrix[i][j] = Math.min(
            matrix[i - 1][j - 1] + 1, // substitution
            Math.min(
              matrix[i][j - 1] + 1, // insertion
              matrix[i - 1][j] + 1 // deletion
            )
          );
        }
      }
    }

    return matrix[b.length][a.length];
  };

  // No Results Element
  const noResultsMsg = document.getElementById("no-results");

  const filterBooks = (query) => {
    const rawQuery = query.trim();
    if (rawQuery.length < 3) {
      viewableBooks = [...allBooks];
    } else {
      const normalizedQuery = normalizeText(rawQuery);
      // Split query into tokens (words) to match individually
      const queryTokens = normalizedQuery.split(/\s+/).filter(t => t.length > 0);

      viewableBooks = allBooks.filter((book) => {
        const title = normalizeText(book.title || "");
        const author = normalizeText(book.author || "");
        // Combine title and author into a search corpus
        const corpus = title + " " + author;
        const corpusTokens = corpus.split(/\s+/);

        // Check if EVERY token in the query matches at least one token in the corpus
        return queryTokens.every(qToken => {
          // 1. Direct substring match (Fast & covers partial words like 'Rowl' -> 'Rowling')
          if (corpus.includes(qToken)) return true;

          // 2. Fuzzy Match against individual corpus tokens
          // Dynamic tolerance: 1 error for length < 5, 2 for length >= 5
          const allowedErrors = qToken.length < 5 ? 1 : 2;

          return corpusTokens.some(cToken => {
            // Optimization: Don't check Levenshtein if length diff is too big
            if (Math.abs(cToken.length - qToken.length) > allowedErrors) return false;
            return getLevenshteinDistance(qToken, cToken) <= allowedErrors;
          });
        });
      });
    }
    window.viewableBookCount = viewableBooks.length;
    // Re-apply sort to the new filtered list
    applySort();
    renderBooks();
  };

  const sortBooks = (criterion) => {
    currentSort = criterion;
    applySort();
    renderBooks();
  };

  // Search Logic
  const initSearch = () => {
    if (!searchToggle || !searchInput || !searchControl) return;

    searchToggle.addEventListener("click", () => {
      const isActive = searchControl.classList.toggle("is-active");
      searchToggle.setAttribute("aria-expanded", String(isActive));
      if (isActive) {
        searchInput.focus();
      }
    });

    searchInput.addEventListener("input", (e) => {
      const value = e.target.value;
      filterBooks(value);
    });

    // Auto-select text on focus
    searchInput.addEventListener("focus", () => {
      if (searchInput.value) {
        searchInput.select();
      }
    });

    // Close search if clicking outside
    document.addEventListener("click", (e) => {
      if (!searchControl.contains(e.target) && searchControl.classList.contains("is-active")) {
        searchControl.classList.remove("is-active");
        searchToggle.setAttribute("aria-expanded", "false");
      }
    });
  };

  const initRandom = () => {
    const randomBtn = document.querySelector(".random-toggle");
    if (!randomBtn) return;
    if (window.bookshelfConfig?.showRandom === false) {
      randomBtn.hidden = true;
      return;
    }

    randomBtn.addEventListener("click", () => {
      if (viewableBooks.length === 0) return;

      // Pick a random index
      const randomIndex = Math.floor(Math.random() * viewableBooks.length);
      const randomBook = viewableBooks[randomIndex];
      const targetId = randomBook.id;

      // Different behavior based on View
      if (currentView === 'coverflow') {
        // In Coverflow: Scroll the slider/state to that index
        // We need to find the index of this book in the DOM/cache to set cfState.targetIndex
        // Since coverflow uses allBooks (essentially) unless filtered.
        // Wait, coverflow respects filterBooks? Yes, renderBooks resets DOM.

        // So randomIndex IS the index in the current coverflow.
        // We can just set the state target.
        if (window.seekCoverflow) window.seekCoverflow(randomIndex);

      } else {
        // Shelf / Stack: Just use standard setActive and Scroll
        // Ensure book is loaded
        while (nextBookIndex <= randomIndex) {
          loadMoreBooks();
        }

        // Use timeout to allow DOM to populate if we just loaded
        setTimeout(() => {
          setActive(targetId, true);
        }, 50);
      }
    });
  };

  initRandom();

  // Custom Dropdown Logic
  const initSortDropdown = () => {
    if (!sortTrigger || !sortList) return;

    const closeDropdown = () => {
      sortTrigger.setAttribute("aria-expanded", "false");
      sortList.hidden = true;
    };

    const openDropdown = () => {
      sortTrigger.setAttribute("aria-expanded", "true");
      sortList.hidden = false;
    };

    const toggleDropdown = () => {
      const isExpanded = sortTrigger.getAttribute("aria-expanded") === "true";

      // Close View Menu
      const viewCtrl = document.getElementById('view-control');
      if (viewCtrl) {
        const viewTrig = viewCtrl.querySelector('.select-trigger');
        const viewList = viewCtrl.querySelector('.select-options');
        if (viewTrig) viewTrig.setAttribute('aria-expanded', 'false');
        if (viewList) viewList.hidden = true;
      }

      if (isExpanded) closeDropdown();
      else openDropdown();
    };

    sortTrigger.addEventListener("click", (e) => {
      e.stopPropagation();
      toggleDropdown();
    });

    sortOptions.forEach((option) => {
      option.addEventListener("click", (e) => {
        e.stopPropagation();
        const value = option.dataset.value;
        const label = option.textContent;

        // Update functionality
        sortBooks(value);

        // Update UI - Sticky Text Requirement
        // if (triggerText) triggerText.textContent = label;

        sortOptions.forEach(opt => opt.setAttribute("aria-selected", "false"));

        option.setAttribute("aria-selected", "true");

        closeDropdown();
      });

      option.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          option.click();
        }
      });
    });

    document.addEventListener("click", (e) => {
      if (sortControl && !sortControl.contains(e.target)) {
        closeDropdown();
      }
    });

  };

  initSortDropdown();

  const renderBooks = () => {
    if (batchObserver) {
      batchObserver.disconnect();
      batchObserver = null;
    }
    bookshelf.replaceChildren();
    bookElements = []; // Reset global
    nextBookIndex = 0; // Reset
    window.bookshelfRenderVersion += 1;

    // Handle No Results
    if (viewableBooks.length === 0) {
      if (noResultsMsg) noResultsMsg.hidden = false;
      return;
    }
    if (noResultsMsg) noResultsMsg.hidden = true;

    // Append sentinel first (it will be pushed down/right as items are added)
    bookshelf.appendChild(sentinel);

    // Load initial batch immediately to fill the screen
    loadMoreBooks();

    // Start observing for subsequent batches
    setTimeout(initObserver, 100);
    window.dispatchEvent(new CustomEvent('bookshelf:rendered'));
  };

  const setActive = (id, shouldCenter = false, historyMode = "push") => {
    if (activeId === id && !shouldCenter) return;
    activeId = id;
    window.activeId = id; // Expose for other scopes

    if (historyMode !== "none") {
      const activeBook = id ? allBooks.find((book) => book.id === id) : null;
      const token = id ? permalinkToken(activeBook) || id : "";
      const destination = token
        ? `#${encodeURIComponent(token)}`
        : window.location.pathname + window.location.search;
      if (history.pushState) {
        const method = historyMode === "replace" ? "replaceState" : "pushState";
        history[method](null, "", destination);
      } else if (historyMode !== "replace") {
        window.location.hash = token;
      }
    }

    // Toggle Elements
    if (!id) {
      closeIsbnPopup();
      // Hide visually immediately but keep layout to allow smooth scroll
      if (detailsSection) {
        detailsSection.style.opacity = '0';
        detailsSection.style.pointerEvents = 'none';
      }

      // Smooth Scroll back up to the very top
      window.scrollTo({ top: 0, behavior: preferredScrollBehavior });

      // Remove from layout after scroll completes
      setTimeout(() => {
        if (!window.activeId) {
          if (detailsSection) {
            detailsSection.hidden = true;
            detailsSection.style.opacity = '';
            detailsSection.style.pointerEvents = '';
          }
          if (detailCard) detailCard.hidden = true;
        }
      }, reduceMotion ? 0 : 800);

      bookElements.forEach(({ el }) => {
        el.classList.remove("is-active");
        el.setAttribute("aria-pressed", "false");
      });
      return;
    }

    bookElements.forEach(({ book, el }) => {
      const isActive = book.id === id;
      el.classList.toggle("is-active", isActive);
      el.setAttribute("aria-pressed", String(isActive));
      if (isActive) {
        updateDetails(book);

        // Auto-Scroll to Details Panel
        if (detailsSection) {
          setTimeout(() => {
            detailsSection.scrollIntoView({ behavior: preferredScrollBehavior, block: 'start' });
          }, reduceMotion ? 0 : 100);
        }

        // Center the book if requested logic (e.g. from click)
        if (shouldCenter) {
          if (document.body.classList.contains('view-stack') || bookshelf.classList.contains("force-stack")) {
            // Vertical Scroll for Stack View (Desktop or Mobile)
            el.scrollIntoView({ behavior: preferredScrollBehavior, block: 'center' });
          } else {
            // Horizontal Scroll for Shelf View
            const containerWidth = bookshelf.clientWidth;
            const bookLeft = el.offsetLeft;
            const bookWidth = el.offsetWidth;
            const centerPos = bookLeft - (containerWidth / 2) + (bookWidth / 2);

            bookshelf.scrollTo({ left: centerPos, behavior: preferredScrollBehavior });
          }
        }
      }
    });
  };
  window.setActive = setActive;

  const copyLinkBtn = document.getElementById('details-copy-btn');
  if (copyLinkBtn) {
    copyLinkBtn.addEventListener('click', () => {
      const url = window.location.href;
      navigator.clipboard.writeText(url).then(() => {
        // Subtle feedback: change icon briefly using unicode checkmark
        const icon = copyLinkBtn.querySelector('.copy-icon');
        const originalIcon = icon.textContent;
        icon.textContent = "\u2713\uFE0E"; // Unicode Checkmark (Text Version)
        setTimeout(() => {
          icon.textContent = originalIcon;
        }, 1500);
      }).catch(err => {
        console.error('Failed to copy: ', err);
      });
    });
  }

  // Close Button Logic
  const closeDetailsBtn = document.querySelector('.details-close');
  if (closeDetailsBtn) {
    closeDetailsBtn.addEventListener('click', () => {
      setActive(null);
    });
  }

  const updateDetails = (book) => {
    if (!detailCard) return;

    // Hide details if in Stack mode
    if (bookshelf.classList.contains("force-stack")) {
      if (detailsSection) detailsSection.hidden = true;
      detailCard.hidden = true;
      return;
    }

    detailCard.hidden = false;
    if (detailsSection) {
      detailsSection.hidden = false;
      detailsSection.style.opacity = '';
      detailsSection.style.pointerEvents = '';
    }
    const palette = pickPalette(book);
    detailCard.style.setProperty("--details-bg", palette.background);
    detailCard.style.setProperty("--details-fg", palette.foreground);
    // Glow removed as requested
    // detailCard.style.boxShadow = `0 0 120px -30px ${palette.background}, 0 0 40px -10px rgba(0,0,0,0.5)`;

    const headingColor = bestHeadingColor(palette.background);
    detailCard.style.setProperty("--details-heading-color", headingColor);
    if (detailTitle) {
      detailTitle.style.color = headingColor;
    }
    if (detailAuthor) {
      detailAuthor.style.color = headingColor;
    }

    if (detailCover) {
      const coverPath = normalizeCoverPath(book.cover);
      if (coverPath) {
        detailCard.classList.add("has-cover");
        detailCover.style.setProperty(
          "background-image",
          `url("${coverPath}")`,
        );
      } else {
        detailCard.classList.remove("has-cover");
        detailCover.style.removeProperty("background-image");
      }
    }

    detailFields.forEach((field) => {
      const key = field.getAttribute("data-field");

      // Identify valid parent container for Grid items (Translator, Publisher, etc)
      // They are wrapped in a <div> inside .details-grid
      const container = field.closest('.details-grid > div');

      if (key === "isbn") {
        const hasIsbn = Boolean(sanitizeIsbn(book[key]));
        if (hasIsbn) {
          updateIsbnField(book[key]);
          if (container) container.hidden = false;
          field.hidden = false;
        } else {
          if (container) container.hidden = true;
          else field.hidden = true;
        }
        return;
      }

      let value = null;
      if (key === "title") {
        value = formatTitle(book.title);
      } else if (key === "author") {
        value = book.author;
      } else if (book[key]) {
        value = book[key];
      }

      if (value) {
        field.textContent = value;
        field.hidden = false;
        if (container) container.hidden = false;
      } else {
        field.hidden = true;
        if (container) container.hidden = true;
      }
    });
  };

  // --- Observers & Interactivity ---
  const setupObservers = () => {
    if (typeof ResizeObserver !== "undefined") {
      layoutObserver = new ResizeObserver(() => {
        measureBookPositions();
      });
      layoutObserver.observe(bookshelf);
    } else {
      window.addEventListener("resize", measureBookPositions);
    }
  };

  const initIsbnPopup = () => {
    if (!isbnTrigger || !isbnPopup) return;
    isbnTrigger.addEventListener("click", () => {
      if (isbnTrigger.disabled) return;
      toggleIsbnPopup(isbnTrigger);
    });

    document.addEventListener("click", (event) => {
      if (!isbnPopup.classList.contains("is-visible") || !event.target) {
        return;
      }
      if (
        event.target === activeIsbnTrigger ||
        activeIsbnTrigger?.contains(event.target) ||
        isbnPopup.contains(event.target)
      ) {
        return;
      }
      closeIsbnPopup();
    });

    isbnLinks.forEach((link) => {
      link.addEventListener("click", () => {
        closeIsbnPopup();
      });
    });
    window.addEventListener("resize", () => {
      if (isbnPopup.classList.contains("is-visible") && activeIsbnTrigger) {
        positionIsbnPopup(activeIsbnTrigger);
      }
    });
    window.addEventListener("scroll", () => {
      if (isbnPopup.classList.contains("is-visible") && activeIsbnTrigger) {
        positionIsbnPopup(activeIsbnTrigger);
      }
    }, true);
  };

  // --- Shelf animation and responsive layout ---
  let currentTilt = 0;
  let targetTilt = 0;
  let lastScroll = -1;

  const shelfState = window.shelfState = {
    keyVelocity: 0,
    keys: { left: false, right: false },
    isAnimating: false,
    animationFrame: 0,
  };
  let shelfAnimationFrame = 0;

  const KEY_BASE_SPEED_PPS = 100 * shelfKeyboardSpeedFactor; // Base crawl speed
  const KEY_MAX_SPEED_PPS = 4000 * shelfKeyboardSpeedFactor; // Max speed
  const KEY_ACCEL_PPS = 2400 * shelfKeyboardSpeedFactor; // Acceleration per second
  const KEY_FRICTION_FACTOR = 8.0; // Damping (v -= v * f * dt)

  // Simple linear interpolation
  const lerp = (start, end, factor) => start + (end - start) * factor;

  const applyResponsiveLayout = () => {
    const isMobile = window.matchMedia("(max-width: 768px)").matches;
    const isStackView = document.body.classList.contains('view-stack');
    const isCoverflow = document.body.classList.contains('view-coverflow');

    bookshelf.classList.remove("force-stack", "force-shelf");

    if (isMobile) {
      bookshelf.classList.add("force-stack");
      bookshelf.style.scrollBehavior = preferredScrollBehavior;
    } else if (isStackView) {
      bookshelf.style.scrollBehavior = preferredScrollBehavior;
    } else if (!isCoverflow) {
      bookshelf.classList.add("force-shelf");
    }
  };

  const initResponsiveLayout = () => {
    const bodyObserver = new MutationObserver(() => {
      applyResponsiveLayout();
    });
    bodyObserver.observe(document.body, { attributes: true, attributeFilter: ['class'] });

    applyResponsiveLayout();

    let resizeTimer;
    window.addEventListener('resize', () => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        applyResponsiveLayout();
        measureBookPositions();

        // Correctly handle detail visibility on resize
        if (bookshelf.classList.contains("force-stack")) {
          if (detailsSection) detailsSection.hidden = true;
          if (detailCard) detailCard.hidden = true;
        } else if (activeId) {
          // If resizing to desktop and we have an active book, show details
          const activeBook = allBooks.find((b) => b.id === activeId);
          if (activeBook) {
            updateDetails(activeBook);
          }
        }
      }, 100);
    });
  };



  const shelfNeedsAnimation = () =>
    shelfState.keys.left ||
    shelfState.keys.right ||
    Math.abs(shelfState.keyVelocity) >= 5 ||
    Math.abs(currentTilt) >= 0.01 ||
    Math.abs(targetTilt) >= 0.01;

  const wakeShelfAnimation = () => {
    if (shelfAnimationFrame !== 0 ||
      bookshelf.classList.contains("force-stack") ||
      document.body.classList.contains("view-coverflow")) {
      return;
    }
    shelfState.isAnimating = true;
    shelfAnimationFrame = requestAnimationFrame(updateAnimation);
    shelfState.animationFrame = shelfAnimationFrame;
  };
  window.wakeShelfAnimation = wakeShelfAnimation;

  const stopShelfAnimation = () => {
    if (shelfAnimationFrame !== 0) cancelAnimationFrame(shelfAnimationFrame);
    shelfAnimationFrame = 0;
    shelfState.animationFrame = 0;
    shelfState.isAnimating = false;
    shelfState.lastFrameTime = 0;
    shelfState.keyVelocity = 0;
    shelfState.keys.left = false;
    shelfState.keys.right = false;
    currentTilt = 0;
    targetTilt = 0;
    lastScroll = bookshelf.scrollLeft;
    bookElements.forEach((entry) => entry?.el?.style.setProperty("--tilt", "0deg"));
  };
  window.stopShelfAnimation = stopShelfAnimation;

  function updateAnimation(dtTime) {
    shelfAnimationFrame = 0;
    shelfState.animationFrame = 0;
    shelfState.isAnimating = false;

    // Delta Time Calculation
    const time = dtTime || performance.now();

    if (!shelfState.lastFrameTime) shelfState.lastFrameTime = time;

    // Safety clamp: if tab was backgrounded, dt can be huge. Cap at 0.1s.
    let dt = (time - shelfState.lastFrameTime) / 1000;
    if (dt > 0.1 || dt < 0) dt = 0.016;

    shelfState.lastFrameTime = time;

    // 1. Handle Shelf Key Momentum (Manual Logic)
    if (!bookshelf.classList.contains('force-stack') && !document.body.classList.contains('view-coverflow')) {
      if (shelfState.keys.left) {
        if (shelfState.keyVelocity > -KEY_BASE_SPEED_PPS) {
          shelfState.keyVelocity = -KEY_BASE_SPEED_PPS;
        } else {
          shelfState.keyVelocity -= KEY_ACCEL_PPS * dt;
        }
        if (shelfState.keyVelocity < -KEY_MAX_SPEED_PPS) shelfState.keyVelocity = -KEY_MAX_SPEED_PPS;
      } else if (shelfState.keys.right) {
        if (shelfState.keyVelocity < KEY_BASE_SPEED_PPS) {
          shelfState.keyVelocity = KEY_BASE_SPEED_PPS;
        } else {
          shelfState.keyVelocity += KEY_ACCEL_PPS * dt;
        }
        if (shelfState.keyVelocity > KEY_MAX_SPEED_PPS) shelfState.keyVelocity = KEY_MAX_SPEED_PPS;
      } else {
        const damp = shelfState.keyVelocity * KEY_FRICTION_FACTOR * dt;
        shelfState.keyVelocity -= damp;
        if (Math.abs(shelfState.keyVelocity) < 5) shelfState.keyVelocity = 0;
      }

      if (Math.abs(shelfState.keyVelocity) > 0) {
        const moveAmount = shelfState.keyVelocity * dt;
        bookshelf.scrollLeft += moveAmount;
      }
    }

    const currentScroll = bookshelf.scrollLeft;
    maybeLoadMoreForShelf();

    // Initialize lastScroll to prevent jump
    if (lastScroll === -1) {
      lastScroll = currentScroll;
    }

    // Velocity in Pixels Per Second
    const scrollDelta = currentScroll - lastScroll;
    const velocityPPS = dt > 0 ? scrollDelta / dt : 0;
    lastScroll = currentScroll;

    // Deadzone (avoid micro-jitters when nearly stopped)
    // 50px/sec is roughly 1px/frame at 60hz
    if (Math.abs(velocityPPS) < 40) {
      targetTilt = 0;
    } else {
      // Sensitivity: 1000px/s -> ~3 degrees (Reduced from 4)
      const targetRotation = velocityPPS * 0.003;
      targetTilt = clamp(targetRotation, -25, 25);
    }

    if (reduceMotion) {
      targetTilt = 0;
      currentTilt = 0;
    } else {
      const smoothFactor = 1 - Math.exp(-5.0 * dt);
      currentTilt = lerp(currentTilt, targetTilt, smoothFactor);
    }

    const viewportLeft = currentScroll - ANIMATION_BUFFER;
    const containerWidth = window.shelfWidth || bookshelf.clientWidth; // Use cached width
    const viewportRight =
      viewportLeft + containerWidth + ANIMATION_BUFFER * 2;

    bookElements.forEach((entry) => {
      // Use cached metrics to avoid layout thrashing
      // entry structure: { el, left, width, ... } provided by measureBookPositions
      if (!entry || !entry.el) return;

      const width = entry.width || 120; // fallback to default card width
      const left = entry.left || 0;

      const rightEdge = left + width;
      // Simple bounds check with cached values
      if (rightEdge < viewportLeft || left > viewportRight) {
        return;
      }

      // Apply uniform tilt to all visible books
      entry.el.style.setProperty("--tilt", `${currentTilt}deg`);
    });

    if (shelfNeedsAnimation()) {
      wakeShelfAnimation();
    } else {
      shelfState.lastFrameTime = 0;
      currentTilt = 0;
      targetTilt = 0;
      bookElements.forEach((entry) => entry?.el?.style.setProperty("--tilt", "0deg"));
    }
  }

  bookshelf.addEventListener("scroll", wakeShelfAnimation, { passive: true });

  const handleDeepLink = () => {
    const hash = window.location.hash.slice(1); // Remove '#'
    if (!hash) {
      setActive(null, false, "none");
      return;
    }

    // Decoding might be needed for some IDs. A malformed shared URL should
    // not prevent the rest of the library from initializing.
    let rawHash = hash;
    try {
      rawHash = decodeURIComponent(hash);
    } catch (_) {
      // Keep the literal hash as a best-effort lookup value.
    }

    // Every permalink representation remains a valid alias regardless of
    // which representation is configured as the default.
    let targetIndex = allBooks.findIndex((book) => book.id === rawHash);

    if (targetIndex === -1) {
      targetIndex = allBooks.findIndex(
        (book) => book.slug && String(book.slug).toLowerCase() === rawHash.toLowerCase(),
      );
    }

    const hashIsbn = sanitizeIsbn(rawHash);
    if (targetIndex === -1 && hashIsbn) {
      targetIndex = allBooks.findIndex(
        (book) => book.isbn && sanitizeIsbn(book.isbn) === hashIsbn,
      );
    }

    if (targetIndex === -1) {
      targetIndex = allBooks.findIndex(
        (book) => book.titleSlug && String(book.titleSlug).toLowerCase() === rawHash.toLowerCase(),
      );
    }

    // Last-resort compatibility for older title-based links.
    // If no ID found, treat hash as a search query (e.g. "great-gatsby" -> "great gatsby")
    // We check if title CONTAINS the dash-less query
    if (targetIndex === -1 && rawHash.length > 2) {
      const query = normalizeText(rawHash.replace(/-/g, " ")); // "great-gatsby" -> "great gatsby"
      targetIndex = allBooks.findIndex(b => {
        const title = normalizeText(b.title || "");
        return title.includes(query);
      });
    }

    if (targetIndex === -1) {
      console.warn("Deep link book not found:", rawHash);
      return;
    }

    // Found it! Get the ID for logic
    // Even if we found it via title, we must rely on the ID for setActive() to work
    const targetId = allBooks[targetIndex].id;

    if (!viewableBooks.some((book) => book.id === targetId)) {
      if (searchInput) searchInput.value = "";
      viewableBooks = [...allBooks];
      window.viewableBookCount = viewableBooks.length;
      applySort();
      renderBooks();
      targetIndex = viewableBooks.findIndex((book) => book.id === targetId);
    } else {
      targetIndex = viewableBooks.findIndex((book) => book.id === targetId);
    }

    // Ensure the book is loaded
    // Calculate required batch count (0-based index)
    // If index is 75 and batch is 50. Batches needed: 0(0-49), 1(50-99).
    // nextBookIndex needs to be > 75.
    while (nextBookIndex <= targetIndex) {
      loadMoreBooks();
    }

    // Wait for DOM to update
    requestAnimationFrame(() => {
      const targetElement = bookElements.find(e => e.book.id === targetId);

      if (targetElement && targetElement.el) {
        setActive(targetId, false, "none");

        // Scroll to center
        const container = bookshelf;
        const element = targetElement.el;

        // Handle scrolling based on layout mode
        if (bookshelf.classList.contains("force-stack")) {
          element.scrollIntoView({ behavior: preferredScrollBehavior, block: 'center' });
        } else {
          const containerWidth = container.clientWidth;
          const targetLeft = element.offsetLeft;
          const targetWidth = element.offsetWidth;
          container.scrollTo({
            left: targetLeft - (containerWidth / 2) + (targetWidth / 2),
            behavior: preferredScrollBehavior
          });
        }
      }
    });
  };

  // Listen for hash changes (e.g. user back button)
  window.addEventListener('hashchange', handleDeepLink);

  // --- Initialization ---
  initResponsiveLayout();
  initSearch();

  applySort();
  renderBooks();

  const defaultSortOption = sortOptions ? Array.from(sortOptions).find(opt => opt.dataset.value === currentSort) : null;
  if (defaultSortOption) defaultSortOption.setAttribute('aria-selected', 'true');


  // Check for deep link on startup
  // We wrap in timeout to ensure initial render/layout is complete
  setTimeout(handleDeepLink, 300);

  setupObservers();
  initIsbnPopup();
  wakeShelfAnimation();
})();

/* --- View Switching Logic --- */
(function () {
  const configuredView = window.bookshelfConfig?.defaultView;
  const defaultView = ['shelf', 'stack', 'coverflow'].includes(configuredView)
    ? configuredView
    : 'shelf';
  const scrollSpeedFactors = Object.freeze({
    slow: 0.65,
    normal: 1,
    fast: 1.6,
  });
  const configuredShelfScrollSpeed = window.bookshelfConfig?.shelfScrollSpeed;
  const configuredCoverflowScrollSpeed = window.bookshelfConfig?.coverflowScrollSpeed;
  const shelfScrollFactor = scrollSpeedFactors[configuredShelfScrollSpeed] || scrollSpeedFactors.normal;
  const coverflowScrollFactor = scrollSpeedFactors[configuredCoverflowScrollSpeed] || scrollSpeedFactors.normal;
  let currentView = defaultView;
  let coverflowInitialized = false;
  let coverflowIndex = 0;
  let cfBg = null;
  let cfTitle = null;
  let cfDetails = null;
  let cfDetailsTimer = null;
  let cfSliderContainer = null;
  let cfSlider = null;
  let cfBookCache = [];
  let cfCacheVersion = -1;
  const CF_IDLE_TIME = 500; // 0.5s

  // Config
  const C_SPACING = 50;
  const C_OFFSET = 200;
  const C_ROTATION = 60;
  const C_Z_DEPTH = -200;

  // Elements
  const body = document.body;
  const bookshelf = document.getElementById('bookshelf');
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  let shelfScrollTarget = bookshelf.scrollLeft;
  let shelfScrollFrame = null;
  let shelfScrollTime = 0;

  const animateShelfScroll = (time) => {
    const elapsed = shelfScrollTime ? Math.min((time - shelfScrollTime) / 1000, 0.05) : 0.016;
    shelfScrollTime = time;
    const distance = shelfScrollTarget - bookshelf.scrollLeft;
    if (Math.abs(distance) < 0.5) {
      bookshelf.scrollLeft = shelfScrollTarget;
      shelfScrollFrame = null;
      shelfScrollTime = 0;
      return;
    }
    const progress = 1 - Math.exp(-14 * elapsed);
    bookshelf.scrollLeft += distance * progress;
    shelfScrollFrame = requestAnimationFrame(animateShelfScroll);
  };

  const scrollShelfBy = (distance) => {
    if (shelfScrollFrame === null) {
      shelfScrollTarget = bookshelf.scrollLeft;
    }
    const limit = Math.max(0, bookshelf.scrollWidth - bookshelf.clientWidth);
    shelfScrollTarget = Math.max(0, Math.min(limit, shelfScrollTarget + distance * shelfScrollFactor));
    if (reduceMotion) {
      bookshelf.scrollLeft = shelfScrollTarget;
      return;
    }
    if (shelfScrollFrame === null) {
      shelfScrollFrame = requestAnimationFrame(animateShelfScroll);
    }
  };

  // Custom Select Elements for View (Initialized in initView)
  let viewControl = null;
  let viewTrigger = null;
  let viewList = null;
  let viewOptions = [];

  function initView() {
    // Global Event Delegation for View Control
    document.addEventListener('click', (e) => {
      const viewCtrl = document.getElementById('view-control');
      const trigger = viewCtrl ? viewCtrl.querySelector('.select-trigger') : null;
      const list = viewCtrl ? viewCtrl.querySelector('.select-options') : null;

      if (!viewCtrl || !trigger || !list) return;

      const clickedTrigger = e.target.closest('.select-trigger');
      const clickedOption = e.target.closest('.option');

      // 1. Click on Trigger
      if (clickedTrigger && viewCtrl.contains(clickedTrigger)) {
        e.stopPropagation();

        // Close Sort Menu
        const sortCtrl = document.getElementById('sort-control');
        if (sortCtrl) {
          const sortList = sortCtrl.querySelector('.select-options');
          const sortTrigger = sortCtrl.querySelector('.select-trigger');
          if (sortList) sortList.hidden = true;
          if (sortTrigger) sortTrigger.setAttribute('aria-expanded', 'false');
        }

        const isExpanded = trigger.getAttribute('aria-expanded') === 'true';
        const newState = !isExpanded;

        trigger.setAttribute('aria-expanded', String(newState));
        list.hidden = !newState;
        return;
      }

      // 2. Click on Option
      if (clickedOption && viewCtrl.contains(clickedOption)) {
        e.preventDefault();
        e.stopPropagation();

        // Close Menu IMMEDIATELY
        list.hidden = true;
        trigger.setAttribute('aria-expanded', 'false');

        const view = clickedOption.dataset.view;
        if (view) {
          setTimeout(() => {
            switchView(view);
          }, 10);
        }

        return;
      }

      // 3. Click Outside (Close)
      if (!viewCtrl.contains(e.target)) {
        if (!list.hidden) {
          list.hidden = true;
          trigger.setAttribute('aria-expanded', 'false');
        }
      }
    });

    // Mobile check
    if (window.matchMedia('(max-width: 768px)').matches) {
      switchView('stack', true);
    } else {
      // Check Session Storage for persistence
      const storedView = sessionStorage.getItem('preferredView');
      if (storedView && ['shelf', 'stack', 'coverflow'].includes(storedView)) {
        switchView(storedView, true);
      } else {
        switchView(defaultView, true);
      }
    }


    const opts = document.querySelectorAll('#view-control .option');
    opts.forEach(opt => {
      opt.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          switchView(opt.dataset.view);
          const viewCtrl = document.getElementById('view-control');
          const list = viewCtrl.querySelector('.select-options');
          const trigger = viewCtrl.querySelector('.select-trigger');
          if (list) list.hidden = true;
          if (trigger) trigger.setAttribute('aria-expanded', 'false');
        }
      });
    });
    initBackToTop();
    initAutoHideControls();
  }

  function initAutoHideControls() {
    const controls = document.querySelector('.controls');
    if (!controls) return;

    let lastScrollY = window.scrollY;
    let ticking = false;

    const onScroll = () => {
      const currentScrollY = window.scrollY;
      const scrollDiff = currentScrollY - lastScrollY;
      const isScrollingDown = scrollDiff > 0;
      const isScrollingUp = scrollDiff < 0;

      // Threshold to start hiding (avoid hiding at very top)
      const threshold = 100;

      if (isScrollingDown && currentScrollY > threshold) {
        controls.classList.add('is-hidden');
        // Close any open dropdowns when hiding
        const openDropdowns = controls.querySelectorAll('.select-trigger[aria-expanded="true"]');
        openDropdowns.forEach(t => {
          t.setAttribute('aria-expanded', 'false');
          const listId = t.getAttribute('aria-controls') || t.nextElementSibling?.id; // sibling usually
          if (t.nextElementSibling && t.nextElementSibling.classList.contains('select-options')) {
            t.nextElementSibling.hidden = true;
          }
        });
      } else if (isScrollingUp || currentScrollY < threshold) {
        controls.classList.remove('is-hidden');
      }

      lastScrollY = currentScrollY;
      ticking = false;
    };

    window.addEventListener('scroll', () => {
      if (!ticking) {
        window.requestAnimationFrame(onScroll);
        ticking = true;
      }
    }, { passive: true });
  }

  function initBackToTop() {
    const btn = document.querySelector('.back-to-top');
    if (!btn) return;

    btn.hidden = false; // logic handles visibility class

    const checkScroll = () => {
      // Only show in Stack View (or on mobile where stack is forced)
      // Access window.currentView set by switchView
      const isStack = window.currentView === 'stack' || window.matchMedia('(max-width: 768px)').matches;

      if (isStack && window.scrollY > 300) {
        btn.classList.add('is-visible');
      } else {
        btn.classList.remove('is-visible');
      }
    };

    window.addEventListener('scroll', checkScroll, { passive: true });
    window.addEventListener('resize', checkScroll);

    btn.addEventListener('click', () => {
      window.scrollTo({ top: 0, behavior: reduceMotion ? 'auto' : 'smooth' });
      setActive(null);
    });
  }

  function switchView(view, skipSync = false) {
    const prevView = currentView;
    currentView = view;
    window.currentView = view; // Expose for Random Button

    // Save preference
    try {
      sessionStorage.setItem('preferredView', view);
    } catch (e) { }

    // Trigger Fade-In Animation
    const shelfContainer = document.getElementById('bookshelf');
    if (shelfContainer) {
      shelfContainer.classList.remove('view-animate');
      if (!reduceMotion) {
        void shelfContainer.offsetWidth;
        shelfContainer.classList.add('view-animate');
      }
    }

    // Toggle Instruction Text
    const instruction = document.getElementById('instruction-text');
    const hasSubtitle = Boolean(String(
      window.bookshelfConfig?.siteSubtitle ?? "Click on a book spine to see its details"
    ).trim());
    if (instruction) {
      instruction.style.display = (view === 'shelf' && hasSubtitle) ? 'block' : 'none';
    }

    // Update Dropdown Selection State
    const viewCtrl = document.getElementById('view-control');
    if (viewCtrl) {
      const opts = viewCtrl.querySelectorAll('.option');
      opts.forEach(opt => {
        const isActive = opt.dataset.view === view;
        opt.setAttribute('aria-selected', String(isActive));
      });
    }

    // Update Body Classes
    body.classList.remove('view-shelf', 'view-stack', 'view-coverflow');
    body.classList.add('view-' + view);
    if (view === 'shelf') {
      requestAnimationFrame(() => window.wakeShelfAnimation?.());
    } else {
      window.stopShelfAnimation?.();
    }

    if (view === 'coverflow') {
      // 1. Hide immediately to prevent visual artifacts
      if (shelfContainer) shelfContainer.style.opacity = '0';

      // 2. Kill transitions
      // const books = document.querySelectorAll('.book');
      // books.forEach(b => b.style.transition = 'none'); 
      // Physics loop handles interpolation so css transition not needed for position?
      // Actually we are setting transform every frame. so Transition should be NONE.
      // 3. Init Layout
      initCoverflow();
      startCoverflowLoop(); // START LOOP

      // 4. Force Reflow & Double-check positions
      void document.body.offsetWidth;
      // updateCoverflow(); // Loop does this

      // 5. Restore visibility
      requestAnimationFrame(() => {
        setTimeout(() => {
          if (shelfContainer) shelfContainer.style.opacity = '';
        }, 50);
      });

    } else {
      // Capture current position before stopping loop
      const targetIndex = Math.round(cfState.index);

      stopCoverflowLoop(); // STOP LOOP
      resetStyles();

      // Ensure layout is recalculated since we un-hid books
      measureBookPositions();

      // Sync Scroll Position
      if (!skipSync) {
        const books = Array.from(document.querySelectorAll('.book'));
        let targetBook = null;

        if (prevView === 'coverflow' && books[targetIndex]) {
          // Coming from Coverflow: Scroll to the book visible in Coverflow, but DO NOT activate (change URL)
          targetBook = books[targetIndex];
        } else if (window.activeId) {
          // Shelf <-> Stack: Keep the currently active book visible
          targetBook = books.find(b => b.dataset.id === window.activeId);
        }

        if (targetBook) {
          // Scroll to keep context without triggering setActive/URL change unless it was already active
          requestAnimationFrame(() => {
            if (view === 'stack') {
              targetBook.scrollIntoView({ behavior: 'auto', block: 'center' });
            } else {
              // Shelf View
              const container = document.getElementById('bookshelf');
              const centerPos = targetBook.offsetLeft - (container.clientWidth / 2) + (targetBook.offsetWidth / 2);
              container.scrollTo({ left: centerPos, behavior: 'auto' });
            }
          });
        }
      }
    }
  }

  function resetStyles() {
    // Remove inline transforms from books so CSS grid/flex takes over
    const books = document.querySelectorAll('.book');
    books.forEach(b => {
      b.style.transform = '';
      b.style.left = '';
      b.style.top = '';
      b.style.zIndex = '';
      b.style.width = '';
      b.style.height = '';
      b.style.display = ''; // Critical: Unhide culled books
      b.style.backgroundImage = '';
      b.style.background = ''; // Critical: Clear gradients/shorthand so CSS var(--bg) works

      b.classList.remove('dimmed');
      b.classList.remove('is-3d-active');
    });

    if (cfBg) cfBg.style.opacity = '0';
    if (cfTitle) cfTitle.classList.remove('visible');
    if (cfDetails) cfDetails.classList.remove('visible');
  }

  // Coverflow Physics State
  const cfState = {
    index: 0,        // Current float position
    velocity: 0,     // Current speed
    lastTime: 0,
    isActive: false,  // Is the loop running
    isAnimating: false,
    cacheVersion: -1,
    keys: { left: false, right: false },
    targetIndex: null // Target for auto-scrolling
  };
  window.cfState = cfState; // Expose for Random Button
  let cfReqId = 0;

  // Constants
  const PHYS_FRICTION = 0.90;
  const PHYS_SNAP_STRENGTH = 0.02; // Reduced from 0.04 for smoother settling
  const PHYS_MAX_VEL = 0.35;
  const PHYS_WHEEL_FACTOR = 0.0008;
  const PHYS_KEY_ACCEL = 0.002 * coverflowScrollFactor;

  function initCoverflow() {
    if (!coverflowInitialized) {
      cfBg = document.createElement('div');
      cfBg.className = 'coverflow-bg';
      body.appendChild(cfBg);

      cfTitle = document.createElement('div');
      cfTitle.className = 'coverflow-title';
      body.appendChild(cfTitle);

      cfDetails = document.createElement('div');
      cfDetails.className = 'coverflow-details';
      body.appendChild(cfDetails);

      // Create Slider
      cfSliderContainer = document.createElement('div');
      cfSliderContainer.className = 'coverflow-slider-container';

      cfSlider = document.createElement('input');
      cfSlider.type = 'range';
      cfSlider.className = 'coverflow-slider';
      cfSlider.min = 0;
      cfSlider.step = 0.01;

      // Events
      cfSlider.addEventListener('input', (e) => {
        const val = parseFloat(e.target.value);

        // Lazy Load Catch-up
        // If we are dragging to an index that isn't loaded yet, force load
        const currentLoaded = refreshCoverflowCache().length;
        if (val > currentLoaded - 5) { // buffer
          while (refreshCoverflowCache().length <= val + 5) {
            if (refreshCoverflowCache().length >= (window.viewableBookCount || 0)) break;
            if (window.loadMoreBooks) window.loadMoreBooks(true);
            else break;
          }
        }

        cfState.targetIndex = null; // Clear auto-scroll
        cfState.index = val;
        cfState.velocity = 0;
        updateCoverflow();

        // Hide details while scrubbing
        if (cfDetails) cfDetails.classList.remove('visible');
        clearTimeout(cfDetailsTimer);
        cfDetailsTimer = null;
      });

      // Prevent other interactions
      cfSlider.addEventListener('click', e => e.stopPropagation());
      cfSlider.addEventListener('mousedown', e => e.stopPropagation());
      cfSlider.addEventListener('touchstart', e => e.stopPropagation());

      cfSliderContainer.appendChild(cfSlider);
      body.appendChild(cfSliderContainer);

      // Reset timer on stop
      cfSlider.addEventListener('change', () => {
        clearTimeout(cfDetailsTimer);
        cfDetailsTimer = null;
        scheduleCoverflowDetails();
      });

      coverflowInitialized = true;
    }

    const books = refreshCoverflowCache();

    if (cfState.index >= books.length) cfState.index = 0;

    cfState.velocity = 0;
    cfState.keys.left = false;
    cfState.keys.right = false;
  }

  function startCoverflowLoop() {
    cfState.isActive = true;

    if (cfSliderContainer) cfSliderContainer.classList.add('visible');
    if (cfSlider) {
      cfSlider.max = Math.max(0, (window.viewableBookCount || document.querySelectorAll('.book').length) - 1);
    }
    updateCoverflow();
    scheduleCoverflowDetails();
    if (coverflowNeedsAnimation()) wakeCoverflow();
  }

  function wakeCoverflow() {
    if (!cfState.isActive || cfReqId !== 0) return;
    cfState.isAnimating = true;
    cfReqId = requestAnimationFrame(runCoverflowFrame);
  }
  window.wakeCoverflow = wakeCoverflow;

  function runCoverflowFrame() {
    cfReqId = 0;
    cfState.isAnimating = false;
    if (!cfState.isActive) return;

    if (cfState.targetIndex !== null) {
      const diff = cfState.targetIndex - cfState.index;
      if (Math.abs(diff) < 0.005) {
        cfState.index = cfState.targetIndex;
        cfState.velocity = 0;
        cfState.targetIndex = null;
      } else {
        const desired = diff * 0.06;
        cfState.velocity = Math.max(Math.min(desired, PHYS_MAX_VEL), -PHYS_MAX_VEL);
      }
    } else {
      cfState.velocity *= PHYS_FRICTION;
    }

    if (cfState.keys.left) {
      cfState.velocity -= PHYS_KEY_ACCEL;
      cfState.targetIndex = null;
    }
    if (cfState.keys.right) {
      cfState.velocity += PHYS_KEY_ACCEL;
      cfState.targetIndex = null;
    }

    const clampedVel = Math.max(Math.min(cfState.velocity, PHYS_MAX_VEL), -PHYS_MAX_VEL);
    cfState.index += clampedVel;

    const maxIdx = Math.max(0, refreshCoverflowCache().length - 1);
    if (cfState.index < 0) {
      cfState.index = 0;
      cfState.velocity = 0;
    }
    if (cfState.index > maxIdx) {
      cfState.index = maxIdx;
      cfState.velocity = 0;
    }

    const isKeyHeld = cfState.keys.left || cfState.keys.right;
    if (!isKeyHeld && cfState.targetIndex === null && Math.abs(cfState.velocity) < 0.01) {
      const target = Math.round(cfState.index);
      const diff = target - cfState.index;
      if (Math.abs(diff) > 0.001) cfState.index += diff * PHYS_SNAP_STRENGTH;
      else cfState.index = target;
    }

    const moving = coverflowNeedsAnimation();
    if (moving) {
      clearTimeout(cfDetailsTimer);
      cfDetailsTimer = null;
      if (cfDetails) cfDetails.classList.remove('visible');
    }

    coverflowIndex = Math.round(cfState.index);
    updateCoverflow();

    if (moving) wakeCoverflow();
    else scheduleCoverflowDetails();
  }

  function coverflowNeedsAnimation() {
    return cfState.targetIndex !== null ||
      cfState.keys.left ||
      cfState.keys.right ||
      Math.abs(cfState.velocity) > 0.001 ||
      Math.abs(Math.round(cfState.index) - cfState.index) > 0.001;
  }

  function scheduleCoverflowDetails() {
    if (cfDetailsTimer || !cfDetails || cfDetails.classList.contains('visible')) return;
    cfDetailsTimer = setTimeout(() => {
      cfDetailsTimer = null;
      if (cfState.isActive && !coverflowNeedsAnimation()) showCoverflowDetails();
    }, CF_IDLE_TIME);
  }

  function setCoverflowIndexImmediately(index) {
    const target = Math.max(0, Math.min(
      Math.round(index),
      Math.max(0, (window.viewableBookCount || 1) - 1),
    ));
    while (refreshCoverflowCache().length <= target) {
      const before = refreshCoverflowCache().length;
      if (window.loadMoreBooks) window.loadMoreBooks(true);
      if (refreshCoverflowCache().length === before) break;
    }
    cfState.targetIndex = null;
    cfState.velocity = 0;
    cfState.index = Math.min(target, Math.max(0, refreshCoverflowCache().length - 1));
    coverflowIndex = Math.round(cfState.index);
    clearTimeout(cfDetailsTimer);
    cfDetailsTimer = null;
    if (cfDetails) cfDetails.classList.remove('visible');
    updateCoverflow();
    scheduleCoverflowDetails();
  }

  function seekCoverflow(index) {
    if (reduceMotion) {
      setCoverflowIndexImmediately(index);
      return;
    }
    cfState.targetIndex = index;
    wakeCoverflow();
  }
  window.seekCoverflow = seekCoverflow;

  function showCoverflowDetails() {
    if (!cfDetails) return;
    const books = refreshCoverflowCache();
    const activeBook = books[coverflowIndex];
    if (!activeBook) return;

    const data = activeBook.bookData || (window.booksData && window.booksData.find(b => b.id == activeBook.dataset.id));
    if (!data) return;

    cfDetails.replaceChildren();
    const author = appendTextElement(cfDetails, "div", "cf-author", data.author || "");
    if (data.translator) {
      author.appendChild(document.createTextNode(" "));
      appendTextElement(author, "span", "cf-trans", `(${data.translator})`);
    }
    const metaRow = appendTextElement(cfDetails, "div", "cf-meta-row", "");
    [data.published, data.publisher, data.binding, data.isbn].forEach((value) => {
      if (value) appendTextElement(metaRow, "span", "", value);
    });
    cfDetails.classList.add('visible');
  }

  function stopCoverflowLoop() {
    cfState.isActive = false;
    if (cfReqId !== 0) cancelAnimationFrame(cfReqId);
    cfReqId = 0;
    cfState.isAnimating = false;
    clearTimeout(cfDetailsTimer);
    cfDetailsTimer = null;
    if (cfDetails) cfDetails.classList.remove('visible');
    if (cfSliderContainer) cfSliderContainer.classList.remove('visible');
  }

  // Refactored Update Function (Handles Float)
  function updateCoverflow() {
    let books = refreshCoverflowCache();

    if (!books.length) return;

    if (books.length - cfState.index < 30) {
      const previousVersion = cfCacheVersion;
      if (window.loadMoreBooks) window.loadMoreBooks(true);
      if (previousVersion !== (window.bookshelfRenderVersion || 0)) {
        books = refreshCoverflowCache();
      }
    }

    const idx = cfState.index; // Float index
    const intIdx = Math.round(idx);

    // Update Background/Title (throttled/only on change)
    const activeBook = books[intIdx];
    if (activeBook) {
      const activeData = activeBook.bookData || (window.booksData && window.booksData.find(b => b.id == activeBook.dataset.id));
      if (activeData) {
        if (cfTitle) {
          if (cfTitle.textContent !== activeData.title) cfTitle.textContent = activeData.title;
          if (!cfTitle.classList.contains('visible')) cfTitle.classList.add('visible');
        }
        if (cfBg && activeData.cover) {
          const url = activeData.cover.replace(/\\/g, '/');
          // Simple substring check to avoid constant string parsing
          if (!cfBg.style.backgroundImage.includes(url)) {
            cfBg.style.backgroundImage = 'url("' + url + '")';
          }
          if (cfBg.style.opacity !== '1') cfBg.style.opacity = '1';
        }
      }
    }

    // Update Slider UI (if not being dragged)
    if (cfSlider && document.activeElement !== cfSlider) {
      cfSlider.max = Math.max(0, (window.viewableBookCount || books.length) - 1);
      const diff = Math.abs(cfSlider.value - cfState.index);
      if (diff > 0.01) cfSlider.value = cfState.index;
    }

    const VISIBLE_RANGE = 15;
    const len = books.length;

    for (let i = 0; i < len; i++) {
      const el = books[i];
      const dist = i - idx; // Float distance
      const absDist = Math.abs(dist);

      // Culling (Smart Display)
      if (absDist > VISIBLE_RANGE) {
        if (el.style.display !== 'none') el.style.display = 'none';
        continue;
      }
      if (el.style.display !== 'block') el.style.display = 'block';

      let x = 0;
      let z = 0;
      let rot = 0;

      const roundedDist = Math.abs(i - intIdx); // Integer distance for classes/stacking
      let zIndex = 1000 - roundedDist;

      if (roundedDist === 0) zIndex = 2000;
      else if (roundedDist === 1) zIndex = 1500;

      // -- ANIMATION LOGIC --
      if (absDist <= 1) {
        x = dist * 200;
        z = 100 - (300 * absDist);
        rot = dist * -60;
      } else {
        const sign = Math.sign(dist);
        x = sign * (200 + (absDist - 1) * 50);
        z = -200;
        rot = sign * -60;
      }

      // Classes
      if (roundedDist === 0) {
        if (!el.classList.contains('is-3d-active')) {
          el.classList.add('is-3d-active');
          el.classList.remove('dimmed');
        }
      } else {
        if (el.classList.contains('is-3d-active')) {
          el.classList.remove('is-3d-active');
          el.classList.add('dimmed');
        }
      }

      // Apply Transform
      // Only valid if visible (handled by culling)
      el.style.transform = `translateX(${x}px) translateZ(${z}px) rotateY(${rot}deg)`;
      el.style.zIndex = zIndex;

      el.onclick = (e) => {
        if (currentView === 'coverflow') {
          e.stopPropagation();
          if (i !== Math.round(cfState.index)) {
            seekCoverflow(i);
          }
          return;
        }
        if (currentView === 'stack') {
          e.stopImmediatePropagation();
          e.preventDefault();
        }
      };
    }
  }

  function refreshCoverflowCache() {
    const version = window.bookshelfRenderVersion || 0;
    if (cfCacheVersion !== version) {
      cfBookCache = Array.from(document.querySelectorAll('.book'));
      cfCacheVersion = version;
      cfState.cacheVersion = version;
    }
    return cfBookCache;
  }

  window.addEventListener('bookshelf:rendered', () => {
    cfCacheVersion = -1;
    if (!cfState.isActive) return;
    const books = refreshCoverflowCache();
    const maxIndex = Math.max(0, books.length - 1);
    cfState.index = Math.min(cfState.index, maxIndex);
    updateCoverflow();
    if (coverflowNeedsAnimation()) wakeCoverflow();
    else scheduleCoverflowDetails();
  });

  // Input Handlers
  const keyboardEventBelongsToControl = (event) => {
    if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) return true;
    if (!document.querySelector("#stats-overlay")?.hidden ||
      document.querySelector("#isbn-popup")?.classList.contains("is-visible") ||
      document.querySelector('.select-trigger[aria-expanded="true"]')) {
      return true;
    }
    const control = event.target instanceof Element
      ? event.target.closest("input, textarea, select, button, a, [contenteditable], [role='option']")
      : null;
    return Boolean(control && !control.classList.contains("book"));
  };

  window.addEventListener('keydown', (e) => {
    if (keyboardEventBelongsToControl(e)) return;
    if (currentView === 'coverflow') {
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        if (reduceMotion) {
          setCoverflowIndexImmediately(cfState.index - 1);
          return;
        }
        cfState.keys.left = true;
        cfState.targetIndex = null;
        wakeCoverflow();
      }
      if (e.key === 'ArrowRight') {
        e.preventDefault();
        if (reduceMotion) {
          setCoverflowIndexImmediately(cfState.index + 1);
          return;
        }
        cfState.keys.right = true;
        cfState.targetIndex = null;
        wakeCoverflow();
      }
    } else if (currentView === 'shelf') {
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        shelfState.keys.left = true;
        window.wakeShelfAnimation?.();
      }
      if (e.key === 'ArrowRight') {
        e.preventDefault();
        shelfState.keys.right = true;
        window.wakeShelfAnimation?.();
      }
    }
  });

  window.addEventListener('keyup', (e) => {
    if (currentView === 'coverflow') {
      if (reduceMotion) return;
      if (e.key === 'ArrowLeft') {
        const wasHeld = cfState.keys.left;
        cfState.keys.left = false;
        if (wasHeld) wakeCoverflow();
      }
      if (e.key === 'ArrowRight') {
        const wasHeld = cfState.keys.right;
        cfState.keys.right = false;
        if (wasHeld) wakeCoverflow();
      }
    } else if (currentView === 'shelf') {
      if (e.key === 'ArrowLeft') {
        const wasHeld = shelfState.keys.left;
        shelfState.keys.left = false;
        if (wasHeld) window.wakeShelfAnimation?.();
      }
      if (e.key === 'ArrowRight') {
        const wasHeld = shelfState.keys.right;
        shelfState.keys.right = false;
        if (wasHeld) window.wakeShelfAnimation?.();
      }
    }
  });

  window.addEventListener('wheel', (e) => {
    if (currentView === 'coverflow') {
      e.preventDefault();
      const delta = e.deltaY || e.deltaX;
      if (reduceMotion) {
        if (delta !== 0) setCoverflowIndexImmediately(cfState.index + Math.sign(delta));
        return;
      }
      cfState.targetIndex = null;
      cfState.velocity += delta * PHYS_WHEEL_FACTOR * coverflowScrollFactor;
      wakeCoverflow();
    } else if (currentView === 'shelf') {
      if (bookshelf.classList.contains('force-shelf') && bookshelf.contains(e.target)) {
        const delta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX;
        if (delta !== 0) {
          e.preventDefault();
          const unit = e.deltaMode === WheelEvent.DOM_DELTA_LINE
            ? 24
            : e.deltaMode === WheelEvent.DOM_DELTA_PAGE
              ? bookshelf.clientWidth
              : 1;
          scrollShelfBy(delta * unit);
        }
      }
    }
  }, { passive: false });

  // Run Init
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initView);
  } else {
    initView();
  }

})();
// --- Library Statistics Logic ---
(function () {
  const statsToggle = document.querySelector('.stats-toggle');
  const statsOverlay = document.querySelector('#stats-overlay');
  const statsClose = document.querySelector('.stats-close');
  const statsCard = document.querySelector('.stats-card');

  const statsTotal = document.querySelector('#stats-total-books');
  const statsAuthor = document.querySelector('#stats-top-author');
  const statsPublisher = document.querySelector('#stats-top-publisher');
  const decadeChart = document.querySelector('#decade-chart');

  if (!statsToggle || !statsOverlay) return;
  if (window.bookshelfConfig?.showStatistics === false) {
    statsToggle.hidden = true;
    statsOverlay.hidden = true;
    return;
  }

  const calculateStats = () => {
    const books = window.booksData || [];
    if (!books.length) return;

    // 1. Core Metrics
    const authors = {};
    const publishers = {};
    const years = {};
    const bindings = {};
    let translatedCount = 0;
    const colors = {};

    books.forEach(b => {
      // Authors
      if (b.author) authors[b.author] = (authors[b.author] || 0) + 1;
      // Publishers
      if (b.publisher) publishers[b.publisher] = (publishers[b.publisher] || 0) + 1;
      // Years
      if (b.published) years[b.published] = (years[b.published] || 0) + 1;
      // Bindings
      if (b.binding) bindings[b.binding] = (bindings[b.binding] || 0) + 1;
      // Translations
      if (b.translator) translatedCount++;
      // Colors
      if (b.spineColor) colors[b.spineColor] = (colors[b.spineColor] || 0) + 1;
    });

    // Top Author
    const sortedAuthors = Object.entries(authors).sort((a, b) => b[1] - a[1]);
    const topAuthor = sortedAuthors[0];

    // Top Publisher
    const topPub = Object.entries(publishers).sort((a, b) => b[1] - a[1])[0];

    // Golden Year
    const topYear = Object.entries(years).sort((a, b) => b[1] - a[1])[0];

    // Update Basic UI
    statsTotal.textContent = books.length;
    document.querySelector('#stats-unique-authors').textContent = Object.keys(authors).length;
    document.querySelector('#stats-unique-publishers').textContent = Object.keys(publishers).length;
    document.querySelector('#stats-golden-year').textContent = topYear ? `${topYear[0]} (${topYear[1]})` : '-';
    statsAuthor.textContent = topAuthor ? topAuthor[0] : '-';
    statsPublisher.textContent = topPub ? topPub[0] : '-';

    const transPercent = Math.round((translatedCount / books.length) * 100);
    document.querySelector('#stats-translation-rate').textContent = `${transPercent}%`;

    // 2. Binding Composition (Segment Bar)
    const bindingBar = document.querySelector('#binding-bar');
    const bindingLegend = document.querySelector('#binding-legend');
    bindingBar.replaceChildren();
    bindingLegend.replaceChildren();

    const bindingColors = ['#A5C9FF', '#FFC4C4', '#B2EBD6', '#FCF1B6', '#E0CFFF'];
    const sortedBindings = Object.entries(bindings).sort((a, b) => b[1] - a[1]);

    const totalKnownBindings = Object.values(bindings).reduce((a, b) => a + b, 0);
    sortedBindings.forEach(([type, count], i) => {
      const percentage = (count / totalKnownBindings) * 100;
      const color = bindingColors[i % bindingColors.length];

      // Bar Segment
      const segment = document.createElement('div');
      segment.className = 'segment';
      segment.style.backgroundColor = color;
      segment.style.width = '0%';
      segment.dataset.width = `${percentage}%`;
      bindingBar.appendChild(segment);

      // Legend Item
      const legendItem = document.createElement('div');
      legendItem.className = 'legend-item';
      const legendDot = appendTextElement(legendItem, "span", "legend-dot", "");
      legendDot.style.backgroundColor = color;
      appendTextElement(legendItem, "span", "", `${type} (${Math.round(percentage)}%)`);
      bindingLegend.appendChild(legendItem);
    });

    // 3. Color Signature
    const dominantColor = Object.entries(colors).sort((a, b) => b[1] - a[1])[0];
    const colorSigEl = document.querySelector('#color-signature');
    const colorNameEl = document.querySelector('#color-name');

    if (dominantColor) {
      colorSigEl.style.backgroundColor = dominantColor[0];
      // Simple color naming logic
      colorNameEl.textContent = dominantColor[0].toUpperCase();
    }

    // 4. Decade Distribution
    const decades = {};
    books.forEach(b => {
      if (b.published) {
        const d = Math.floor(b.published / 10) * 10;
        decades[d] = (decades[d] || 0) + 1;
      }
    });

    const sortedDecades = Object.entries(decades).sort((a, b) => b[0] - a[0]);
    const maxCount = Math.max(...Object.values(decades));

    decadeChart.replaceChildren();
    sortedDecades.forEach(([decade, count]) => {
      const percentage = (count / maxCount) * 100;
      const row = document.createElement('div');
      row.className = 'chart-row';
      appendTextElement(row, "span", "chart-label", `${decade}s`);
      const wrapper = appendTextElement(row, "div", "chart-bar-wrapper", "");
      const bar = appendTextElement(wrapper, "div", "chart-bar", "");
      bar.style.width = "0%";
      bar.dataset.width = `${percentage}%`;
      appendTextElement(row, "span", "chart-count", count);
      decadeChart.appendChild(row);
    });
  };

  const openStats = () => {
    calculateStats();
    statsOverlay.hidden = false;
    void statsOverlay.offsetWidth;

    // Trigger bar animations
    const bars = document.querySelectorAll('.chart-bar, .segment');
    setTimeout(() => {
      bars.forEach(bar => {
        bar.style.width = bar.dataset.width;
      });
    }, 50);
  };

  const closeStats = () => {
    statsOverlay.hidden = true;
  };

  statsToggle.addEventListener('click', (e) => {
    e.stopPropagation();
    openStats();
  });

  statsClose.addEventListener('click', closeStats);
  statsOverlay.addEventListener('click', (e) => {
    if (e.target === statsOverlay) closeStats();
  });

})();

// Close exactly one active interface layer per Escape press.
(function () {
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;

    let handled = false;
    const statsOverlay = document.querySelector("#stats-overlay");
    const statsToggle = document.querySelector(".stats-toggle");
    if (statsOverlay && !statsOverlay.hidden) {
      statsOverlay.hidden = true;
      statsToggle?.focus();
      handled = true;
    } else {
      const isbnPopup = document.querySelector("#isbn-popup");
      if (isbnPopup?.classList.contains("is-visible")) {
        window.closeIsbnPopup?.(true);
        handled = true;
      }
    }

    if (!handled) {
      const expanded = document.querySelector('.select-trigger[aria-expanded="true"]');
      if (expanded) {
        expanded.setAttribute("aria-expanded", "false");
        const list = expanded.closest(".custom-select")?.querySelector(".select-options");
        if (list) list.hidden = true;
        expanded.focus();
        handled = true;
      }
    }

    if (!handled) {
      const searchControl = document.querySelector("#search-control.is-active");
      if (searchControl) {
        searchControl.classList.remove("is-active");
        const searchToggle = searchControl.querySelector(".search-toggle");
        searchToggle?.setAttribute("aria-expanded", "false");
        searchToggle?.focus();
        handled = true;
      }
    }

    if (!handled && window.activeId) {
      window.setActive?.(null);
      handled = true;
    }

    if (handled) {
      event.preventDefault();
      event.stopPropagation();
    }
  });
})();
