var { describe, it } = require("node:test");
var assert = require("node:assert/strict");
var FP = require("./utils.js");

describe("stripExt", () => {
	it("removes a simple extension", () => {
		assert.equal(FP.stripExt("game.nes"), "game");
	});

	it("removes only the last extension", () => {
		assert.equal(FP.stripExt("my.cool.game.zip"), "my.cool.game");
	});

	it("returns the filename when there is no extension", () => {
		assert.equal(FP.stripExt("noext"), "noext");
	});

	it("returns the filename when dot is at position 0 (hidden file)", () => {
		assert.equal(FP.stripExt(".hidden"), ".hidden");
	});

	it("handles empty string", () => {
		assert.equal(FP.stripExt(""), "");
	});
});

describe("favKey", () => {
	it("joins console and filename with slash", () => {
		assert.equal(
			FP.favKey({ console: "SNES", filename: "zelda.smc" }),
			"SNES/zelda.smc",
		);
	});

	it("preserves special characters", () => {
		assert.equal(
			FP.favKey({ console: "Game Boy", filename: "Pok\u00e9mon (USA).gb" }),
			"Game Boy/Pok\u00e9mon (USA).gb",
		);
	});
});

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

describe("coverUrl", () => {
	it("builds a cover URL with encoded components", () => {
		assert.equal(
			FP.coverUrl({ console: "SNES", filename: "Zelda.smc" }),
			"/covers/SNES/Zelda.png",
		);
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.coverUrl({ console: "Game Boy", filename: "Pok\u00e9mon (USA).gb" }),
			"/covers/Game%20Boy/Pok%C3%A9mon%20(USA).png",
		);
	});
});

describe("playUrl", () => {
	it("builds a play URL with encoded components", () => {
		assert.equal(
			FP.playUrl({ console: "SNES", filename: "Zelda.smc" }),
			"/play?console=SNES&rom=Zelda.smc",
		);
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.playUrl({ console: "Game Boy", filename: "Pok\u00e9mon (USA).gb" }),
			"/play?console=Game%20Boy&rom=Pok%C3%A9mon%20(USA).gb",
		);
	});
});

describe("romUrl", () => {
	it("builds a ROM URL", () => {
		assert.equal(FP.romUrl("SNES", "Zelda.smc"), "/roms/SNES/Zelda.smc");
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.romUrl("Game Boy", "Pok\u00e9mon (USA).gb"),
			"/roms/Game%20Boy/Pok%C3%A9mon%20(USA).gb",
		);
	});
});

describe("saveBasePath", () => {
	it("builds a save base path", () => {
		assert.equal(FP.saveBasePath("SNES", "Zelda"), "/api/saves/SNES/Zelda");
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.saveBasePath("Game Boy", "Pok\u00e9mon (USA)"),
			"/api/saves/Game%20Boy/Pok%C3%A9mon%20(USA)",
		);
	});
});

describe("biosUrl", () => {
	it("builds a BIOS URL", () => {
		assert.equal(FP.biosUrl("SNES"), "/bios/SNES");
	});

	it("encodes special characters", () => {
		assert.equal(FP.biosUrl("Game Boy"), "/bios/Game%20Boy");
	});
});

describe("detailsUrl", () => {
	it("builds a details URL with encoded components", () => {
		assert.equal(
			FP.detailsUrl({ console: "SNES", filename: "Zelda.smc" }),
			"/details?console=SNES&rom=Zelda.smc",
		);
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.detailsUrl({ console: "Game Boy", filename: "Pok\u00e9mon (USA).gb" }),
			"/details?console=Game%20Boy&rom=Pok%C3%A9mon%20(USA).gb",
		);
	});
});

describe("manualUrl", () => {
	it("builds a manual URL, stripping the ROM extension", () => {
		assert.equal(
			FP.manualUrl({ console: "NES", filename: "Mega Man.nes" }),
			"/manuals/NES/Mega%20Man.pdf",
		);
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.manualUrl({ console: "Game Boy", filename: "Pok\u00e9mon (USA).gb" }),
			"/manuals/Game%20Boy/Pok%C3%A9mon%20(USA).pdf",
		);
	});
});

describe("gameDetailsUrl", () => {
	it("builds a game-details API URL with encoded components", () => {
		assert.equal(
			FP.gameDetailsUrl("NES", "Mega Man.nes"),
			"/api/game-details?console=NES&rom=Mega%20Man.nes",
		);
	});

	it("encodes special characters", () => {
		assert.equal(
			FP.gameDetailsUrl("Game Boy", "Pok\u00e9mon (USA).gb"),
			"/api/game-details?console=Game%20Boy&rom=Pok%C3%A9mon%20(USA).gb",
		);
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

describe("readGamepadAction", () => {
	function makeGamepad(overrides) {
		var buttons = Array.from({ length: 16 }, () => ({ pressed: false }));
		var gp = { buttons: buttons, axes: [0, 0] };
		if (overrides) overrides(gp);
		return gp;
	}

	it("returns null when nothing is pressed", () => {
		assert.equal(FP.readGamepadAction(makeGamepad()), null);
	});

	it("maps D-pad up (button 12) to ACTION_UP", () => {
		var gp = makeGamepad((g) => (g.buttons[12].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("maps D-pad down (button 13) to ACTION_DOWN", () => {
		var gp = makeGamepad((g) => (g.buttons[13].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_DOWN);
	});

	it("maps D-pad left (button 14) to ACTION_LEFT", () => {
		var gp = makeGamepad((g) => (g.buttons[14].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_LEFT);
	});

	it("maps D-pad right (button 15) to ACTION_RIGHT", () => {
		var gp = makeGamepad((g) => (g.buttons[15].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_RIGHT);
	});

	it("maps button 0 (A/Cross) to ACTION_ACTIVATE", () => {
		var gp = makeGamepad((g) => (g.buttons[0].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_ACTIVATE);
	});

	it("maps button 9 (Start) to ACTION_ACTIVATE", () => {
		var gp = makeGamepad((g) => (g.buttons[9].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_ACTIVATE);
	});

	it("maps L1 (button 4) to ACTION_PREV_FILTER", () => {
		var gp = makeGamepad((g) => (g.buttons[4].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_PREV_FILTER);
	});

	it("maps R1 (button 5) to ACTION_NEXT_FILTER", () => {
		var gp = makeGamepad((g) => (g.buttons[5].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_NEXT_FILTER);
	});

	it("falls back to axes when no buttons pressed", () => {
		var gp = makeGamepad((g) => (g.axes = [0, -0.8]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("maps positive Y axis to ACTION_DOWN", () => {
		var gp = makeGamepad((g) => (g.axes = [0, 0.8]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_DOWN);
	});

	it("maps negative X axis to ACTION_LEFT", () => {
		var gp = makeGamepad((g) => (g.axes = [-0.8, 0]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_LEFT);
	});

	it("maps positive X axis to ACTION_RIGHT", () => {
		var gp = makeGamepad((g) => (g.axes = [0.8, 0]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_RIGHT);
	});

	it("ignores axes below threshold", () => {
		var gp = makeGamepad((g) => (g.axes = [0.3, -0.4]));
		assert.equal(FP.readGamepadAction(gp), null);
	});

	it("buttons take priority over axes", () => {
		var gp = makeGamepad((g) => {
			g.buttons[12].pressed = true;
			g.axes = [0.8, 0];
		});
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("handles gamepads with fewer than 16 buttons", () => {
		var gp = { buttons: [{ pressed: false }], axes: [0, 0] };
		assert.equal(FP.readGamepadAction(gp), null);
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
