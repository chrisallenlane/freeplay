const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const FP = require("./api.js");

describe("filterGames", () => {
	const games = [
		{ console: "SNES", filename: "Zelda.smc" },
		{ console: "SNES", filename: "Mario.smc" },
		{ console: "NES", filename: "Zelda.nes" },
		{ console: "NES", filename: "Metroid.nes" },
	];

	it("returns all games with no filters", () => {
		const result = FP.filterGames(games, {});
		assert.equal(result.length, 4);
	});

	it("filters by console", () => {
		const result = FP.filterGames(games, { console: "NES" });
		assert.equal(result.length, 2);
		assert.ok(result.every((g) => g.console === "NES"));
	});

	it("filters by search query (case-insensitive)", () => {
		const result = FP.filterGames(games, { query: "zelda" });
		assert.equal(result.length, 2);
		assert.ok(result.every((g) => g.filename.toLowerCase().includes("zelda")));
	});

	it("filters by favorites", () => {
		const favs = new Set(["SNES/Mario.smc", "NES/Metroid.nes"]);
		const result = FP.filterGames(games, {
			favoritesOnly: true,
			favorites: favs,
		});
		assert.equal(result.length, 2);
		assert.deepEqual(
			result.map((g) => g.filename),
			["Mario.smc", "Metroid.nes"],
		);
	});

	it("combines console and query filters", () => {
		const result = FP.filterGames(games, { console: "SNES", query: "zel" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "Zelda.smc");
	});

	it("combines favorites and query filters", () => {
		const favs = new Set(["SNES/Zelda.smc", "SNES/Mario.smc", "NES/Zelda.nes"]);
		const result = FP.filterGames(games, {
			favoritesOnly: true,
			favorites: favs,
			query: "zelda",
		});
		assert.equal(result.length, 2);
	});

	it("returns empty array when nothing matches", () => {
		const result = FP.filterGames(games, { query: "nonexistent" });
		assert.equal(result.length, 0);
	});

	it("treats empty query as no filter", () => {
		const result = FP.filterGames(games, { query: "" });
		assert.equal(result.length, 4);
	});

	it("throws when favoritesOnly is true but favorites is missing", () => {
		assert.throws(() => FP.filterGames(games, { favoritesOnly: true }));
	});

	// Metadata search tests (developers, publishers, year, igdbName).
	const metaGames = [
		{
			console: "NES",
			filename: "smb.nes",
			igdbName: "Super Mario Bros.",
			developers: ["Nintendo R&D4"],
			publishers: ["Nintendo"],
			year: 1985,
		},
		{
			console: "NES",
			filename: "contra.nes",
			igdbName: "Contra",
			developers: ["Konami"],
			publishers: ["Konami"],
			year: 1987,
		},
		{
			console: "SNES",
			filename: "sf2.smc",
			igdbName: "Street Fighter II",
			developers: ["Capcom"],
			publishers: ["Capcom"],
			year: 1992,
		},
		{
			console: "NES",
			filename: "plain.nes",
			// no igdbName, developers, publishers, or year
		},
	];

	it("matches by developer substring", () => {
		const result = FP.filterGames(metaGames, { query: "konami" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "contra.nes");
	});

	it("matches by publisher substring", () => {
		const result = FP.filterGames(metaGames, { query: "capcom" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "sf2.smc");
	});

	it("matches by year", () => {
		const result = FP.filterGames(metaGames, { query: "1985" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "smb.nes");
	});

	it("matches by igdbName when filename differs", () => {
		const result = FP.filterGames(metaGames, { query: "mario" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "smb.nes");
	});

	it("multi-token AND across fields matches when all tokens present", () => {
		const result = FP.filterGames(metaGames, { query: "konami 1987" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "contra.nes");
	});

	it("multi-token AND returns no result when only one token matches", () => {
		const result = FP.filterGames(metaGames, { query: "konami 1985" });
		assert.equal(result.length, 0);
	});

	it("games without IGDB metadata still match by filename", () => {
		const result = FP.filterGames(metaGames, { query: "plain" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "plain.nes");
	});

	it("empty query returns all games", () => {
		const result = FP.filterGames(metaGames, { query: "" });
		assert.equal(result.length, metaGames.length);
	});

	it("whitespace-only query returns all games", () => {
		const result = FP.filterGames(metaGames, { query: "   " });
		assert.equal(result.length, metaGames.length);
	});

	it("extra whitespace in query works the same as trimmed query", () => {
		const trimmed = FP.filterGames(metaGames, { query: "konami 1987" });
		const padded = FP.filterGames(metaGames, { query: "  konami   1987  " });
		assert.deepEqual(
			trimmed.map((g) => g.filename),
			padded.map((g) => g.filename),
		);
	});
});

describe("findGame", () => {
	const games = [
		{ console: "SNES", filename: "Zelda.smc" },
		{ console: "NES", filename: "Zelda.nes" },
	];

	it("finds a game by console and filename", () => {
		const game = FP.findGame(games, "NES", "Zelda.nes");
		assert.equal(game.console, "NES");
		assert.equal(game.filename, "Zelda.nes");
	});

	it("returns null when not found", () => {
		assert.equal(FP.findGame(games, "NES", "Mario.nes"), null);
	});

	it("requires both console and filename to match", () => {
		assert.equal(FP.findGame(games, "SNES", "Zelda.nes"), null);
	});
});
