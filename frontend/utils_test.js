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
		assert.equal(FP.biosUrl("SNES"), "/bios/SNES/");
	});

	it("encodes special characters", () => {
		assert.equal(FP.biosUrl("Game Boy"), "/bios/Game%20Boy/");
	});
});
