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
