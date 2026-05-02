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

	// Case-insensitivity must apply to the query itself (not just the
	// fields). Kills the .toLowerCase() mutation on opts.query.
	it("uppercase query matches lowercase field content", () => {
		const result = FP.filterGames(metaGames, { query: "KONAMI" });
		assert.equal(result.length, 1);
		assert.equal(result[0].filename, "contra.nes");
	});

	// The developer/publisher lists are lowercased before joining. Test
	// with uppercase developer content to kill the .toLowerCase() mutations.
	it("query matches uppercase developer/publisher content case-insensitively", () => {
		const games = [
			{ console: "NES", filename: "x.nes", developers: ["BIG COMPANY"] },
			{ console: "NES", filename: "y.nes", publishers: ["PUB CO"] },
		];
		// Lowercase query should match uppercase developer name.
		assert.equal(FP.filterGames(games, { query: "big" }).length, 1);
		// Lowercase query should match uppercase publisher name.
		assert.equal(FP.filterGames(games, { query: "pub" }).length, 1);
	});

	// The corpus is field-joined with " ". Across-field substring match
	// must NOT work — e.g. "contrakonami" must not match because the
	// filename and developer are different fields. Kills the
	// parts.join(" ") -> parts.join("") mutation (which would produce a
	// concatenated string that lets arbitrary cross-field substrings match).
	it("query does not match cross-field concatenation (join keeps separator)", () => {
		// "konami1987" spans publisher and year with no separator in the
		// mutated corpus; with the correct separator it's never a substring.
		const result = FP.filterGames(metaGames, { query: "konami1987" });
		assert.equal(result.length, 0);
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

describe("loadGame", () => {
	let origFetch, origShowError;
	let showErrorCalls;

	const setFetch = (impl) => {
		globalThis.fetch = impl;
	};

	const beforeEach = () => {
		origFetch = globalThis.fetch;
		origShowError = FP.showError;
		showErrorCalls = [];
		FP.showError = (id, msg) => showErrorCalls.push({ id, msg });
	};

	const afterEach = () => {
		globalThis.fetch = origFetch;
		FP.showError = origShowError;
	};

	it("returns the game on success", async () => {
		beforeEach();
		try {
			setFetch(() =>
				Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							games: [{ console: "NES", filename: "Mega Man.nes" }],
						}),
				}),
			);
			const game = await FP.loadGame("NES", "Mega Man.nes", "content");
			assert.equal(game.filename, "Mega Man.nes");
			assert.equal(game.console, "NES");
			assert.equal(showErrorCalls.length, 0);
		} finally {
			afterEach();
		}
	});

	it("throws on non-ok HTTP response so callers can render an error", async () => {
		beforeEach();
		try {
			setFetch(() => Promise.resolve({ ok: false, status: 500 }));
			await assert.rejects(FP.loadGame("NES", "x", "content"), /HTTP 500/);
			// Catalog itself failed to load — game-not-found error is not shown
			// from loadGame; that's the caller's job via .catch.
			assert.equal(showErrorCalls.length, 0);
		} finally {
			afterEach();
		}
	});

	it("calls showError and returns null when game not in catalog", async () => {
		beforeEach();
		try {
			setFetch(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ games: [] }),
				}),
			);
			const game = await FP.loadGame("NES", "Missing.nes", "content");
			assert.equal(game, null);
			assert.equal(showErrorCalls.length, 1);
			assert.equal(showErrorCalls[0].id, "content");
			assert.match(showErrorCalls[0].msg, /Game not found/);
		} finally {
			afterEach();
		}
	});
});

describe("loadGameDetails", () => {
	let origFetch;

	const beforeEach = () => {
		origFetch = globalThis.fetch;
	};
	const afterEach = () => {
		globalThis.fetch = origFetch;
	};

	it("returns parsed JSON on success", async () => {
		beforeEach();
		try {
			globalThis.fetch = () =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ name: "Mega Man" }),
				});
			const details = await FP.loadGameDetails("NES", "Mega Man.nes");
			assert.deepEqual(details, { name: "Mega Man" });
		} finally {
			afterEach();
		}
	});

	it("returns null on non-ok response (cache miss is not an error)", async () => {
		beforeEach();
		try {
			globalThis.fetch = () => Promise.resolve({ ok: false, status: 404 });
			const details = await FP.loadGameDetails("NES", "x");
			assert.equal(details, null);
		} finally {
			afterEach();
		}
	});

	it("returns null on fetch rejection (network error swallowed)", async () => {
		beforeEach();
		try {
			globalThis.fetch = () => Promise.reject(new TypeError("network down"));
			const details = await FP.loadGameDetails("NES", "x");
			assert.equal(details, null);
		} finally {
			afterEach();
		}
	});
});
