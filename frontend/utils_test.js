var { describe, it } = require("node:test");
var assert = require("node:assert/strict");
var FP = require("./utils.js");

describe("filterGames", () => {
	var games = [
		{ console: "SNES", filename: "Zelda.smc" },
		{ console: "SNES", filename: "Mario.smc" },
		{ console: "NES", filename: "Zelda.nes" },
		{ console: "NES", filename: "Metroid.nes" },
	];

	it("returns all games with no filters", () => {
		var result = FP.filterGames(games, {});
		assert.equal(result.length, 4);
	});

	it("filters by console", () => {
		var result = FP.filterGames(games, { console: "NES" });
		assert.equal(result.length, 2);
		assert.ok(result.every((g) => g.console === "NES"));
	});

	it("filters by search query (case-insensitive)", () => {
		var result = FP.filterGames(games, { query: "zelda" });
		assert.equal(result.length, 2);
		assert.ok(result.every((g) => g.filename.toLowerCase().includes("zelda")));
	});

	it("filters by favorites", () => {
		var favs = new Set(["SNES/Mario.smc", "NES/Metroid.nes"]);
		var result = FP.filterGames(games, {
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
		var result = FP.filterGames(games, { console: "SNES", query: "zel" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "Zelda.smc");
	});

	it("combines favorites and query filters", () => {
		var favs = new Set(["SNES/Zelda.smc", "SNES/Mario.smc", "NES/Zelda.nes"]);
		var result = FP.filterGames(games, {
			favoritesOnly: true,
			favorites: favs,
			query: "zelda",
		});
		assert.equal(result.length, 2);
	});

	it("returns empty array when nothing matches", () => {
		var result = FP.filterGames(games, { query: "nonexistent" });
		assert.equal(result.length, 0);
	});

	it("treats empty query as no filter", () => {
		var result = FP.filterGames(games, { query: "" });
		assert.equal(result.length, 4);
	});

	it("throws when favoritesOnly is true but favorites is missing", () => {
		assert.throws(() => FP.filterGames(games, { favoritesOnly: true }));
	});

	// Metadata search tests (developers, publishers, year, igdbName).
	var metaGames = [
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
		var result = FP.filterGames(metaGames, { query: "konami" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "contra.nes");
	});

	it("matches by publisher substring", () => {
		var result = FP.filterGames(metaGames, { query: "capcom" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "sf2.smc");
	});

	it("matches by year", () => {
		var result = FP.filterGames(metaGames, { query: "1985" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "smb.nes");
	});

	it("matches by igdbName when filename differs", () => {
		var result = FP.filterGames(metaGames, { query: "mario" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "smb.nes");
	});

	it("multi-token AND across fields matches when all tokens present", () => {
		var result = FP.filterGames(metaGames, { query: "konami 1987" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "contra.nes");
	});

	it("multi-token AND returns no result when only one token matches", () => {
		var result = FP.filterGames(metaGames, { query: "konami 1985" });
		assert.equal(result.length, 0);
	});

	it("games without IGDB metadata still match by filename", () => {
		var result = FP.filterGames(metaGames, { query: "plain" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "plain.nes");
	});

	it("empty query returns all games", () => {
		var result = FP.filterGames(metaGames, { query: "" });
		assert.equal(result.length, metaGames.length);
	});

	it("whitespace-only query returns all games", () => {
		var result = FP.filterGames(metaGames, { query: "   " });
		assert.equal(result.length, metaGames.length);
	});

	it("extra whitespace in query works the same as trimmed query", () => {
		var trimmed = FP.filterGames(metaGames, { query: "konami 1987" });
		var padded = FP.filterGames(metaGames, { query: "  konami   1987  " });
		assert.deepEqual(
			trimmed.map((g) => g.filename),
			padded.map((g) => g.filename),
		);
	});
});

describe("findGame", () => {
	var games = [
		{ console: "SNES", filename: "Zelda.smc" },
		{ console: "NES", filename: "Zelda.nes" },
	];

	it("finds a game by console and filename", () => {
		var game = FP.findGame(games, "NES", "Zelda.nes");
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

describe("gridColumns", () => {
	it("returns 1 for empty list", () => {
		assert.equal(FP.gridColumns([]), 1);
	});

	it("counts cards sharing the same offsetTop", () => {
		var cards = [
			{ offsetTop: 0 },
			{ offsetTop: 0 },
			{ offsetTop: 0 },
			{ offsetTop: 100 },
			{ offsetTop: 100 },
		];
		assert.equal(FP.gridColumns(cards), 3);
	});

	it("returns total count when all cards are on one row", () => {
		var cards = [{ offsetTop: 0 }, { offsetTop: 0 }];
		assert.equal(FP.gridColumns(cards), 2);
	});

	it("returns 1 when each card is on its own row", () => {
		var cards = [{ offsetTop: 0 }, { offsetTop: 50 }, { offsetTop: 100 }];
		assert.equal(FP.gridColumns(cards), 1);
	});
});

describe("findCardIndex", () => {
	var items = ["a", "b", "c", "d"];

	it("returns index of first match", () => {
		assert.equal(
			FP.findCardIndex(items, (x) => x === "c"),
			2,
		);
	});

	it("returns -1 when nothing matches", () => {
		assert.equal(
			FP.findCardIndex(items, (x) => x === "z"),
			-1,
		);
	});

	it("returns first match when multiple exist", () => {
		var dupes = ["x", "y", "x"];
		assert.equal(
			FP.findCardIndex(dupes, (x) => x === "x"),
			0,
		);
	});

	it("works with empty list", () => {
		assert.equal(
			FP.findCardIndex([], () => true),
			-1,
		);
	});
});

describe("initThemeToggle", () => {
	function withThemeMocks(theme, hasButton, fn) {
		var clickHandler;
		var btn = hasButton
			? {
					textContent: "",
					addEventListener: (event, handler) => {
						if (event === "click") clickHandler = handler;
					},
				}
			: null;
		var dataset = { theme: theme };
		var storage = {};
		var origDoc = globalThis.document;
		var origStorage = globalThis.localStorage;
		globalThis.document = {
			getElementById: (id) => (id === "theme-toggle" ? btn : null),
			documentElement: { dataset: dataset },
		};
		globalThis.localStorage = {
			setItem: (k, v) => (storage[k] = v),
		};
		try {
			fn({ btn, dataset, storage, click: () => clickHandler() });
		} finally {
			globalThis.document = origDoc;
			globalThis.localStorage = origStorage;
		}
	}

	it("sets sun icon for dark theme", () => {
		withThemeMocks("dark", true, ({ btn }) => {
			FP.initThemeToggle();
			assert.equal(btn.textContent, "\u2600");
		});
	});

	it("sets moon icon for light theme", () => {
		withThemeMocks("light", true, ({ btn }) => {
			FP.initThemeToggle();
			assert.equal(btn.textContent, "\u263D");
		});
	});

	it("does nothing when button is not found", () => {
		withThemeMocks("dark", false, ({ storage }) => {
			FP.initThemeToggle();
			assert.deepEqual(storage, {});
		});
	});

	it("toggles theme on click", () => {
		withThemeMocks("dark", true, ({ btn, dataset, storage, click }) => {
			FP.initThemeToggle();
			assert.equal(btn.textContent, "\u2600");

			click();
			assert.equal(dataset.theme, "light");
			assert.equal(storage["freeplay-theme"], "light");
			assert.equal(btn.textContent, "\u263D");

			click();
			assert.equal(dataset.theme, "dark");
			assert.equal(storage["freeplay-theme"], "dark");
			assert.equal(btn.textContent, "\u2600");
		});
	});
});
