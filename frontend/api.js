if (typeof module !== "undefined") {
	require("./urls.js");
}

((exports) => {
	exports.el = (tag, cls, text) => {
		const node = document.createElement(tag);
		if (cls) node.className = cls;
		if (text !== undefined) node.textContent = text;
		return node;
	};

	exports.showError = (containerId, msg) => {
		document.getElementById(containerId).style.display = "none";
		const el = document.getElementById("error");
		el.style.display = "";
		el.textContent = msg;
	};

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
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
