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
