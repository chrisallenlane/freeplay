// Pure grid-layout helpers shared between app.js's keyboard/gamepad
// navigation and the tests. Cards are objects with an `offsetTop`
// number; the predicate variant is over arbitrary card-shaped values.
((exports) => {
	// gridColumns counts how many cards share the first row's offsetTop.
	// Returns 1 for an empty list so callers can divide without guarding.
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

	// findCardIndex returns the index of the first card matching
	// predicate, or -1 if none match. Same shape as Array.prototype
	// .findIndex; lifted for use against NodeList in the page (which
	// historically didn't support .findIndex).
	exports.findCardIndex = (cards, predicate) => {
		for (let i = 0; i < cards.length; i++) {
			if (predicate(cards[i])) return i;
		}
		return -1;
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
