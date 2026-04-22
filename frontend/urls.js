((exports) => {
	exports.stripExt = (filename) => {
		const dot = filename.lastIndexOf(".");
		return dot > 0 ? filename.substring(0, dot) : filename;
	};

	exports.favKey = (game) => `${game.console}/${game.filename}`;

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

	exports.biosUrl = (consoleName) => `/bios/${encodeURIComponent(consoleName)}`;

	exports.saveBasePath = (consoleName, gameSlug) =>
		`/api/saves/${encodeURIComponent(consoleName)}/${encodeURIComponent(gameSlug)}`;

	exports.gameDetailsUrl = (consoleName, rom) =>
		`/api/game-details?console=${encodeURIComponent(consoleName)}&rom=${encodeURIComponent(rom)}`;
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
