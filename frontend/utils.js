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

	// Called per-invocation rather than cached: the user may toggle the
	// OS setting mid-session.
	exports.prefersReducedMotion = () =>
		window.matchMedia("(prefers-reduced-motion: reduce)").matches;
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
