var { describe, it } = require("node:test");
var assert = require("node:assert/strict");
var FP = require("./urls.js");

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
			FP.favKey({ console: "Game Boy", filename: "Pokémon (USA).gb" }),
			"Game Boy/Pokémon (USA).gb",
		);
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
			FP.coverUrl({ console: "Game Boy", filename: "Pokémon (USA).gb" }),
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
			FP.playUrl({ console: "Game Boy", filename: "Pokémon (USA).gb" }),
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
			FP.romUrl("Game Boy", "Pokémon (USA).gb"),
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
			FP.saveBasePath("Game Boy", "Pokémon (USA)"),
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
			FP.detailsUrl({ console: "Game Boy", filename: "Pokémon (USA).gb" }),
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
			FP.manualUrl({ console: "Game Boy", filename: "Pokémon (USA).gb" }),
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
			FP.gameDetailsUrl("Game Boy", "Pokémon (USA).gb"),
			"/api/game-details?console=Game%20Boy&rom=Pok%C3%A9mon%20(USA).gb",
		);
	});
});
