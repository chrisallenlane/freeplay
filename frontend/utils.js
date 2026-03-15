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

	exports.playUrl = (game) =>
		`/play?console=${encodeURIComponent(game.console)}&rom=${encodeURIComponent(game.filename)}`;

	exports.romUrl = (consoleName, rom) =>
		`/roms/${encodeURIComponent(consoleName)}/${encodeURIComponent(rom)}`;

	exports.saveBasePath = (consoleName, gameSlug) =>
		`/api/saves/${encodeURIComponent(consoleName)}/${encodeURIComponent(gameSlug)}`;

	exports.biosUrl = (consoleName) =>
		`/bios/${encodeURIComponent(consoleName)}/`;

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
