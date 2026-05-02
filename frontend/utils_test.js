const test = require("node:test");
const assert = require("node:assert/strict");

// No DOM stubs: theme.js (required by utils.js) early-returns when
// typeof document === "undefined". safeExternalHref itself does not
// touch window/document.

const FP = require("./utils.js");

test("safeExternalHref", async (t) => {
	await t.test("accepts valid https URL", () => {
		assert.equal(
			FP.safeExternalHref("https://www.igdb.com/games/mega-man"),
			"https://www.igdb.com/games/mega-man",
		);
	});

	await t.test("rejects javascript: scheme", () => {
		assert.equal(FP.safeExternalHref("javascript:alert(1)"), null);
	});

	await t.test("rejects data: scheme", () => {
		assert.equal(
			FP.safeExternalHref("data:text/html,<script>alert(1)</script>"),
			null,
		);
	});

	await t.test("rejects http:", () => {
		assert.equal(FP.safeExternalHref("http://attacker.example/"), null);
	});

	await t.test("rejects file:", () => {
		assert.equal(FP.safeExternalHref("file:///etc/passwd"), null);
	});

	await t.test("rejects malformed URL", () => {
		assert.equal(FP.safeExternalHref("not a url"), null);
	});

	await t.test("rejects non-string input", () => {
		assert.equal(FP.safeExternalHref(undefined), null);
		assert.equal(FP.safeExternalHref(null), null);
		assert.equal(FP.safeExternalHref(42), null);
	});
});

test("parseSubpage", async (t) => {
	await t.test("returns null when console and rom are absent", () => {
		assert.equal(FP.parseSubpage(""), null);
		assert.equal(FP.parseSubpage("?other=x"), null);
	});

	await t.test("returns null when only console is present", () => {
		assert.equal(FP.parseSubpage("?console=NES"), null);
	});

	await t.test("returns null when only rom is present", () => {
		assert.equal(FP.parseSubpage("?rom=Mega+Man.nes"), null);
	});

	await t.test(
		"strips the extension to produce gameName (slug convention)",
		() => {
			assert.deepEqual(FP.parseSubpage("?console=NES&rom=Mega+Man.nes"), {
				consoleName: "NES",
				rom: "Mega Man.nes",
				gameName: "Mega Man",
			});
		},
	);

	await t.test("preserves gameName when filename has no extension", () => {
		assert.deepEqual(FP.parseSubpage("?console=NES&rom=PlainName"), {
			consoleName: "NES",
			rom: "PlainName",
			gameName: "PlainName",
		});
	});

	await t.test("URL-decodes parameters", () => {
		assert.deepEqual(
			FP.parseSubpage("?console=Game%20Boy&rom=Pok%C3%A9mon.gb"),
			{
				consoleName: "Game Boy",
				rom: "Pokémon.gb",
				gameName: "Pokémon",
			},
		);
	});
});
