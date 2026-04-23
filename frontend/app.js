(() => {
	const FP = window.Freeplay;

	let allGames = [];
	let consoles = [];
	let activeConsole = null;
	let activeFavorites = false;
	const favorites = new Set(
		JSON.parse(localStorage.getItem("freeplay-favorites") || "[]"),
	);

	const grid = document.getElementById("game-grid");
	const filtersBar = document.getElementById("filters");
	const searchInput = document.getElementById("search");
	const rescanBtn = document.getElementById("rescan-btn");
	const rescanStatus = document.getElementById("rescan-status");

	// Key of the currently focused game card, used to restore focus after re-renders.
	let focusedKey = null;

	// Key of the last mouse-hovered game card, used as the starting position
	// for directional navigation when no card has focus.
	let hoveredKey = null;

	function saveFavorites() {
		localStorage.setItem(
			"freeplay-favorites",
			JSON.stringify(Array.from(favorites)),
		);
	}

	function getFilteredGames() {
		return FP.filterGames(allGames, {
			favorites,
			favoritesOnly: activeFavorites,
			console: activeConsole,
			query: searchInput.value,
		});
	}

	function blurActiveCard() {
		if (document.activeElement?.classList.contains("game-card")) {
			document.activeElement.blur();
		}
	}

	function renderAll() {
		focusedKey = null;
		hoveredKey = null;
		blurActiveCard();

		// Preserve keyboard focus across the filter rebuild: without this,
		// focus falls to <body> and the user must Tab back from the top.
		const activeFilterLabel = document.activeElement?.classList.contains(
			"filter-btn",
		)
			? document.activeElement.textContent
			: null;

		renderFilters();
		renderGrid();

		if (activeFilterLabel) {
			for (const btn of filtersBar.querySelectorAll(".filter-btn")) {
				if (btn.textContent === activeFilterLabel) {
					btn.focus();
					break;
				}
			}
		}
	}

	function addFilterBtn(label, isActive, onClick) {
		const btn = FP.el(
			"button",
			`btn filter-btn${isActive ? " active" : ""}`,
			label,
		);
		btn.setAttribute("aria-pressed", isActive ? "true" : "false");
		btn.addEventListener("click", onClick);
		filtersBar.appendChild(btn);
	}

	function renderFilters() {
		filtersBar.innerHTML = "";

		addFilterBtn("\u2605 Favorites", activeFavorites, () => {
			activeFavorites = !activeFavorites;
			if (activeFavorites) activeConsole = null;
			renderAll();
		});

		addFilterBtn("All", !activeConsole && !activeFavorites, () => {
			activeConsole = null;
			activeFavorites = false;
			renderAll();
		});

		consoles.forEach((name) => {
			addFilterBtn(name, activeConsole === name, () => {
				activeConsole = name;
				activeFavorites = false;
				renderAll();
			});
		});
	}

	function renderCard(game) {
		const key = FP.favKey(game);
		const displayName = game.igdbName || FP.stripExt(game.filename);

		const card = FP.el("a", "game-card");
		card.href = FP.detailsUrl(game);
		card.dataset.key = key;

		// Favorite button
		const isFav = favorites.has(key);
		const fav = FP.el(
			"button",
			`fav-btn${isFav ? " favorited" : ""}`,
			isFav ? "\u2605" : "\u2606",
		);
		fav.setAttribute(
			"aria-label",
			isFav
				? `Remove ${displayName} from favorites`
				: `Add ${displayName} to favorites`,
		);
		fav.addEventListener("click", (e) => {
			e.preventDefault();
			e.stopPropagation();
			const wasSet = favorites.has(key);
			if (wasSet) favorites.delete(key);
			else favorites.add(key);
			fav.textContent = wasSet ? "\u2606" : "\u2605";
			fav.setAttribute(
				"aria-label",
				wasSet
					? `Add ${displayName} to favorites`
					: `Remove ${displayName} from favorites`,
			);
			fav.classList.toggle("favorited", !wasSet);
			saveFavorites();
			if (activeFavorites) renderGrid();
		});
		// Cover art or placeholder
		let coverEl;
		if (game.hasCover) {
			coverEl = FP.el("img", "cover");
			coverEl.src = FP.coverUrl(game);
			coverEl.alt = displayName;
			coverEl.loading = "lazy";
			coverEl.width = 180;
			coverEl.height = 240;
		} else {
			coverEl = FP.el("div", "placeholder-cover");
			coverEl.append(
				FP.el("div", "placeholder-name", displayName),
				FP.el("div", "placeholder-console", game.console),
			);
		}

		// Card info
		const info = FP.el("div", "card-info");
		info.append(
			FP.el("div", "card-title", displayName),
			FP.el("div", "card-console", game.console),
		);

		card.append(fav, coverEl, info);
		grid.appendChild(card);
	}

	function renderGrid() {
		// Capture focused key before destroying the DOM.
		const activeEl = document.activeElement;
		if (activeEl?.classList.contains("game-card")) {
			focusedKey = activeEl.dataset.key ?? null;
		}

		grid.innerHTML = "";
		const games = getFilteredGames();

		if (games.length === 0) {
			const msg = FP.el("div", "message");
			if (allGames.length === 0) {
				msg.textContent =
					"No games found. Add ROMs to your library and check your freeplay.toml configuration.";
			} else if (activeFavorites) {
				msg.textContent =
					"No favorites yet. Click the star on a game to add it.";
			} else {
				msg.textContent = "No games match your search.";
			}
			grid.appendChild(msg);
			return;
		}

		games.forEach(renderCard);

		// Restore focus to the previously focused card, or the first card if
		// the key is no longer present (e.g. after a filter change).
		// Skip restoration when the search input has focus to avoid stealing
		// focus while the user is typing.
		if (focusedKey !== null && document.activeElement !== searchInput) {
			const target =
				grid.querySelector(`[data-key="${CSS.escape(focusedKey)}"]`) ??
				grid.querySelector(".game-card");
			target?.focus();
		}
	}

	function loadCatalog() {
		return fetch("/api/games")
			.then((res) => {
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return res.json();
			})
			.then((catalog) => {
				allGames = catalog.games || [];
				consoles = catalog.consoles || [];
				renderAll();
			})
			.catch(() => {
				grid.innerHTML = "";
				const msg = FP.el(
					"div",
					"message",
					"Could not load game library. Check that Freeplay is running.",
				);
				const retry = FP.el("button", null, "Retry");
				retry.addEventListener("click", loadCatalog);
				msg.appendChild(retry);
				grid.appendChild(msg);
			});
	}

	searchInput.addEventListener("input", renderGrid);

	// ---------------------------------------------------------------------------
	// Directional navigation (shared by keyboard and gamepad)
	// ---------------------------------------------------------------------------

	const {
		ACTION_LEFT,
		ACTION_RIGHT,
		ACTION_UP,
		ACTION_DOWN,
		ACTION_ACTIVATE,
		ACTION_PREV_FILTER,
		ACTION_NEXT_FILTER,
	} = FP;

	function gridColumns(cards) {
		if (cards.length === 0) return 1;
		const firstTop = cards[0].offsetTop;
		let cols = 0;
		for (const card of cards) {
			if (card.offsetTop !== firstTop) break;
			cols++;
		}
		return cols;
	}

	function findCardIndex(cards, predicate) {
		for (let i = 0; i < cards.length; i++) {
			if (predicate(cards[i])) return i;
		}
		return -1;
	}

	/**
	 * Sets the `.highlighted` class on the given card, removing it from any
	 * previously highlighted card.
	 * @param {Element|null} card
	 */
	function highlightCard(card) {
		const prev = grid.querySelector(".game-card.highlighted");
		if (prev) prev.classList.remove("highlighted");
		if (card) card.classList.add("highlighted");
	}

	/**
	 * Moves focus to the card at the given index, clamped to valid range.
	 * @param {NodeList} cards
	 * @param {number} index
	 */
	function focusCard(cards, index) {
		if (cards.length === 0) return;
		const clamped = Math.max(0, Math.min(index, cards.length - 1));
		const card = cards[clamped];
		card.focus({ preventScroll: true });
		card.scrollIntoView({
			behavior: FP.prefersReducedMotion() ? "instant" : "smooth",
			block: "nearest",
		});
		highlightCard(card);
		focusedKey = card.dataset.key ?? null;
	}

	/**
	 * Handles a single logical directional action.
	 * @param {string} action
	 */
	function handleAction(action) {
		const cards = grid.querySelectorAll(".game-card");

		switch (action) {
			case ACTION_ACTIVATE: {
				const card = grid.querySelector(".game-card.highlighted");
				if (card) card.click();
				return;
			}

			case ACTION_PREV_FILTER:
			case ACTION_NEXT_FILTER: {
				const btns = filtersBar.querySelectorAll(".filter-btn");
				if (btns.length === 0) return;
				const activeBtn = filtersBar.querySelector(".filter-btn.active");
				const sibling =
					action === ACTION_PREV_FILTER
						? activeBtn?.previousElementSibling
						: activeBtn?.nextElementSibling;
				if (!sibling) return;
				sibling.click();
				return;
			}

			default: {
				if (cards.length === 0) return;

				let current = findCardIndex(cards, (c) => c === document.activeElement);
				// Fall back to the last mouse-hovered card when no card has focus.
				if (current < 0 && hoveredKey !== null) {
					current = findCardIndex(cards, (c) => c.dataset.key === hoveredKey);
				}
				if (current < 0) {
					focusCard(cards, 0);
					return;
				}

				const cols = gridColumns(cards);

				switch (action) {
					case ACTION_LEFT:
						// Clamp: do nothing if already at start of row.
						if (current % cols !== 0) focusCard(cards, current - 1);
						break;
					case ACTION_RIGHT:
						// Clamp: do nothing if already at end of row.
						if ((current + 1) % cols !== 0 && current + 1 < cards.length) {
							focusCard(cards, current + 1);
						}
						break;
					case ACTION_UP:
						if (current >= cols) focusCard(cards, current - cols);
						break;
					case ACTION_DOWN:
						if (current + cols < cards.length) focusCard(cards, current + cols);
						break;
				}
			}
		}
	}

	// Keyboard shortcuts: [/] cycle filters, arrow keys navigate cards.
	document.addEventListener("keydown", (e) => {
		if (document.activeElement === searchInput) return;
		const actionMap = {
			"[": ACTION_PREV_FILTER,
			"]": ACTION_NEXT_FILTER,
			ArrowLeft: ACTION_LEFT,
			ArrowRight: ACTION_RIGHT,
			ArrowUp: ACTION_UP,
			ArrowDown: ACTION_DOWN,
		};
		const action = actionMap[e.key];
		if (!action) return;
		e.preventDefault();
		handleAction(action);
	});

	// Track and highlight mouse-hovered card.
	grid.addEventListener("mouseover", (e) => {
		const card = e.target.closest(".game-card");
		if (!card) return;
		hoveredKey = card.dataset.key ?? null;
		highlightCard(card);
	});

	// Highlight cards reached via Tab navigation.
	grid.addEventListener("focusin", (e) => {
		const card = e.target.closest(".game-card");
		if (card) highlightCard(card);
	});

	// Clear directional focus when the mouse takes over.
	grid.addEventListener("mousemove", blurActiveCard);

	// Rescan button
	let statusPollTimer = null;

	function resetRescanBtn() {
		rescanBtn.disabled = false;
		rescanBtn.textContent = "Rescan \u21BB";
		rescanBtn.classList.remove("fetching");
		rescanStatus.textContent = "";
	}

	function pollCoverStatus() {
		fetch("/api/status")
			.then((res) => res.json())
			.then((data) => {
				if (data.fetchingDetails) {
					rescanBtn.disabled = true;
					rescanBtn.innerHTML =
						'<span class="spinner">\u21BB</span> Fetching game data\u2026';
					rescanBtn.classList.add("fetching");
					rescanStatus.textContent = "Fetching game data\u2026";
					statusPollTimer = setTimeout(pollCoverStatus, 2000);
				} else {
					resetRescanBtn();
					loadCatalog();
				}
			})
			.catch(resetRescanBtn);
	}

	rescanBtn.addEventListener("click", () => {
		rescanBtn.disabled = true;
		rescanBtn.textContent = "Scanning\u2026";
		rescanStatus.textContent = "Scanning\u2026";
		fetch("/api/rescan", {
			method: "POST",
			headers: { "X-Requested-With": "freeplay" },
		})
			.then((res) => {
				if (res.status === 409) {
					alert("Scan already in progress.");
					return;
				}
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return loadCatalog().then(pollCoverStatus);
			})
			.catch(() => {
				alert("Rescan failed. Check that Freeplay is running.");
			})
			.finally(() => {
				if (!statusPollTimer) {
					resetRescanBtn();
				}
			});
	});

	FP.initThemeToggle();
	loadCatalog();
	FP.gamepadLoop(handleAction);
})();
