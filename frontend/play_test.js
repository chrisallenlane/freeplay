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

describe("postSave: data guard behavior", () => {
	// NOTE: These tests verify JavaScript language semantics, not application
	// code. They are intentional documentation tests that record the truthiness
	// assumptions the postSave guard (`if (data) fetch(...)`) relies on. They
	// will pass regardless of any changes to play.js and cannot detect
	// regressions in postSave itself.
	//
	// The postSave function (lines 75-82) has this guard:
	//   if (data) fetch(...)
	// This means falsy values prevent the save. The tests below document
	// which values are falsy vs. truthy and what that means for postSave.

	it("null is falsy — save skipped correctly", () => {
		assert.ok(!null);
	});

	it("undefined is falsy — save skipped correctly", () => {
		assert.ok(!undefined);
	});

	it("empty Uint8Array is truthy — save still sent", () => {
		// An empty Uint8Array (length 0) is truthy in JavaScript.
		// This means postSave will POST zero bytes to the server,
		// potentially overwriting valid save data with nothing.
		assert.ok(new Uint8Array([]));
	});

	it("zero-length ArrayBuffer is truthy — save still sent", () => {
		assert.ok(new ArrayBuffer(0));
	});
});

describe("SRAM save: duplicate save on manual button click", () => {
	// NOTE: This test documents a known bug in EmulatorJS behavior using a
	// local simulation. It does NOT exercise play.js code and cannot detect
	// if the bug is fixed or regresses. It exists to record the upstream
	// EmulatorJS call sequence that causes duplicate POSTs on manual save.
	//
	// When the user clicks "Save SRAM" in EmulatorJS:
	//
	// 1. emulator.js calls: const file = await this.gameManager.getSaveFile()
	// 2. getSaveFile() calls this.saveSaveFiles() (GameManager.js line 427)
	// 3. saveSaveFiles() emits "saveSaveFiles" event (GameManager.js line 413)
	// 4. play.js "saveSaveFiles" handler (line 119-121) calls postSave("sram", data)
	//    => FIRST POST to server
	// 5. Back in emulator.js, callEvent("saveSave", {save: file}) fires
	// 6. EJS_onSaveSave (line 87-89) calls postSave("sram", data.save)
	//    => SECOND POST to server
	//
	// Result: every manual save click sends SRAM data to server twice.
	it("demonstrates that manual save triggers two postSave calls", () => {
		const postCalls = [];

		function postSave(type, data) {
			if (data) postCalls.push({ type, size: data.length });
		}

		const sramData = new Uint8Array([1, 2, 3, 4]);

		// Step 4: "saveSaveFiles" event fires (from getSaveFile -> saveSaveFiles)
		const onSaveSaveFiles = (data) => postSave("sram", data);
		onSaveSaveFiles(sramData);

		// Step 6: "saveSave" event fires (from emulator.js after getSaveFile returns)
		const onSaveSave = (data) => postSave("sram", data.save);
		onSaveSave({ save: sramData });

		// Both fire, resulting in two POSTs for a single user action
		assert.equal(postCalls.length, 2);
	});
});

describe("SRAM save: duplicate handler registration on game restart", () => {
	// NOTE: This test documents a known bug in EmulatorJS behavior using a
	// local simulation. It does NOT exercise play.js code and cannot detect
	// if the bug is fixed or regresses. It exists to record the root cause
	// of duplicate POSTs when games are restarted within the same page load.
	//
	// EJS_onGameStart registers a "saveSaveFiles" event handler each time
	// it's called (play.js line 119). EmulatorJS's emulator.on() appends
	// to an array (emulator.js lines 407-410):
	//   on(event, func) {
	//       if (!this.functions) this.functions = {};
	//       if (!Array.isArray(this.functions[event])) this.functions[event] = [];
	//       this.functions[event].push(func);
	//   }
	//
	// If EJS_onGameStart fires multiple times (e.g., game restart within
	// the same page), handlers accumulate, causing duplicate POSTs for
	// each save event.
	it("accumulates handlers across multiple game starts", () => {
		const handlers = [];
		const emulator = {
			on: (event, fn) => {
				handlers.push({ event, fn });
			},
		};

		// Simulate three game starts
		for (let i = 0; i < 3; i++) {
			emulator.on("saveSaveFiles", () => {});
		}

		assert.equal(handlers.length, 3);

		// All three handlers would fire on a single save event,
		// resulting in three POSTs to the server.
		let postCalls = 0;
		for (const handler of handlers) {
			handler.fn();
			postCalls++;
		}
		assert.equal(postCalls, 3);
	});
});

describe("SRAM save: exit triggers double saveSaveFiles", () => {
	// NOTE: This test documents a known bug in EmulatorJS behavior using a
	// local simulation. It does NOT exercise play.js code and cannot detect
	// if the bug is fixed or regresses. It exists to record the upstream
	// EmulatorJS call sequence that causes duplicate POSTs on exit.
	//
	// On exit, GameManager.js calls saveSaveFiles() TWICE (lines 48-50):
	//   this.EJS.on("exit", () => {
	//       if (!this.EJS.failedToStart) {
	//           this.saveSaveFiles();     // First call
	//           this.functions.restart();
	//           this.saveSaveFiles();     // Second call
	//       }
	//       ...
	//   });
	//
	// Each call emits the "saveSaveFiles" event, causing the play.js
	// handler to POST SRAM data twice during a single exit.
	it("demonstrates that exit triggers two saveSaveFiles events", () => {
		let eventsFired = 0;
		const events = [];

		// Simulate the event emission for each saveSaveFiles() call
		function simulateSaveSaveFiles() {
			eventsFired++;
			events.push("saveSaveFiles");
		}

		// Simulate exit sequence (GameManager.js lines 47-51)
		const failedToStart = false;
		if (!failedToStart) {
			simulateSaveSaveFiles(); // line 48
			// this.functions.restart() happens here
			simulateSaveSaveFiles(); // line 50
		}

		assert.equal(eventsFired, 2);
		assert.deepEqual(events, ["saveSaveFiles", "saveSaveFiles"]);
	});
});
