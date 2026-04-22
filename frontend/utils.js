if (typeof module !== "undefined") {
	require("./urls.js");
}

((exports) => {
	exports.filterGames = (games, opts) => {
		const tokens = (opts.query || "")
			.toLowerCase()
			.split(/\s+/)
			.filter((t) => t.length > 0);
		return games.filter((g) => {
			if (opts.favoritesOnly && !opts.favorites.has(exports.favKey(g)))
				return false;
			if (opts.console && g.console !== opts.console) return false;
			if (tokens.length > 0) {
				const parts = [g.filename.toLowerCase()];
				if (g.igdbName) parts.push(g.igdbName.toLowerCase());
				if (g.developers) {
					for (const d of g.developers) parts.push(d.toLowerCase());
				}
				if (g.publishers) {
					for (const p of g.publishers) parts.push(p.toLowerCase());
				}
				if (g.year) parts.push(String(g.year));
				const corpus = parts.join(" ");
				if (!tokens.every((t) => corpus.includes(t))) return false;
			}
			return true;
		});
	};

	exports.findGame = (games, consoleName, filename) =>
		games.find((g) => g.console === consoleName && g.filename === filename) ??
		null;

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

	exports.el = (tag, cls, text) => {
		const node = document.createElement(tag);
		if (cls) node.className = cls;
		if (text !== undefined) node.textContent = text;
		return node;
	};

	/**
	 * Fetches the game catalog, finds a game by console and filename, and returns
	 * it. Returns null (and renders an error into errorContainerId) if the game is
	 * not in the catalog. Throws on network or parse failure so the caller's catch
	 * handler still fires.
	 * @param {string} consoleName
	 * @param {string} gameName
	 * @param {string} errorContainerId
	 * @returns {Promise<object|null>}
	 */
	exports.loadGame = async (consoleName, gameName, errorContainerId) => {
		const res = await fetch("/api/games");
		if (!res.ok) {
			throw new Error(`HTTP ${res.status}`);
		}
		const catalog = await res.json();
		const game = exports.findGame(catalog.games, consoleName, gameName);
		if (!game) {
			exports.showError(
				errorContainerId,
				"Game not found. It may have been removed from the library.",
			);
			return null;
		}
		return game;
	};

	exports.loadGameDetails = async (consoleName, rom) => {
		try {
			const res = await fetch(exports.gameDetailsUrl(consoleName, rom));
			return res.ok ? await res.json() : null;
		} catch {
			return null;
		}
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
				document.documentElement.dataset.theme === "light" ? "☽" : "☀";
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
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
