if (typeof module !== "undefined") {
	require("./urls.js");
}

((exports) => {
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
