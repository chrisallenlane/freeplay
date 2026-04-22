var { describe, it } = require("node:test");
var assert = require("node:assert/strict");
var FP = require("./utils.js");

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
