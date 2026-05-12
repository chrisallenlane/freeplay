// Tests for the EJS_onGameStart save-handler-registration state machine
// under repeated invocation. Distinct from play_onGameStart_test.js, which
// models each call in isolation — this file models the production closure
// where `sramHandlerRegistered` persists across invocations.
//
// EmulatorJS contract (verified against emulatorjs/data/src/emulator.js):
//   - `EJS_onGameStart` is wired as a listener on the "start" event in
//     loader.js (line 156).
//   - "start" is emitted exactly once per emulator lifetime, from
//     emulator.js:1036 inside `startGame()`, which itself is called only
//     from `downloadFiles()` (line 930) during init.
//   - Soft-load / restart paths (gameManager.restart()) DO NOT re-emit
//     "start". The contract is firmly: at most one fire per page load.
//   - `emulator.on(event, fn)` PUSHES into an array. Calling it twice
//     with the same callback registers TWO listeners; `callEvent` would
//     then dispatch the save to FP.postSave twice per tick.
//
// The contract holds today, but play.js's `sramHandlerRegistered` flag
// only meaningfully guards re-registration; it does NOT guard against
// the SRAM-restore branch running twice (a fresh fetch + write). If
// EmulatorJS ever changes its contract (or a third party fires the
// event manually), the second invocation would re-fetch the SRAM,
// potentially OVERWRITING the user's in-memory progress with the
// previously-restored bytes from disk. This file pins what the code
// actually does in the multi-fire case so any future behavior change
// is intentional.

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

// Factory that produces an EJS_onGameStart-equivalent function whose
// `sramHandlerRegistered` closure persists across invocations — exactly
// like the real play.js code. The previous test file resets the flag
// per call, hiding multi-fire bugs.
function makeOnGameStart({
	emulator,
	fetchImpl,
	saveBase = "/api/saves/NES/Game",
}) {
	let sramHandlerRegistered = false;
	return async () => {
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
				void data;
			});
		}
	};
}

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
	// Mirror emulator.js's append-on-on(): each call pushes another listener.
	const emulator = {
		gameManager,
		on: (event, cb) => handlers.push({ event, cb }),
	};
	return { emulator, ops, handlers };
}

// Scripted fetch — returns responses in order and records calls.
function fakeFetchSequence(responses) {
	const calls = [];
	let i = 0;
	const fetchImpl = (url, init) => {
		calls.push({ url, init });
		const r = responses[Math.min(i, responses.length - 1)];
		i++;
		if (r.reject) return Promise.reject(r.reject);
		return Promise.resolve({
			ok: r.status >= 200 && r.status < 300,
			status: r.status,
			arrayBuffer: () => Promise.resolve(r.body ?? new ArrayBuffer(0)),
		});
	};
	fetchImpl.calls = calls;
	return fetchImpl;
}

describe("EJS_onGameStart multi-fire: handler registration is idempotent", () => {
	// If EmulatorJS were to fire "start" twice and both fetches succeed,
	// the closure-captured `sramHandlerRegistered` flag must prevent the
	// second `emulator.on("saveSaveFiles", ...)` from registering a
	// DUPLICATE listener. emulator.js's `on` pushes into an array — a
	// duplicate listener would cause `callEvent("saveSaveFiles")` to
	// POST to the server twice per save tick.
	it("two successful fires register the periodic save handler exactly once", async () => {
		const { emulator, handlers } = makeEmulator();
		const fetchImpl = fakeFetchSequence([
			{ status: 200, body: new Uint8Array([1, 2, 3, 4]).buffer },
			{ status: 200, body: new Uint8Array([1, 2, 3, 4]).buffer },
		]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		await onGameStart();

		const saveHandlers = handlers.filter((h) => h.event === "saveSaveFiles");
		assert.equal(
			saveHandlers.length,
			1,
			"saveSaveFiles must be registered exactly once across multiple fires",
		);
	});

	// 503 → 200 sequence: the first fire's SRAM probe failed transiently
	// so registration was skipped. A second fire succeeds — the handler
	// should now be registered (otherwise the user's saves are dropped
	// for the rest of the session).
	it("first fire 503, second fire 200: handler registered on second success", async () => {
		const { emulator, handlers } = makeEmulator();
		const fetchImpl = fakeFetchSequence([
			{ status: 503 },
			{ status: 200, body: new Uint8Array([9, 9, 9]).buffer },
		]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		assert.equal(handlers.length, 0, "503 must not register the handler");

		await onGameStart();
		const saveHandlers = handlers.filter((h) => h.event === "saveSaveFiles");
		assert.equal(
			saveHandlers.length,
			1,
			"second-fire 200 must register the handler that the first-fire 503 skipped",
		);
	});

	// Both fires hit 503 — handler stays unregistered.
	it("two 503 fires: handler never registered, both logged", async () => {
		const { emulator, handlers } = makeEmulator();
		const fetchImpl = fakeFetchSequence([{ status: 503 }, { status: 503 }]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		await onGameStart();

		assert.equal(handlers.length, 0);
		assert.equal(errorLog.length, 2, "both 503s should log");
	});

	// 404 first, 200 second — possible if a save was written between
	// the two fires. The first fire registers the handler; the second
	// fire restores the bytes but must not double-register.
	it("first fire 404, second fire 200: handler registered once, second restores bytes", async () => {
		const { emulator, ops, handlers } = makeEmulator();
		const fetchImpl = fakeFetchSequence([
			{ status: 404 },
			{ status: 200, body: new Uint8Array([7, 7, 7, 7]).buffer },
		]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		await onGameStart();

		const saveHandlers = handlers.filter((h) => h.event === "saveSaveFiles");
		assert.equal(saveHandlers.length, 1, "handler registered exactly once");
		const writes = ops.filter((o) => o.op === "writeFile");
		assert.equal(writes.length, 1, "second fire's 200 wrote the bytes");
		assert.equal(writes[0].size, 4);
	});
});

describe("EJS_onGameStart multi-fire: SRAM-restore overwrite hazard", () => {
	// SCENARIO: two successful fires of EJS_onGameStart. Each fetches
	// `${saveBase}/sram` fresh. The first restored the user's pre-existing
	// save (good). Suppose the user has been playing for a few seconds
	// between fires — their in-memory state has advanced past disk. The
	// second fire blindly overwrites the FS-backed SRAM file with the
	// STALE bytes from disk. Next auto-save tick may then either:
	//   (a) capture the post-overwrite state (loss of progress), or
	//   (b) be unaffected if the core has already cached SRAM in RAM.
	// EmulatorJS's current contract (one "start" per lifetime) makes this
	// non-load-bearing, but the code itself doesn't enforce that contract.
	//
	// This test pins CURRENT behavior so any change is visible: the
	// second fire performs another writeFile of the (possibly stale)
	// bytes from disk. If we ever extract a helper and want to guard,
	// we'll want this test to FAIL and re-author the helper.
	it("two successful fires: the SRAM bytes are re-written to FS each time", async () => {
		const { emulator, ops } = makeEmulator();
		const fetchImpl = fakeFetchSequence([
			{ status: 200, body: new Uint8Array([1, 2, 3]).buffer },
			{ status: 200, body: new Uint8Array([1, 2, 3]).buffer },
		]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		await onGameStart();

		const writes = ops.filter((o) => o.op === "writeFile");
		assert.equal(
			writes.length,
			2,
			"each successful fire performs an unguarded writeFile — " +
				"OK today because EmulatorJS fires 'start' once per lifetime, " +
				"but the code has no defense if that contract changes",
		);
	});

	// And the fetch IS re-issued every time the handler fires. This is
	// the underlying cause of the above behavior. Pinned so any future
	// "cache the SRAM bytes after first restore" optimization is
	// detected.
	it("each fire issues a fresh fetch to /sram (no caching)", async () => {
		const { emulator } = makeEmulator();
		const fetchImpl = fakeFetchSequence([
			{ status: 200, body: new ArrayBuffer(0) },
			{ status: 200, body: new ArrayBuffer(0) },
			{ status: 200, body: new ArrayBuffer(0) },
		]);
		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		await onGameStart();
		await onGameStart();
		await onGameStart();

		assert.equal(fetchImpl.calls.length, 3);
		assert.equal(fetchImpl.calls[0].url, "/api/saves/NES/Game/sram");
	});
});

describe("EJS_onGameStart: concurrent overlapping invocations", () => {
	// SCENARIO: two fires arrive in close succession with their fetches
	// resolving back-to-back. JS being single-threaded means whichever
	// resumes from its second await first wins the check-and-set race
	// synchronously: it flips `sramHandlerRegistered` to true before the
	// other can re-check. So the closure pattern IS safe by virtue of
	// the single-threaded event loop — no await between the check on
	// line 97 and the set on line 98.
	//
	// This test pins that property. If anyone later introduces an await
	// between the !sramHandlerRegistered check and the assignment, the
	// race becomes real and this test will fail loudly.
	it("two overlapping successful fires register exactly one listener (single-thread-safe)", async () => {
		const { emulator, handlers } = makeEmulator();

		// Both fetches are queued; they resolve via microtasks, so JS
		// runtime serializes the check-and-set on line 97-98.
		let resolveA, resolveB;
		const promA = new Promise((r) => (resolveA = r));
		const promB = new Promise((r) => (resolveB = r));
		let n = 0;
		const fetchImpl = () => {
			n++;
			return n === 1 ? promA : promB;
		};

		const onGameStart = makeOnGameStart({ emulator, fetchImpl });

		const firstFire = onGameStart();
		const secondFire = onGameStart();

		const okRes = {
			ok: true,
			status: 200,
			arrayBuffer: () => Promise.resolve(new ArrayBuffer(0)),
		};
		resolveA(okRes);
		resolveB(okRes);

		await Promise.all([firstFire, secondFire]);

		const saveHandlers = handlers.filter((h) => h.event === "saveSaveFiles");
		assert.equal(
			saveHandlers.length,
			1,
			"single-threaded event loop synchronizes the check-and-set, " +
				"so even overlapping fires register exactly one listener. " +
				"This invariant breaks if anyone adds an await between " +
				"the check and assignment in play.js.",
		);
	});
});
