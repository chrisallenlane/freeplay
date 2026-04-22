if (typeof module !== "undefined") {
	require("./urls.js");
	require("./theme.js");
}

((exports) => {
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
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
