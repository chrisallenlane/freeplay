if (typeof module !== "undefined") {
	require("./urls.js");
	require("./theme.js");
}

((exports) => {
	// parseSubpage parses a URL search string (e.g. window.location.search)
	// and returns the {consoleName, rom, gameName} triple a subpage needs.
	// gameName is rom with the final extension stripped — the slug
	// convention used everywhere (URLs, on-disk save paths, IGDB cache).
	// Returns null when console or rom is missing.
	exports.parseSubpage = (search) => {
		const params = new URLSearchParams(search);
		const consoleName = params.get("console");
		const rom = params.get("rom");
		if (!consoleName || !rom) return null;
		return { consoleName, rom, gameName: exports.stripExt(rom) };
	};

	exports.initSubpage = () => {
		const sub = exports.parseSubpage(window.location.search);
		if (!sub) return null;
		document.title = `Freeplay - ${sub.gameName}`;
		exports.initThemeToggle();
		return sub;
	};

	// Called per-invocation rather than cached: the user may toggle the
	// OS setting mid-session.
	exports.prefersReducedMotion = () =>
		window.matchMedia("(prefers-reduced-motion: reduce)").matches;

	// scrollElementIntoView scrolls el into view, honoring the user's
	// reduced-motion preference (instant instead of smooth).
	exports.scrollElementIntoView = (el, block = "nearest") => {
		el.scrollIntoView({
			behavior: exports.prefersReducedMotion() ? "instant" : "smooth",
			block,
		});
	};

	// safeExternalHref returns u if it parses as a valid https: URL, or
	// null otherwise. Defense-in-depth backstop for IGDB javascript:
	// XSS (SEC-2 / H-2): the server-side safeIGDBInfoURL is the primary
	// control, but this catches any regression that reaches the DOM.
	exports.safeExternalHref = (u) => {
		if (typeof u !== "string") return null;
		try {
			const parsed = new URL(u);
			if (parsed.protocol === "https:") return u;
		} catch {
			// malformed URL — fall through
		}
		return null;
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
