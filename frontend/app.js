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

	function saveFavorites() {
		localStorage.setItem(
			"freeplay-favorites",
			JSON.stringify(Array.from(favorites)),
		);
	}

	function getFilteredGames() {
		return FP.filterGames(allGames, {
			favorites: favorites,
			favoritesOnly: activeFavorites,
			console: activeConsole,
			query: searchInput.value,
		});
	}

	function renderAll() {
		renderFilters();
		renderGrid();
	}

	function addFilterBtn(label, isActive, onClick) {
		const btn = document.createElement("button");
		btn.className = `filter-btn${isActive ? " active" : ""}`;
		btn.textContent = label;
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

	function renderGrid() {
		// Capture focused key before destroying the DOM.
		const activeEl = document.activeElement;
		if (activeEl?.classList.contains("game-card")) {
			focusedKey = activeEl.dataset.key ?? null;
		}

		grid.innerHTML = "";
		const games = getFilteredGames();

		if (games.length === 0) {
			const msg = document.createElement("div");
			msg.className = "message";
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

		games.forEach((game) => {
			const key = FP.favKey(game);
			const displayName = FP.stripExt(game.filename);

			const card = document.createElement("a");
			card.className = "game-card";
			card.href = FP.playUrl(game);
			card.dataset.key = key;

			// Favorite button
			const isFav = favorites.has(key);
			const fav = document.createElement("button");
			fav.className = `fav-btn${isFav ? " favorited" : ""}`;
			fav.textContent = isFav ? "\u2605" : "\u2606";
			fav.addEventListener("click", (e) => {
				e.preventDefault();
				e.stopPropagation();
				if (favorites.has(key)) {
					favorites.delete(key);
					fav.textContent = "\u2606";
					fav.classList.remove("favorited");
				} else {
					favorites.add(key);
					fav.textContent = "\u2605";
					fav.classList.add("favorited");
				}
				saveFavorites();
				if (activeFavorites) renderGrid();
			});
			card.appendChild(fav);

			// Cover art or placeholder
			if (game.hasCover) {
				const img = document.createElement("img");
				img.className = "cover";
				img.src = FP.coverUrl(game);
				img.alt = displayName;
				img.loading = "lazy";
				card.appendChild(img);
			} else {
				const ph = document.createElement("div");
				ph.className = "placeholder-cover";
				const phName = document.createElement("div");
				phName.className = "placeholder-name";
				phName.textContent = displayName;
				const phConsole = document.createElement("div");
				phConsole.className = "placeholder-console";
				phConsole.textContent = game.console;
				ph.appendChild(phName);
				ph.appendChild(phConsole);
				card.appendChild(ph);
			}

			// Card info
			const info = document.createElement("div");
			info.className = "card-info";
			const title = document.createElement("div");
			title.className = "card-title";
			title.textContent = displayName;
			const consoleName = document.createElement("div");
			consoleName.className = "card-console";
			consoleName.textContent = game.console;
			info.appendChild(title);
			info.appendChild(consoleName);
			card.appendChild(info);

			grid.appendChild(card);
		});

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
				const msg = document.createElement("div");
				msg.className = "message";
				msg.textContent =
					"Could not load game library. Check that Freeplay is running.";
				const retry = document.createElement("button");
				retry.textContent = "Retry";
				retry.addEventListener("click", loadCatalog);
				msg.appendChild(retry);
				grid.appendChild(msg);
			});
	}

	searchInput.addEventListener("input", renderGrid);

	// Input mode: suppress hover effects during gamepad navigation,
	// and clear gamepad focus when the mouse takes over.
	grid.addEventListener("mousemove", () => {
		if (grid.dataset.input === "gamepad") {
			grid.dataset.input = "pointer";
			if (document.activeElement?.classList.contains("game-card")) {
				document.activeElement.blur();
			}
		}
	});

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

	// Logical actions produced by the gamepad.
	const ACTION_LEFT = "left";
	const ACTION_RIGHT = "right";
	const ACTION_UP = "up";
	const ACTION_DOWN = "down";
	const ACTION_ACTIVATE = "activate";
	const ACTION_PREV_FILTER = "prevFilter";
	const ACTION_NEXT_FILTER = "nextFilter";

	// Axis threshold for treating an analog stick / D-pad axis as pressed.
	const AXIS_THRESHOLD = 0.5;

	/**
	 * Returns the number of columns in the game grid by counting cards that
	 * share the same offsetTop as the first card.
	 * @param {NodeList} cards
	 * @returns {number}
	 */
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

	/**
	 * Returns the index of the focused game card, or -1 if none is focused.
	 * @param {NodeList} cards
	 * @returns {number}
	 */
	function focusedCardIndex(cards) {
		for (let i = 0; i < cards.length; i++) {
			if (cards[i] === document.activeElement) return i;
		}
		return -1;
	}

	/**
	 * Moves focus to the card at the given index, clamped to valid range.
	 * @param {NodeList} cards
	 * @param {number} index
	 */
	function focusCard(cards, index) {
		if (cards.length === 0) return;
		const clamped = Math.max(0, Math.min(index, cards.length - 1));
		cards[clamped].focus();
		focusedKey = cards[clamped].dataset.key ?? null;
	}

	/**
	 * Handles a single logical gamepad action.
	 * @param {string} action
	 */
	function handleAction(action) {
		grid.dataset.input = "gamepad";
		const cards = grid.querySelectorAll(".game-card");

		switch (action) {
			case ACTION_ACTIVATE: {
				const idx = focusedCardIndex(cards);
				if (idx >= 0) cards[idx].click();
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
				focusedKey = null;
				sibling.click();
				// renderAll() was called synchronously by the click handler.
				const newCards = grid.querySelectorAll(".game-card");
				if (newCards.length > 0) {
					newCards[0].focus();
					focusedKey = newCards[0].dataset.key ?? null;
				}
				return;
			}

			default: {
				if (cards.length === 0) return;

				const current = focusedCardIndex(cards);
				// If nothing is focused yet, focus the first card on any directional input.
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

	/**
	 * Reads the current logical action (if any) from a gamepad snapshot.
	 * Combines D-pad buttons (12–15) and axes.
	 * @param {Gamepad} gp
	 * @returns {string|null}
	 */
	function readGamepadAction(gp) {
		const b = gp.buttons;

		// Buttons take priority over axes.
		if (b[12]?.pressed) return ACTION_UP;
		if (b[13]?.pressed) return ACTION_DOWN;
		if (b[14]?.pressed) return ACTION_LEFT;
		if (b[15]?.pressed) return ACTION_RIGHT;
		if (b[0]?.pressed || b[9]?.pressed) return ACTION_ACTIVATE;
		if (b[4]?.pressed) return ACTION_PREV_FILTER;
		if (b[5]?.pressed) return ACTION_NEXT_FILTER;

		// Fall back to axes (axis 0 = left/right, axis 1 = up/down for most controllers).
		const ax = gp.axes;
		if (ax.length >= 2) {
			if (ax[1] < -AXIS_THRESHOLD) return ACTION_UP;
			if (ax[1] > AXIS_THRESHOLD) return ACTION_DOWN;
			if (ax[0] < -AXIS_THRESHOLD) return ACTION_LEFT;
			if (ax[0] > AXIS_THRESHOLD) return ACTION_RIGHT;
		}

		return null;
	}

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
			const candidate = readGamepadAction(gp);
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
