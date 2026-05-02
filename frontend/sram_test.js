const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

// restoreSaveToFS is the pure SRAM-restore helper extracted from play.js.
// play.js calls FP.restoreSaveToFS(gm, buf) so these tests exercise the
// same code path the browser runs.
const FP = require("./sram.js");

// Drive FP.restoreSaveToFS against an in-memory gameManager stub and
// return the log of FS operations performed.
function simulateRestore(path, existingPaths, buf) {
	const ops = [];
	const known = new Set(existingPaths || []);

	const gm = {
		FS: {
			analyzePath: (p) => ({ exists: known.has(p) }),
			mkdir: (p) => {
				ops.push({ op: "mkdir", path: p });
				known.add(p);
			},
			unlink: (p) => {
				ops.push({ op: "unlink", path: p });
				known.delete(p);
			},
			writeFile: (p, data) => {
				ops.push({ op: "writeFile", path: p, size: data.length });
				known.add(p);
			},
		},
		getSaveFilePath: () => path,
		loadSaveFiles: () => {
			ops.push({ op: "loadSaveFiles" });
		},
	};

	FP.restoreSaveToFS(gm, buf);

	return { ops, known };
}

describe("SRAM restore: directory creation logic", () => {
	it("creates intermediate directories for typical path /data/saves/game.srm", () => {
		const { ops } = simulateRestore(
			"/data/saves/game.srm",
			[],
			new ArrayBuffer(16),
		);
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir").map((o) => o.path),
			["/data", "/data/saves"],
		);
	});

	it("skips directory creation when directories already exist", () => {
		const { ops } = simulateRestore(
			"/data/saves/game.srm",
			["/data", "/data/saves"],
			new ArrayBuffer(16),
		);
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir"),
			[],
		);
	});

	it("unlinks existing save file before writing", () => {
		const { ops } = simulateRestore(
			"/data/saves/game.srm",
			["/data", "/data/saves", "/data/saves/game.srm"],
			new ArrayBuffer(16),
		);
		const unlinkOps = ops.filter((o) => o.op === "unlink");
		assert.equal(unlinkOps.length, 1);
		assert.equal(unlinkOps[0].path, "/data/saves/game.srm");
	});

	it("writes the file and calls loadSaveFiles", () => {
		const { ops } = simulateRestore(
			"/data/saves/game.srm",
			["/data", "/data/saves"],
			new ArrayBuffer(32),
		);
		const writeOps = ops.filter((o) => o.op === "writeFile");
		assert.equal(writeOps.length, 1);
		assert.equal(writeOps[0].path, "/data/saves/game.srm");
		assert.equal(writeOps[0].size, 32);

		const loadOps = ops.filter((o) => o.op === "loadSaveFiles");
		assert.equal(loadOps.length, 1);
	});

	it("creates all intermediate directories for deeply nested path", () => {
		const { ops } = simulateRestore(
			"/a/b/c/d/e/game.srm",
			[],
			new ArrayBuffer(8),
		);
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir").map((o) => o.path),
			["/a", "/a/b", "/a/b/c", "/a/b/c/d", "/a/b/c/d/e"],
		);
	});
});

describe("SRAM restore: edge case — path with no slashes", () => {
	// If getSaveFilePath() returns a bare filename (no slashes), e.g. "game.srm",
	// the split produces ["game.srm"]. The loop runs 0 iterations (length-1 = 0).
	// No directories are created. The file is written at "game.srm" (a relative
	// path in the virtual FS).
	it("handles path with no slashes gracefully", () => {
		const { ops } = simulateRestore("game.srm", [], new ArrayBuffer(8));
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir"),
			[],
		);
		assert.equal(ops.filter((o) => o.op === "writeFile").length, 1);
		assert.equal(ops[ops.length - 2].path, "game.srm");
	});
});

describe("SRAM restore: edge case — empty string path", () => {
	// If getSaveFilePath() returns "", split("/") yields [""].
	// The loop runs 0 iterations. writeFile is called with path "".
	it("handles empty path without crashing", () => {
		const { ops } = simulateRestore("", [], new ArrayBuffer(8));
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir"),
			[],
		);
		// writeFile is still called with empty path
		assert.equal(ops.filter((o) => o.op === "writeFile").length, 1);
		assert.equal(ops[ops.length - 2].path, "");
	});
});

describe("SRAM restore: edge case — root path", () => {
	// Path "/" splits into ["", ""], loop runs 1 iteration, skips empty part.
	// No mkdir is called. writeFile is called with "/".
	it("handles root path without creating directories", () => {
		const { ops } = simulateRestore("/", [], new ArrayBuffer(8));
		assert.deepEqual(
			ops.filter((o) => o.op === "mkdir"),
			[],
		);
	});
});

describe("SRAM restore: edge case — trailing slash in path", () => {
	// If getSaveFilePath() returns "/data/saves/", split produces
	// ["", "data", "saves", ""]. parts.length - 1 = 3, so the loop runs
	// over indices 0, 1, 2. Index 0 is "" (skipped), index 1 is "data",
	// index 2 is "saves". So mkdir is called for /data and /data/saves.
	// Then writeFile is called with "/data/saves/" which treats the
	// directory itself as a file path.
	it("treats trailing-slash path as a file, writing to directory path", () => {
		const { ops } = simulateRestore("/data/saves/", [], new ArrayBuffer(8));
		const writeOps = ops.filter((o) => o.op === "writeFile");
		assert.equal(writeOps.length, 1);
		assert.equal(writeOps[0].path, "/data/saves/");
	});
});
