((exports) => {
	const enc = encodeURIComponent;
	const gameQuery = (consoleName, rom) =>
		`?console=${enc(consoleName)}&rom=${enc(rom)}`;

	exports.stripExt = (filename) => {
		const dot = filename.lastIndexOf(".");
		return dot > 0 ? filename.substring(0, dot) : filename;
	};

	exports.favKey = (game) => `${game.console}/${game.filename}`;

	exports.coverUrl = (game) =>
		`/covers/${enc(game.console)}/${enc(exports.stripExt(game.filename))}.png`;

	exports.manualUrl = (game) =>
		`/manuals/${enc(game.console)}/${enc(exports.stripExt(game.filename))}.pdf`;

	exports.playUrl = (game) => `/play${gameQuery(game.console, game.filename)}`;

	exports.detailsUrl = (game) =>
		`/details${gameQuery(game.console, game.filename)}`;

	exports.romUrl = (consoleName, rom) =>
		`/roms/${enc(consoleName)}/${enc(rom)}`;

	exports.biosUrl = (consoleName) => `/bios/${enc(consoleName)}`;

	exports.saveBasePath = (consoleName, gameSlug) =>
		`/api/saves/${enc(consoleName)}/${enc(gameSlug)}`;

	exports.gameDetailsUrl = (consoleName, rom) =>
		`/api/game-details${gameQuery(consoleName, rom)}`;
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
