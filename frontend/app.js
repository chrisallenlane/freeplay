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
		renderFilters();
		renderGrid();
	}

	function el(tag, cls, text) {
		const e = document.createElement(tag);
		if (cls) e.className = cls;
		if (text) e.textContent = text;
		return e;
	}

	function addFilterBtn(label, isActive, onClick) {
		const btn = el(
			"button",
			`btn filter-btn${isActive ? " active" : ""}`,
			label,
		);
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
		const displayName = FP.stripExt(game.filename);

		const card = el("a", "game-card");
		card.href = FP.playUrl(game);
		card.dataset.key = key;

		// Favorite button
		const isFav = favorites.has(key);
		const fav = el(
			"button",
			`fav-btn${isFav ? " favorited" : ""}`,
			isFav ? "\u2605" : "\u2606",
		);
		fav.addEventListener("click", (e) => {
			e.preventDefault();
			e.stopPropagation();
			const wasSet = favorites.has(key);
			if (wasSet) favorites.delete(key);
			else favorites.add(key);
			fav.textContent = wasSet ? "\u2606" : "\u2605";
			fav.classList.toggle("favorited", !wasSet);
			saveFavorites();
			if (activeFavorites) renderGrid();
		});
		// Cover art or placeholder
		let coverEl;
		if (game.hasCover) {
			coverEl = el("img", "cover");
			coverEl.src = FP.coverUrl(game);
			coverEl.alt = displayName;
			coverEl.loading = "lazy";
			coverEl.width = 180;
			coverEl.height = 240;
		} else {
			coverEl = el("div", "placeholder-cover");
			coverEl.append(
				el("div", "placeholder-name", displayName),
				el("div", "placeholder-console", game.console),
			);
		}

		// Card info
		const info = el("div", "card-info");
		info.append(
			el("div", "card-title", displayName),
			el("div", "card-console", game.console),
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
			const msg = el("div", "message");
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
				const msg = el(
					"div",
					"message",
					"Could not load game library. Check that Freeplay is running.",
				);
				const retry = el("button", null, "Retry");
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
		gridColumns,
		findCardIndex,
	} = FP;

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
		card.scrollIntoView({ behavior: "smooth", block: "nearest" });
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
	}

	function pollCoverStatus() {
		fetch("/api/status")
			.then((res) => res.json())
			.then((data) => {
				if (data.fetchingCovers) {
					rescanBtn.disabled = true;
					rescanBtn.innerHTML =
						'<span class="spinner">\u21BB</span> Fetching covers\u2026';
					rescanBtn.classList.add("fetching");
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
		fetch("/api/rescan", { method: "POST" })
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

	// ---------------------------------------------------------------------------
	// Gamepad support
	// ---------------------------------------------------------------------------

	// Number of gamepads currently connected.
	let gamepadCount = 0;

	// ID of the running requestAnimationFrame loop, or null when stopped.
	let rafId = null;

	// Debounce state: which logical action is being held and when it last fired.
	let heldAction = null;
	let heldLastFired = 0;

	// Debounce intervals (ms).
	const REPEAT_DELAY = 180;

	/**
	 * The main poll loop. Runs every animation frame while a gamepad is connected.
	 * @param {DOMHighResTimeStamp} now
	 */
	function pollGamepads(now) {
		if (gamepadCount === 0) {
			rafId = null;
			return;
		}

		const gamepads = navigator.getGamepads();
		let action = null;

		for (const gp of gamepads) {
			if (!gp) continue;
			const candidate = FP.readGamepadAction(gp);
			if (candidate) {
				action = candidate;
				break;
			}
		}

		if (action === null) {
			// No input — reset debounce state.
			heldAction = null;
			heldLastFired = 0;
		} else if (action !== heldAction) {
			// New action started — fire immediately.
			heldAction = action;
			heldLastFired = now;
			handleAction(action);
		} else {
			// Continuing to hold the same action — repeat after REPEAT_DELAY.
			if (now - heldLastFired >= REPEAT_DELAY) {
				heldLastFired = now;
				handleAction(action);
			}
		}

		rafId = requestAnimationFrame(pollGamepads);
	}

	window.addEventListener("gamepadconnected", () => {
		gamepadCount++;
		if (rafId === null) {
			rafId = requestAnimationFrame(pollGamepads);
		}
	});

	window.addEventListener("gamepaddisconnected", () => {
		gamepadCount = Math.max(0, gamepadCount - 1);
		// The loop will stop itself on the next frame when gamepadCount reaches 0.
	});
})();
