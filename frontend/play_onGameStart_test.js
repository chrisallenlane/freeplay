// Tests for the SRAM-restore + periodic-save-handler-registration logic
// inside play.js's EJS_onGameStart hook and the parallel EJS_loadStateURL
// HEAD probe.
//
// These tests don't load play.js itself — that file is tightly coupled to
// EmulatorJS and the DOM. Instead they replay the exact control flow
// against injectable fetch/gameManager stubs. When the logic is later
// extracted into a pure helper, these assertions transfer directly:
// they assert the OBSERVABLE outcome (was the periodic save handler
// registered? was a save URL set?), not the call shape.
//
// Post-fix contract under test:
//   - 2xx: restore + register periodic save.
//   - 404: legitimate "no save on disk"; register periodic save so the
//     user's first session is captured.
//   - other non-2xx (5xx, 503, 403, …): server cannot confirm absence;
//     refuse to register so the next auto-save tick can't overwrite a
//     still-on-disk-but-unreadable real save. Log via console.error.
//   - fetch threw: same — don't register, log.
// Same shape applies to the EJS_loadStateURL HEAD probe.

const { describe, it, beforeEach, afterEach } = require("node:test");
const assert = require("node:assert/strict");

require("./sram.js");
const FP = globalThis.Freeplay;

let originalError;
let errorLog;

beforeEach(() => {
	originalError = console.error;
	errorLog = [];
	console.error = (...args) => errorLog.push(args);
});

afterEach(() => {
	console.error = originalError;
});

// Builds a stub gameManager that records FS ops and emulator.on("saveSaveFiles", …)
// registrations. Mirrors the real EmulatorJS surface that play.js touches.
function makeEmulator(savePath = "/data/saves/game.srm") {
	const ops = [];
	const handlers = [];
	const known = new Set();
	const gameManager = {
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
		getSaveFilePath: () => savePath,
		loadSaveFiles: () => ops.push({ op: "loadSaveFiles" }),
	};
	const emulator = {
		gameManager,
		on: (event, cb) => handlers.push({ event, cb }),
	};
	return { emulator, ops, handlers };
}

// Replays play.js's EJS_onGameStart hook. Every branch and side effect
// mirrors the production code. Returns whether the periodic save
// handler ended up registered, plus the FS op log so we can assert on
// what was restored.
async function runOnGameStart({
	emulator,
	fetchImpl,
	saveBase = "/api/saves/NES/Game",
}) {
	let sramHandlerRegistered = false;
	let safeToRegister = false;

	try {
		const res = await fetchImpl(`${saveBase}/sram`);
		if (res.ok) {
			const buf = await res.arrayBuffer();
			FP.restoreSaveToFS(emulator.gameManager, buf);
			safeToRegister = true;
		} else if (res.status === 404) {
			safeToRegister = true;
		} else {
			console.error(
				"SRAM restore: server returned non-2xx, non-404; " +
					"refusing to register periodic save to avoid " +
					"overwriting an existing on-disk save",
				res.status,
			);
		}
	} catch (err) {
		console.error("SRAM restore failed:", err);
	}

	if (safeToRegister && !sramHandlerRegistered) {
		sramHandlerRegistered = true;
		emulator.on("saveSaveFiles", (data) => {
			// Real handler calls FP.postSave; we just record registration.
			void data;
		});
	}

	return { sramHandlerRegistered };
}

// Replays the EJS_loadStateURL HEAD probe.
async function runStateProbe({ fetchImpl, saveBase = "/api/saves/NES/Game" }) {
	let loadStateURL = null;
	try {
		const res = await fetchImpl(`${saveBase}/state`, { method: "HEAD" });
		if (res.ok) {
			loadStateURL = `${saveBase}/state`;
		} else if (res.status !== 404) {
			console.error(
				"save-state probe: server returned non-2xx, non-404; " +
					"skipping state load",
				res.status,
			);
		}
	} catch (err) {
		console.error("save-state probe failed:", err);
	}
	return { loadStateURL };
}

function fakeFetchStatus(status, body = null) {
	const calls = [];
	const fetchImpl = (url, init) => {
		calls.push({ url, init });
		return Promise.resolve({
			ok: status >= 200 && status < 300,
			status,
			arrayBuffer: () => Promise.resolve(body ?? new ArrayBuffer(0)),
		});
	};
	fetchImpl.calls = calls;
	return fetchImpl;
}

function fakeFetchReject(err) {
	const calls = [];
	const fetchImpl = (url, init) => {
		calls.push({ url, init });
		return Promise.reject(err);
	};
	fetchImpl.calls = calls;
	return fetchImpl;
}

describe("EJS_onGameStart: SRAM restore on 200 OK with real bytes", () => {
	it("writes the buffer to the FS and registers the periodic save handler", async () => {
		const { emulator, ops, handlers } = makeEmulator();
		const fetchImpl = fakeFetchStatus(200, new Uint8Array([1, 2, 3, 4]).buffer);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		// Save was restored to the FS.
		const writes = ops.filter((o) => o.op === "writeFile");
		assert.equal(writes.length, 1);
		assert.equal(writes[0].size, 4);
		// Periodic save handler is registered exactly once.
		assert.equal(sramHandlerRegistered, true);
		assert.equal(handlers.length, 1);
		assert.equal(handlers[0].event, "saveSaveFiles");
	});
});

describe("EJS_onGameStart: SRAM restore — non-2xx response handling", () => {
	// 404 is the "no save on disk" signal. Registering the periodic save
	// handler is correct in this case: the user is starting fresh and we
	// want their progress saved going forward.
	it("404 (no save): writes nothing, registers the periodic save handler", async () => {
		const { emulator, ops } = makeEmulator();
		const fetchImpl = fakeFetchStatus(404);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		assert.equal(ops.filter((o) => o.op === "writeFile").length, 0);
		assert.equal(sramHandlerRegistered, true);
	});

	// 500 (server error / unreadable file per #47) must NOT register
	// the periodic save. If it did, the next auto-save tick would
	// overwrite the still-on-disk-but-unreadable real save. The
	// handler also logs so operators can correlate against the
	// server-side slog.Warn from #47's handleGetSave 5xx path.
	it("500 server error: does NOT register periodic save and logs", async () => {
		const { emulator, ops } = makeEmulator();
		const fetchImpl = fakeFetchStatus(500);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		assert.equal(ops.filter((o) => o.op === "writeFile").length, 0);
		assert.equal(
			sramHandlerRegistered,
			false,
			"500 must not register the periodic save — it could overwrite a real save",
		);
		assert.equal(errorLog.length, 1);
		assert.match(errorLog[0][0], /SRAM restore.*non-2xx, non-404/);
		assert.equal(errorLog[0][1], 500);
	});

	// Same shape as 500 but for 403 — permission-denied (#47's
	// "exists but unreadable" surface).
	it("403 permission-denied: does NOT register periodic save and logs", async () => {
		const { emulator } = makeEmulator();
		const fetchImpl = fakeFetchStatus(403);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		assert.equal(sramHandlerRegistered, false);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][1], 403);
	});

	// 502/503/504 — proxy errors. Treated as transient; no registration.
	it("503 service-unavailable: does NOT register periodic save and logs", async () => {
		const { emulator } = makeEmulator();
		const fetchImpl = fakeFetchStatus(503);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		assert.equal(sramHandlerRegistered, false);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][1], 503);
	});
});

describe("EJS_onGameStart: SRAM restore — network failure handling", () => {
	// Network errors log AND skip registration — same overwrite hazard
	// as the 5xx case if the handler were allowed to register.
	it("network failure: logs and does NOT register periodic save", async () => {
		const { emulator } = makeEmulator();
		const err = new TypeError("network down");
		const fetchImpl = fakeFetchReject(err);

		const { sramHandlerRegistered } = await runOnGameStart({
			emulator,
			fetchImpl,
		});

		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "SRAM restore failed:");
		assert.equal(errorLog[0][1], err);

		assert.equal(
			sramHandlerRegistered,
			false,
			"network failure must skip registration — server state unknown",
		);
	});
});

describe("EJS_loadStateURL HEAD probe (play.js:87-94)", () => {
	it("200: sets EJS_loadStateURL to the state endpoint", async () => {
		const fetchImpl = fakeFetchStatus(200);
		const { loadStateURL } = await runStateProbe({ fetchImpl });
		assert.equal(loadStateURL, "/api/saves/NES/Game/state");
	});

	it("404 (no state): leaves EJS_loadStateURL unset", async () => {
		const fetchImpl = fakeFetchStatus(404);
		const { loadStateURL } = await runStateProbe({ fetchImpl });
		assert.equal(loadStateURL, null);
	});

	// Post-fix: 5xx leaves loadStateURL unset AND emits a console.error
	// so the failure isn't invisible. Same shape as the SRAM probe.
	it("500 server error: leaves URL unset AND logs", async () => {
		const fetchImpl = fakeFetchStatus(500);
		const { loadStateURL } = await runStateProbe({ fetchImpl });
		assert.equal(loadStateURL, null);
		assert.equal(errorLog.length, 1);
		assert.match(errorLog[0][0], /save-state probe.*non-2xx, non-404/);
		assert.equal(errorLog[0][1], 500);
	});

	it("network failure: leaves URL unset AND logs (catch is no longer empty)", async () => {
		const err = new TypeError("network down");
		const fetchImpl = fakeFetchReject(err);
		const { loadStateURL } = await runStateProbe({ fetchImpl });
		assert.equal(loadStateURL, null);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "save-state probe failed:");
		assert.equal(errorLog[0][1], err);
	});
});
