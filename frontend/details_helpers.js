// Pure data-shaping helpers for the details page. details.js builds
// DOM trees from the values these functions return; isolating the
// data shape keeps it testable without a DOM and pins the legacy-cache
// fallback behaviors in the gallery and meta-row code paths.
((exports) => {
	// buildMetaRows produces the label/value pairs the details page
	// renders into a key-value table. Rows whose value is falsy are
	// dropped so the table omits "Year:" rows for games whose IGDB
	// entry has no release date, etc.
	exports.buildMetaRows = (consoleName, details) =>
		[
			["Console", consoleName],
			["Year", details?.firstReleaseDate?.substring(0, 4)],
			["Developer", details?.developers?.join(", ")],
			["Publisher", details?.publishers?.join(", ")],
			["Platforms", details?.platforms?.join(", ")],
			["Series", details?.collection],
		].filter(([, v]) => v);

	// buildGalleryEntries produces the link/img pairs that make up a
	// screenshot or artwork gallery. The thumbUrl-or-url fallback is
	// the legacy-cache compatibility path (PERF-6): older details.json
	// files stored bare URL strings; the cache layer up-converts those
	// to {url} objects with no thumbUrl, so the gallery uses the full
	// image as both thumbnail and target.
	exports.buildGalleryEntries = (heading, refs) =>
		refs.map((ref, i) => ({
			href: ref.url,
			src: ref.thumbUrl || ref.url,
			ariaLabel: `View full image: ${heading} ${i + 1} of ${refs.length}`,
			alt: `${heading} ${i + 1} of ${refs.length}`,
		}));

	// splitParagraphs splits a multi-paragraph text block on blank-line
	// boundaries and trims each paragraph. Used by appendSection in
	// details.js to render IGDB-supplied summary/storyline strings as
	// proper <p> elements.
	exports.splitParagraphs = (text) =>
		text
			.split(/\n\n+/)
			.map((p) => p.trim())
			.filter(Boolean);
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
