const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./grid.js");

describe("gridColumns", () => {
	it("returns 1 for an empty card list (avoids divide-by-zero in callers)", () => {
		assert.equal(FP.gridColumns([]), 1);
	});

	it("returns the count of cards sharing the first row's offsetTop", () => {
		const cards = [
			{ offsetTop: 0 },
			{ offsetTop: 0 },
			{ offsetTop: 0 },
			{ offsetTop: 200 },
			{ offsetTop: 200 },
		];
		assert.equal(FP.gridColumns(cards), 3);
	});

	it("returns 1 when all cards have distinct offsetTops (single column)", () => {
		const cards = [{ offsetTop: 0 }, { offsetTop: 100 }, { offsetTop: 200 }];
		assert.equal(FP.gridColumns(cards), 1);
	});

	it("returns the full count when every card shares the same offsetTop (single row)", () => {
		const cards = Array.from({ length: 7 }, () => ({ offsetTop: 0 }));
		assert.equal(FP.gridColumns(cards), 7);
	});

	it("stops counting at the first row break (does not look beyond)", () => {
		const cards = [
			{ offsetTop: 0 },
			{ offsetTop: 0 },
			{ offsetTop: 200 },
			{ offsetTop: 200 },
			// A card returning to row 0 (impossible in CSS grid layout but
			// pinned so a regression that loops past the first row break
			// fails this test).
			{ offsetTop: 0 },
		];
		assert.equal(FP.gridColumns(cards), 2);
	});
});

describe("findCardIndex", () => {
	it("returns the first matching index", () => {
		const cards = [{ k: "a" }, { k: "b" }, { k: "c" }];
		assert.equal(
			FP.findCardIndex(cards, (c) => c.k === "b"),
			1,
		);
	});

	it("returns -1 when no card matches", () => {
		const cards = [{ k: "a" }, { k: "b" }];
		assert.equal(
			FP.findCardIndex(cards, (c) => c.k === "z"),
			-1,
		);
	});

	it("returns -1 for an empty list", () => {
		assert.equal(
			FP.findCardIndex([], () => true),
			-1,
		);
	});

	it("stops at the first match (does not call predicate past the hit)", () => {
		let calls = 0;
		const cards = [{}, {}, {}, {}];
		FP.findCardIndex(cards, () => {
			calls++;
			return calls === 2;
		});
		assert.equal(calls, 2);
	});
});
