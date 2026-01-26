(function () {
  const allBooks = Array.isArray(window.booksData) ? window.booksData : [];
  // Default sort by Author on init
  let viewableBooks = [...allBooks]; // Base list for current view (filtered or not)
  window.viewableBookCount = viewableBooks.length;
  let currentFilter = ""; // Search query
  let currentSort = "title"; // Default sort
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
  const isbnPopup = detailCard ? detailCard.querySelector("#isbn-popup") : null;
  const isbnLinks = isbnPopup ? Array.from(isbnPopup.querySelectorAll(".isbn-link")) : null;



  // Search Elements
  const searchControl = document.getElementById("search-control");
  const searchToggle = searchControl ? searchControl.querySelector(".search-toggle") : null;
  const searchInput = searchControl ? searchControl.querySelector(".search-input") : null;

  if (!bookshelf || !allBooks.length) {
    if (bookshelf) bookshelf.textContent = "No books were found.";
    eturn;
  }

  let bookElements = [];
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

  const closeIsbnPopup = () => {
    if (!isbnPopup || !isbnTrigger) return;
    isbnPopup.classList.remove("is-visible");
    isbnPopup.setAttribute("aria-hidden", "true");
    isbnTrigger.setAttribute("aria-expanded", "false");
  };

  const openIsbnPopup = () => {
    if (!isbnPopup || !isbnTrigger || isbnTrigger.disabled) return;
    isbnPopup.classList.add("is-visible");
    isbnPopup.setAttribute("aria-hidden", "false");
    isbnTrigger.setAttribute("aria-expanded", "true");
  };

  const toggleIsbnPopup = () => {
    if (!isbnPopup) return;
    if (isbnPopup.classList.contains("is-visible")) {
      closeIsbnPopup();
    } else {
      openIsbnPopup();
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
    closeIsbnPopup();
    if (!hasIsbn) {
      isbnLinks.forEach((link) => link.removeAttribute("href"));
      return;
    }
    isbnLinks.forEach((link) => {
      const source = ISBN_SOURCES[link.dataset.source];
      if (!source) return;
      link.href = source.buildUrl(cleanIsbn);
    });
  };

  const titleCaseWords = new Set([
    "a",
    "an",
    "and",
    "as",
    "at",
    "but",
    "by",
    "for",
    "in",
    "of",
    "on",
    "or",
    "the",
    "to",
    "vs",
    "via",
  ]);

  const formatTitle = (title) => {
    if (!title || typeof title !== "string") {
      return "";
    }
    const words = title
      .trim()
      .split(/\s+/)
      .map((word) => word.toLowerCase());

    return words
      .map((word, index) => {
        if (!word) return "";
        const isEdge = index === 0 || index === words.length - 1;
        if (!isEdge && titleCaseWords.has(word)) {
          return word;
        }
        return word.charAt(0).toUpperCase() + word.slice(1);
      })
      .join(" ");
  };

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
    const coverPath = normalizeCoverPath(book.cover);
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
    const cssCoverUrl = metrics.coverPath ? `url("../${metrics.coverPath}")` : "";
    const docCoverUrl = metrics.coverPath ? `url("${metrics.coverPath}")` : "";

    if (metrics.coverPath) {
      el.classList.add("has-cover");
      el.style.setProperty("--cover-image", cssCoverUrl);
    } else {
      el.classList.remove("has-cover");
      el.style.removeProperty("--cover-image");
    }

    // --- Shelf View Elements (Direct children, no wrapper) ---
    const shelfCover = `<span class="shelf-cover" aria-hidden="true"></span>`;
    const shelfText = `
        <div class="shelf-text">
             ${book.author ? `<small class="author">${book.author}</small>` : ""}
             <span class="title">${displayTitle}</span>
         </div>
      `;

    // --- Stack View Content (Details Card Look) ---
    const stackItems = [];
    if (book.publisher) stackItems.push(`<div><dt>Publisher</dt><dd>${book.publisher}</dd></div>`);
    if (book.published) stackItems.push(`<div><dt>Published</dt><dd>${book.published}</dd></div>`);
    if (book.binding) stackItems.push(`<div><dt>Binding</dt><dd>${book.binding}</dd></div>`);
    if (book.translator) stackItems.push(`<div><dt>Translator</dt><dd>${book.translator}</dd></div>`);
    if (book.isbn) {
      stackItems.push(`<div><dt>ISBN</dt><dd class="isbn-field">
                        <span class="isbn-trigger stack-isbn-btn" role="button" tabindex="0" aria-label="Open ISBN links for ${book.isbn}" aria-haspopup="true" aria-expanded="false" data-isbn="${book.isbn}">${sanitizeIsbn(book.isbn)}</span>
                    </dd></div>`);
    }

    const stackHtml = `
        <div class="stack-content">
            <div class="stack-cover" style='${docCoverUrl ? `background-image: ${docCoverUrl}` : ''}'></div>
            <div class="stack-body">
                <div class="stack-heading">
                    <p class="stack-title">${displayTitle}</p>
                    ${book.author ? `<p class="stack-author">${book.author}</p>` : ""}
                </div>
                <dl class="stack-grid" data-count="${stackItems.length}">
                    ${stackItems.join("")}
                </dl>
            </div>
        </div>
      `;

    el.innerHTML = shelfCover + shelfText + stackHtml;

    // Attach handler for Stack View ISBN trigger
    const stackTrigger = el.querySelector(".stack-content .isbn-trigger");
    if (stackTrigger) {
      // Shared logic
      const triggerAction = (e) => {
        e.stopPropagation();
        e.preventDefault();
        updateIsbnField(stackTrigger.dataset.isbn);
        openIsbnPopup();
      };

      stackTrigger.addEventListener("click", triggerAction);
      stackTrigger.addEventListener("keydown", (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          triggerAction(e);
        }
      });
    }

    el.addEventListener("click", () => setActive(book.id, true));
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

    if (!skipMeasure) {
      measureBookPositions();
    }
    if (typeof lenis !== 'undefined' && lenis) {
      lenis.resize();
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

  const initObserver = () => {
    // Disconnect any existing observer logic handled by variable scope if needed
    // In this specific flow, observer is created fresh.
    const options = {
      root: null, // viewport
      rootMargin: "200px", // Reduced margin to ensure lazy loading is felt
      threshold: 0.1 // Require at least a pixel to be visible
    };

    const observer = new IntersectionObserver((entries) => {
      // If the sentinel is visible, load more
      if (entries.some(entry => entry.isIntersecting)) {
        loadMoreBooks(observer);
      }
    }, options);

    observer.observe(sentinel);
  };

  const applySort = () => {
    // Sort viewableBooks in place based on currentSort
    if (currentSort === "author") {
      viewableBooks.sort((a, b) => {
        const authorA = (a.author || "").toLowerCase();
        const authorB = (b.author || "").toLowerCase();
        return authorA.localeCompare(authorB);
      });
    } else if (currentSort === "title") {
      viewableBooks.sort((a, b) => {
        const titleA = (a.title || "").toLowerCase();
        const titleB = (b.title || "").toLowerCase();
        return titleA.localeCompare(titleB);
      });
    } else if (currentSort === "year") {
      viewableBooks.sort((a, b) => {
        return (b.published || 0) - (a.published || 0);
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
        if (typeof cfState !== 'undefined') {
          cfState.targetIndex = randomIndex;
        }

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

    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") closeDropdown();
    });
  };

  initSortDropdown();

  const renderBooks = () => {
    bookshelf.innerHTML = "";
    bookElements = []; // Reset global
    nextBookIndex = 0; // Reset

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
  };

  const setActive = (id, shouldCenter = false) => {
    // If null passed, we are closing.

    if (activeId === id && !shouldCenter && id !== null) return;
    activeId = id;
    window.activeId = id; // Expose for other scopes

    // Update URL without reloading
    if (id) {
      if (history.pushState) {
        history.pushState(null, null, `#${id}`);
      } else {
        window.location.hash = id;
      }
    } else {
      // Clear Hash
      if (history.pushState) {
        history.pushState("", document.title, window.location.pathname + window.location.search);
      } else {
        window.location.hash = "";
      }
    }

    // Toggle Elements
    if (!id) {
      // Close everything
      if (detailsSection) detailsSection.hidden = true;
      if (detailCard) detailCard.hidden = true;
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

        // Center the book if requested logic (e.g. from click)
        if (shouldCenter) {
          if (document.body.classList.contains('view-stack') || bookshelf.classList.contains("force-stack")) {
            // Vertical Scroll for Stack View (Desktop or Mobile)
            el.scrollIntoView({ behavior: 'smooth', block: 'center' });
          } else {
            // Horizontal Scroll for Shelf View
            const containerWidth = bookshelf.clientWidth;
            const bookLeft = el.offsetLeft;
            const bookWidth = el.offsetWidth;
            const centerPos = bookLeft - (containerWidth / 2) + (bookWidth / 2);

            if (lenis) {
              lenis.scrollTo(centerPos, { smooth: true, duration: 1.2 });
            } else {
              bookshelf.scrollTo({ left: centerPos, behavior: 'smooth' });
            }
          }
        }
      }
    });
  };
  window.setActive = setActive;

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
    }
    const palette = pickPalette(book);
    detailCard.style.setProperty("--details-bg", palette.background);
    detailCard.style.setProperty("--details-fg", palette.foreground);
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
      toggleIsbnPopup();
    });

    document.addEventListener("click", (event) => {
      if (!isbnPopup.classList.contains("is-visible") || !event.target) {
        return;
      }
      if (
        event.target === isbnTrigger ||
        isbnTrigger.contains(event.target) ||
        isbnPopup.contains(event.target)
      ) {
        return;
      }
      closeIsbnPopup();
    });

    document.addEventListener("keydown", (event) => {
      if (
        event.key === "Escape" &&
        isbnPopup.classList.contains("is-visible")
      ) {
        closeIsbnPopup();
        isbnTrigger.focus();
      }
    });

    isbnLinks.forEach((link) => {
      link.addEventListener("click", () => {
        closeIsbnPopup();
      });
    });
  };

  // --- Animation Setup & Lenis Management ---
  let lenis;
  let currentTilt = 0;
  let targetTilt = 0;
  let lastScroll = -1;

  // Shelf Physics State (Turtle -> Cheetah)
  // Shelf Physics State (Turtle -> Cheetah)
  window.shelfState = {
    keyVelocity: 0,
    wsBoost: 1, // Wheel Speed Boost
    lastWheelTime: 0,
    keys: { left: false, right: false }
  };

  // Exponential Constants
  // Time-Based Constants (Pixels Per Second)
  const KEY_BASE_SPEED_PPS = 100; // Base crawl speed
  const KEY_MAX_SPEED_PPS = 4000; // Max speed
  const KEY_ACCEL_PPS = 2400; // Acceleration per second
  const KEY_FRICTION_FACTOR = 8.0; // Damping (v -= v * f * dt)

  const WHEEL_BASE_MULT = 0.3; // Dampen native scroll
  window.WHEEL_MAX_MULT = 1.1; // Negligible boost
  window.PHYS_WHEEL_FACTOR = 0.35; // Coverflow speed

  // Simple linear interpolation
  const lerp = (start, end, factor) => start + (end - start) * factor;

  const manageLenis = () => {
    const isMobile = window.matchMedia("(max-width: 768px)").matches;
    const isStackView = document.body.classList.contains('view-stack');
    const isCoverflow = document.body.classList.contains('view-coverflow');

    // 1. Destroy Lenis if we are NOT in Shelf View (Desktop)
    // i.e. If Mobile, or Stack, or Coverflow
    if (lenis && (isMobile || isStackView || isCoverflow)) {
      lenis.destroy();
      lenis = null;
    }

    // 2. Class Management
    bookshelf.classList.remove("force-stack", "force-shelf");

    if (isMobile) {
      bookshelf.classList.add("force-stack");
      bookshelf.style.scrollBehavior = "smooth";
    } else {
      // Desktop
      if (isStackView) {
        // Native Scroll for Stack, but NO 'force-stack' class (preserves grid)
        bookshelf.style.scrollBehavior = "smooth";
      } else if (!isCoverflow) {
        // Shelf View
        bookshelf.classList.add("force-shelf");
        if (typeof Lenis !== "undefined" && bookshelf && !lenis) {
          lenis = new Lenis({
            wrapper: bookshelf,
            content: bookshelf,
            orientation: "horizontal",
            gestureOrientation: "both",
            smoothWheel: true,
            syncTouch: true,
            wheelMultiplier: 1,
            touchMultiplier: 2,
          });
        }
      }
    }
  };

  const initResponsiveLayout = () => {
    // Watch for View Changes (Body Class)
    const bodyObserver = new MutationObserver(() => {
      manageLenis();
    });
    bodyObserver.observe(document.body, { attributes: true, attributeFilter: ['class'] });

    // Initial check
    manageLenis();

    // Listen for resize to switch modes if crossing breakpoint
    let resizeTimer;
    window.addEventListener('resize', () => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        manageLenis(); // Re-evaluate layout
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



  const updateAnimation = (dtTime) => {
    // Delta Time Calculation
    const time = dtTime || performance.now();

    if (!shelfState.lastFrameTime) shelfState.lastFrameTime = time;

    // Safety clamp: if tab was backgrounded, dt can be huge. Cap at 0.1s.
    let dt = (time - shelfState.lastFrameTime) / 1000;
    if (dt > 0.1 || dt < 0) dt = 0.016;

    shelfState.lastFrameTime = time;

    // 1. Handle Shelf Key Momentum (Manual Logic)
    if (!bookshelf.classList.contains('force-stack') && !document.body.classList.contains('view-coverflow')) {
      let active = false;


      if (shelfState.keys.left) {
        if (shelfState.keyVelocity > -KEY_BASE_SPEED_PPS) {
          shelfState.keyVelocity = -KEY_BASE_SPEED_PPS;
        } else {
          shelfState.keyVelocity -= KEY_ACCEL_PPS * dt;
        }
        if (shelfState.keyVelocity < -KEY_MAX_SPEED_PPS) shelfState.keyVelocity = -KEY_MAX_SPEED_PPS;
        active = true;
      } else if (shelfState.keys.right) {
        if (shelfState.keyVelocity < KEY_BASE_SPEED_PPS) {
          shelfState.keyVelocity = KEY_BASE_SPEED_PPS;
        } else {
          shelfState.keyVelocity += KEY_ACCEL_PPS * dt;
        }
        if (shelfState.keyVelocity > KEY_MAX_SPEED_PPS) shelfState.keyVelocity = KEY_MAX_SPEED_PPS;
        active = true;
      } else {
        const damp = shelfState.keyVelocity * KEY_FRICTION_FACTOR * dt;
        shelfState.keyVelocity -= damp;
        if (Math.abs(shelfState.keyVelocity) < 5) shelfState.keyVelocity = 0;
      }

      if (Math.abs(shelfState.keyVelocity) > 0) {
        const moveAmount = shelfState.keyVelocity * dt;
        const current = lenis ? lenis.scroll : bookshelf.scrollLeft;
        const target = current + moveAmount;

        if (lenis) {
          lenis.scrollTo(target, { immediate: true });
        } else {
          bookshelf.scrollLeft = target;
        }
      }
    }

    if (lenis) {
      // Dynamic Wheel Boost Application
      // Decay excess boost smoothly back to 1
      if (shelfState.wsBoost > 1) {
        shelfState.wsBoost = 1 + (shelfState.wsBoost - 1) * 0.96;
        if (shelfState.wsBoost < 1.001) shelfState.wsBoost = 1;
      }


      // Attempt to apply multiplier to Lenis instance
      // Lenis v1 structure adaptation
      if (lenis.options) lenis.options.wheelMultiplier = WHEEL_BASE_MULT * shelfState.wsBoost;

      lenis.raf(time);
    }

    // Get reliable scroll position
    const currentScroll = lenis ? lenis.scroll : bookshelf.scrollLeft;

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

    // Time-Based Interpolation (Damping) for consistent 'weight'
    // decay of 5.0 gives a heavy, smooth feel roughly equivalent to 0.08@60hz but consistent
    const smoothFactor = 1 - Math.exp(-5.0 * dt);
    currentTilt = lerp(currentTilt, targetTilt, smoothFactor);

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

    requestAnimationFrame(updateAnimation);
  };

  const handleDeepLink = () => {
    const hash = window.location.hash.slice(1); // Remove '#'
    if (!hash) return;

    // Decoding might be needed for some IDs
    const rawHash = decodeURIComponent(hash);

    // 1. Try Exact ID Match first
    let targetIndex = viewableBooks.findIndex(b => b.id === rawHash);

    // 2. Fallback: Fuzzy Title/Author Match
    // If no ID found, treat hash as a search query (e.g. "great-gatsby" -> "great gatsby")
    // We check if title CONTAINS the dash-less query
    if (targetIndex === -1 && rawHash.length > 2) {
      const query = normalizeText(rawHash.replace(/-/g, " ")); // "great-gatsby" -> "great gatsby"
      targetIndex = viewableBooks.findIndex(b => {
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
    const targetId = viewableBooks[targetIndex].id;

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
        setActive(targetId);

        // Scroll to center
        const container = bookshelf;
        const element = targetElement.el;

        // Handle scrolling based on layout mode
        if (bookshelf.classList.contains("force-stack")) {
          element.scrollIntoView({ behavior: 'smooth', block: 'center' });
        } else {
          if (lenis) {
            // Lenis horizontal scroll
            const containerWidth = container.clientWidth;
            const targetLeft = element.offsetLeft;
            const targetWidth = element.offsetWidth;
            const centerPos = targetLeft - (containerWidth / 2) + (targetWidth / 2);
            lenis.scrollTo(centerPos, { duration: 1.5 });
          } else {
            // Native horizontal scroll fallback
            const containerWidth = container.clientWidth;
            const targetLeft = element.offsetLeft;
            const targetWidth = element.offsetWidth;
            container.scrollTo({
              left: targetLeft - (containerWidth / 2) + (targetWidth / 2),
              behavior: 'smooth'
            });
          }
        }
      }
    });
  };

  // Listen for hash changes (e.g. user back button)
  window.addEventListener('hashchange', handleDeepLink);

  // --- Initialization ---
  initResponsiveLayout();
  initSearch();

  // Close Button Logic
  const closeDetailsBtn = document.querySelector('.details-close');
  if (closeDetailsBtn) {
    closeDetailsBtn.addEventListener('click', () => {
      setActive(null);
    });
  }

  // Global ESC to key to Close
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && activeId) {
      setActive(null);
    }
  });

  applySort();
  renderBooks();

  const defaultSortOption = sortOptions ? Array.from(sortOptions).find(opt => opt.dataset.value === currentSort) : null;
  if (defaultSortOption) defaultSortOption.setAttribute('aria-selected', 'true');


  // Check for deep link on startup
  // We wrap in timeout to ensure initial render/layout is complete
  setTimeout(handleDeepLink, 300);

  setupObservers();
  initIsbnPopup();
  requestAnimationFrame(updateAnimation);
})();

/* --- View Switching Logic --- */
(function () {
  let currentView = 'shelf';
  let coverflowInitialized = false;
  let coverflowIndex = 0;
  let cfBg = null;
  let cfTitle = null;
  let cfDetails = null;
  let cfDetailsTimer = null;
  let cfSliderContainer = null;
  let cfSlider = null;
  let cfBookCache = []; // Optimization cache
  const CF_IDLE_TIME = 500; // 0.5s

  // Config
  const C_SPACING = 50;
  const C_OFFSET = 200;
  const C_ROTATION = 60;
  const C_Z_DEPTH = -200;

  // Elements
  const body = document.body;
  const bookshelf = document.getElementById('bookshelf');

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
        switchView('shelf', true);
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

    // Attach handler for Stack View ISBN trigger

    const stackTrigger = document.querySelector(".stack-content .isbn-trigger");
    if (stackTrigger) {
      // Shared logic
      const triggerAction = (e) => {
        e.stopPropagation();
        e.preventDefault();
        updateIsbnField(stackTrigger.dataset.isbn);
        openIsbnPopup();
      };

      stackTrigger.addEventListener("click", triggerAction);
      stackTrigger.addEventListener("keydown", (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          triggerAction(e);
        }
      });
    }



    initBackToTop();
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
      window.scrollTo({ top: 0, behavior: 'smooth' });
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
      void shelfContainer.offsetWidth;
      shelfContainer.classList.add('view-animate');
    }

    // Toggle Instruction Text
    const instruction = document.getElementById('instruction-text');
    if (instruction) {
      instruction.style.display = (view === 'shelf') ? 'block' : 'none';
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
      if (typeof lenis !== 'undefined' && lenis) {
        lenis.resize();
      }

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

              if (typeof lenis !== 'undefined' && lenis) {
                lenis.scrollTo(centerPos, { immediate: true });
              } else {
                container.scrollTo({ left: centerPos, behavior: 'auto' });
              }
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
  const PHYS_KEY_ACCEL = 0.002;

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
        const currentLoaded = document.querySelectorAll('.book').length;
        if (val > currentLoaded - 5) { // buffer
          // We need to load more
          while (document.querySelectorAll('.book').length <= val + 5) {
            // Check if we reached the absolute end
            if (document.querySelectorAll('.book').length >= (window.viewableBookCount || 0)) break;

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
        cfDetailsTimer = setTimeout(() => {
          showCoverflowDetails();
        }, CF_IDLE_TIME);
      });

      coverflowInitialized = true;
    }

    const books = Array.from(document.querySelectorAll('.book'));
    cfBookCache = books; // Update cache

    if (cfState.index >= books.length) cfState.index = 0;

    cfState.velocity = 0;
    cfState.keys.left = false;
    cfState.keys.right = false;
  }

  function startCoverflowLoop() {
    if (cfState.isActive) return;
    cfState.isActive = true;

    if (cfSliderContainer) cfSliderContainer.classList.add('visible');
    // Ensure accurate max on start
    if (cfSlider) {
      cfSlider.max = Math.max(0, (window.viewableBookCount || document.querySelectorAll('.book').length) - 1);
    }

    const loop = () => {
      if (!cfState.isActive) return;

      // 0. Auto-Scroll Logic
      if (cfState.targetIndex !== null) {
        const diff = cfState.targetIndex - cfState.index;
        // Snap when very close (reduced from 0.05 to 0.005 to prevent jump)
        if (Math.abs(diff) < 0.005) {
          cfState.index = cfState.targetIndex;
          cfState.velocity = 0;
          cfState.targetIndex = null;
        } else {
          // Proportional control for smooth seek
          // Use a higher max velocity for seeking if needed, or stick to PHYS_MAX_VEL
          const pGain = 0.06;
          const desired = diff * pGain;
          cfState.velocity = Math.max(Math.min(desired, PHYS_MAX_VEL), -PHYS_MAX_VEL);
        }
      } else {
        // Normal Friction
        cfState.velocity *= PHYS_FRICTION;
      }

      // 1. Input Acceleration (Continuous for held keys)
      if (cfState.keys.left) {
        cfState.velocity -= PHYS_KEY_ACCEL;
        cfState.targetIndex = null;
      }
      if (cfState.keys.right) {
        cfState.velocity += PHYS_KEY_ACCEL;
        cfState.targetIndex = null;
      }

      // 2. Friction (Moved to else block of auto-scroll)
      // cfState.velocity *= PHYS_FRICTION;

      // 3. Update Position
      const clampedVel = Math.max(Math.min(cfState.velocity, PHYS_MAX_VEL), -PHYS_MAX_VEL);
      cfState.index += clampedVel;

      // 4. Boundaries (Bounce logic or hard clamp?) -> Hard clamp + zero vel for now
      const maxIdx = document.querySelectorAll('.book').length - 1;
      if (cfState.index < 0) {
        cfState.index = 0;
        cfState.velocity = 0;
      }
      if (cfState.index > maxIdx) {
        cfState.index = maxIdx;
        cfState.velocity = 0;
      }

      // 5. Snapping (when slow and not pressing keys)
      const isKeyHeld = cfState.keys.left || cfState.keys.right;
      if (!isKeyHeld && Math.abs(cfState.velocity) < 0.01) {
        const target = Math.round(cfState.index);
        const diff = target - cfState.index;
        if (Math.abs(diff) > 0.001) cfState.index += diff * PHYS_SNAP_STRENGTH;
        else cfState.index = target;
      }

      // Idle Timer Logic
      // If moving, clear timer and hide details
      if (Math.abs(cfState.velocity) > 0.001 || isKeyHeld) {
        clearTimeout(cfDetailsTimer);
        cfDetailsTimer = null;
        if (cfDetails && cfDetails.classList.contains('visible')) {
          cfDetails.classList.remove('visible');
        }
      } else {
        // If stopped and no timer, start one
        if (!cfDetailsTimer && cfDetails && !cfDetails.classList.contains('visible')) {
          cfDetailsTimer = setTimeout(() => {
            // Double check we are still stopped
            if (Math.abs(cfState.velocity) < 0.001) {
              showCoverflowDetails();
            }
          }, CF_IDLE_TIME);
        }
      }

      // 6. Output to valid index for legacy checks
      coverflowIndex = Math.round(cfState.index);

      updateCoverflow();
      cfReqId = requestAnimationFrame(loop);
    };
    cfReqId = requestAnimationFrame(loop);
  }

  function showCoverflowDetails() {
    if (!cfDetails) return;
    const books = Array.from(document.querySelectorAll('.book'));
    const activeBook = books[coverflowIndex];
    if (!activeBook) return;

    const data = activeBook.bookData || (window.booksData && window.booksData.find(b => b.id == activeBook.dataset.id));
    if (!data) return;

    const translatorHTML = data.translator ? `<span class="cf-trans">(${data.translator})</span>` : '';

    const metaItems = [];
    if (data.published) metaItems.push(`<span>${data.published}</span>`);
    if (data.publisher) metaItems.push(`<span>${data.publisher}</span>`);
    if (data.binding) metaItems.push(`<span>${data.binding}</span>`);
    if (data.isbn) metaItems.push(`<span>${data.isbn}</span>`);

    cfDetails.innerHTML = `
        <div class="cf-author">${data.author || ''} ${translatorHTML}</div>
        <div class="cf-meta-row">
            ${metaItems.join('')}
        </div>
      `;
    cfDetails.classList.add('visible');
  }

  function stopCoverflowLoop() {
    cfState.isActive = false;
    cancelAnimationFrame(cfReqId);
    clearTimeout(cfDetailsTimer);
    if (cfDetails) cfDetails.classList.remove('visible');
    if (cfSliderContainer) cfSliderContainer.classList.remove('visible');
  }

  // Refactored Update Function (Handles Float)
  function updateCoverflow() {
    // Optimization: Refresh cache if mismatched (lazy load happened)
    // Fast check: current simple cache vs global count
    if (cfBookCache.length !== (window.bookElements?.length || 0)) {
      cfBookCache = Array.from(document.querySelectorAll('.book'));
    }
    const books = cfBookCache;

    if (!books.length) return;

    // LAZY LOAD TRIGGER
    if (books.length - cfState.index < 30) {
      if (window.loadMoreBooks) window.loadMoreBooks(true);
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
          if (i !== intIdx) {
            cfState.targetIndex = i; // Trigger auto-scroll
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

  // Input Handlers
  window.addEventListener('keydown', (e) => {
    if (currentView === 'coverflow') {
      if (e.key === 'ArrowLeft') { cfState.keys.left = true; cfState.targetIndex = null; }
      if (e.key === 'ArrowRight') { cfState.keys.right = true; cfState.targetIndex = null; }
    } else if (currentView === 'shelf') {
      if (e.key === 'ArrowLeft') shelfState.keys.left = true;
      if (e.key === 'ArrowRight') shelfState.keys.right = true;
    }
  });

  window.addEventListener('keyup', (e) => {
    if (currentView === 'coverflow') {
      if (e.key === 'ArrowLeft') cfState.keys.left = false;
      if (e.key === 'ArrowRight') cfState.keys.right = false;
    } else if (currentView === 'shelf') {
      if (e.key === 'ArrowLeft') shelfState.keys.left = false;
      if (e.key === 'ArrowRight') shelfState.keys.right = false;
    }
  });

  window.addEventListener('wheel', (e) => {
    if (currentView === 'coverflow') {
      e.preventDefault();
      cfState.targetIndex = null;
      const delta = e.deltaY || e.deltaX;
      cfState.velocity += delta * PHYS_WHEEL_FACTOR;
    } else if (currentView === 'shelf') {
      // Shelf "Cheetah" Wheel Logic
      // Detect frequency
      const now = performance.now();
      const dt = now - shelfState.lastWheelTime;
      shelfState.lastWheelTime = now;

      // If scrolling fast (less than 120ms between events), boost up
      if (dt < 120) {
        shelfState.wsBoost = Math.min(shelfState.wsBoost + 0.005, WHEEL_MAX_MULT);
      } else if (dt > 500) {
        // Reset if paused
        shelfState.wsBoost = 1;
      }
      // Lenis will read shelfState.wsBoost in the loop
    }
  }, { passive: false });

  // Run Init
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initView);
  } else {
    initView();
  }

})();
