const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./details_helpers.js");

describe("buildMetaRows", () => {
	it("returns only Console when details is undefined", () => {
		assert.deepEqual(FP.buildMetaRows("NES", undefined), [["Console", "NES"]]);
	});

	it("returns only Console when details is empty", () => {
		assert.deepEqual(FP.buildMetaRows("NES", {}), [["Console", "NES"]]);
	});

	it("extracts the year from an ISO date prefix", () => {
		const rows = FP.buildMetaRows("NES", { firstReleaseDate: "1987-12-17" });
		assert.deepEqual(rows, [
			["Console", "NES"],
			["Year", "1987"],
		]);
	});

	it("joins developers and publishers with comma-space", () => {
		const rows = FP.buildMetaRows("NES", {
			developers: ["Capcom", "Nintendo"],
			publishers: ["Nintendo of America"],
		});
		assert.deepEqual(rows, [
			["Console", "NES"],
			["Developer", "Capcom, Nintendo"],
			["Publisher", "Nintendo of America"],
		]);
	});

	it("omits rows with empty arrays after the join (filter sees empty string)", () => {
		const rows = FP.buildMetaRows("NES", {
			developers: [],
			platforms: [],
			collection: "Mega Man",
		});
		// "" is falsy so the filter drops Developer/Platforms rows.
		assert.deepEqual(rows, [
			["Console", "NES"],
			["Series", "Mega Man"],
		]);
	});

	it("emits all rows when every field is present", () => {
		const rows = FP.buildMetaRows("NES", {
			firstReleaseDate: "1990-01-01",
			developers: ["A"],
			publishers: ["B"],
			platforms: ["C", "D"],
			collection: "E",
		});
		assert.deepEqual(rows, [
			["Console", "NES"],
			["Year", "1990"],
			["Developer", "A"],
			["Publisher", "B"],
			["Platforms", "C, D"],
			["Series", "E"],
		]);
	});
});

describe("buildGalleryEntries", () => {
	it("uses thumbUrl as src and url as href when both are present", () => {
		const refs = [{ url: "/full/a.jpg", thumbUrl: "/thumb/a.jpg" }];
		assert.deepEqual(FP.buildGalleryEntries("Screenshots", refs), [
			{
				href: "/full/a.jpg",
				src: "/thumb/a.jpg",
				ariaLabel: "View full image: Screenshots 1 of 1",
				alt: "Screenshots 1 of 1",
			},
		]);
	});

	it("falls back to url when thumbUrl is absent (PERF-6 legacy-cache compat)", () => {
		// Older details.json (pre-PERF-6) stored screenshots/artworks as
		// bare URL strings; the cache layer up-converts those to {url}
		// objects with no thumbUrl. The gallery must still render.
		const refs = [{ url: "/legacy/a.jpg" }];
		const entries = FP.buildGalleryEntries("Artworks", refs);
		assert.equal(entries[0].src, "/legacy/a.jpg");
		assert.equal(entries[0].href, "/legacy/a.jpg");
	});

	it("falls back to url when thumbUrl is empty string", () => {
		const refs = [{ url: "/full/a.jpg", thumbUrl: "" }];
		assert.equal(
			FP.buildGalleryEntries("Screenshots", refs)[0].src,
			"/full/a.jpg",
		);
	});

	it("renders heading and 1-indexed position into aria-label and alt", () => {
		const refs = [{ url: "/a.jpg" }, { url: "/b.jpg" }, { url: "/c.jpg" }];
		const entries = FP.buildGalleryEntries("Screenshots", refs);
		assert.equal(entries[0].ariaLabel, "View full image: Screenshots 1 of 3");
		assert.equal(entries[1].alt, "Screenshots 2 of 3");
		assert.equal(entries[2].alt, "Screenshots 3 of 3");
	});

	it("returns an empty array for empty refs", () => {
		assert.deepEqual(FP.buildGalleryEntries("Artworks", []), []);
	});
});

describe("splitParagraphs", () => {
	it("splits on blank lines and trims each paragraph", () => {
		assert.deepEqual(FP.splitParagraphs("first\n\nsecond"), [
			"first",
			"second",
		]);
	});

	it("collapses multiple blank lines into a single boundary", () => {
		assert.deepEqual(FP.splitParagraphs("a\n\n\n\nb"), ["a", "b"]);
	});

	it("drops empty paragraphs (e.g. leading/trailing blank lines)", () => {
		assert.deepEqual(FP.splitParagraphs("\n\nactual\n\n"), ["actual"]);
	});

	it("returns a single-element array when there are no paragraph breaks", () => {
		assert.deepEqual(FP.splitParagraphs("one block"), ["one block"]);
	});

	it("returns an empty array for empty input", () => {
		assert.deepEqual(FP.splitParagraphs(""), []);
	});

	it("preserves single newlines within a paragraph", () => {
		assert.deepEqual(FP.splitParagraphs("line1\nline2\n\npara2"), [
			"line1\nline2",
			"para2",
		]);
	});
});
