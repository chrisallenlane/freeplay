((exports) => {
	exports.stripExt = (filename) => {
		const dot = filename.lastIndexOf(".");
		return dot > 0 ? filename.substring(0, dot) : filename;
	};

	exports.favKey = (game) => `${game.console}/${game.filename}`;

	exports.filterGames = (games, opts) => {
		const query = (opts.query || "").toLowerCase();
		return games.filter((g) => {
			if (opts.favoritesOnly && !opts.favorites.has(exports.favKey(g)))
				return false;
			if (opts.console && g.console !== opts.console) return false;
			if (query && !g.filename.toLowerCase().includes(query)) return false;
			return true;
		});
	};

	exports.findGame = (games, consoleName, filename) =>
		games.find((g) => g.console === consoleName && g.filename === filename) ??
		null;

	exports.coverUrl = (game) =>
		`/covers/${encodeURIComponent(game.console)}/${encodeURIComponent(exports.stripExt(game.filename))}.png`;

	exports.manualUrl = (game) =>
		`/manuals/${encodeURIComponent(game.console)}/${encodeURIComponent(exports.stripExt(game.filename))}.pdf`;

	exports.playUrl = (game) =>
		`/play?console=${encodeURIComponent(game.console)}&rom=${encodeURIComponent(game.filename)}`;

	exports.detailsUrl = (game) =>
		`/details?console=${encodeURIComponent(game.console)}&rom=${encodeURIComponent(game.filename)}`;

	exports.romUrl = (consoleName, rom) =>
		`/roms/${encodeURIComponent(consoleName)}/${encodeURIComponent(rom)}`;

	exports.saveBasePath = (consoleName, gameSlug) =>
		`/api/saves/${encodeURIComponent(consoleName)}/${encodeURIComponent(gameSlug)}`;

	exports.biosUrl = (consoleName) => `/bios/${encodeURIComponent(consoleName)}`;

	exports.gameDetailsUrl = (consoleName, rom) =>
		`/api/game-details?console=${encodeURIComponent(consoleName)}&rom=${encodeURIComponent(rom)}`;

	// Logical actions for directional navigation (shared by keyboard and gamepad).
	exports.ACTION_LEFT = "left";
	exports.ACTION_RIGHT = "right";
	exports.ACTION_UP = "up";
	exports.ACTION_DOWN = "down";
	exports.ACTION_ACTIVATE = "activate";
	exports.ACTION_BACK = "back";
	exports.ACTION_PREV_FILTER = "prevFilter";
	exports.ACTION_NEXT_FILTER = "nextFilter";

	exports.gridColumns = (cards) => {
		if (cards.length === 0) return 1;
		const firstTop = cards[0].offsetTop;
		let cols = 0;
		for (const card of cards) {
			if (card.offsetTop !== firstTop) break;
			cols++;
		}
		return cols;
	};

	exports.findCardIndex = (cards, predicate) => {
		for (let i = 0; i < cards.length; i++) {
			if (predicate(cards[i])) return i;
		}
		return -1;
	};

	exports.readGamepadAction = (gp) => {
		const b = gp.buttons;
		if (b[12]?.pressed) return exports.ACTION_UP;
		if (b[13]?.pressed) return exports.ACTION_DOWN;
		if (b[14]?.pressed) return exports.ACTION_LEFT;
		if (b[15]?.pressed) return exports.ACTION_RIGHT;
		if (b[0]?.pressed || b[9]?.pressed) return exports.ACTION_ACTIVATE;
		if (b[1]?.pressed) return exports.ACTION_BACK;
		if (b[4]?.pressed) return exports.ACTION_PREV_FILTER;
		if (b[5]?.pressed) return exports.ACTION_NEXT_FILTER;

		const ax = gp.axes;
		if (ax.length >= 2) {
			if (ax[1] < -0.5) return exports.ACTION_UP;
			if (ax[1] > 0.5) return exports.ACTION_DOWN;
			if (ax[0] < -0.5) return exports.ACTION_LEFT;
			if (ax[0] > 0.5) return exports.ACTION_RIGHT;
		}

		return null;
	};

	/**
	 * Starts a gamepad poll loop and manages connect/disconnect listeners.
	 * Calls `handleAction` with a logical action string whenever input is read.
	 * @param {(action: string) => void} handleAction
	 */
	exports.gamepadLoop = (handleAction) => {
		// Number of gamepads currently connected.
		let gamepadCount = 0;

		// ID of the running requestAnimationFrame loop, or null when stopped.
		let rafId = null;

		// Debounce state: which logical action is being held and when it last fired.
		let heldAction = null;
		let heldLastFired = 0;

		// Debounce interval (ms).
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
				const candidate = exports.readGamepadAction(gp);
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
	};

	exports.showError = (containerId, msg) => {
		document.getElementById(containerId).style.display = "none";
		const el = document.getElementById("error");
		el.style.display = "";
		el.textContent = msg;
	};

	exports.initSubpage = () => {
		const params = new URLSearchParams(window.location.search);
		const consoleName = params.get("console");
		const rom = params.get("rom");
		if (!consoleName || !rom) return null;
		const gameName = exports.stripExt(rom);
		document.title = `Freeplay - ${gameName}`;
		exports.initThemeToggle();
		return { consoleName, rom, gameName };
	};

	exports.initThemeToggle = () => {
		const btn = document.getElementById("theme-toggle");
		if (!btn) return;

		const update = () => {
			btn.textContent =
				document.documentElement.dataset.theme === "light"
					? "\u263D"
					: "\u2600";
		};

		btn.addEventListener("click", () => {
			const next =
				document.documentElement.dataset.theme === "light" ? "dark" : "light";
			document.documentElement.dataset.theme = next;
			localStorage.setItem("freeplay-theme", next);
			update();
		});

		update();
	};
})(typeof module !== "undefined" ? module.exports : (window.Freeplay = {}));
