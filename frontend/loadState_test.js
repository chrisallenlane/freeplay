// Tests for loadState.js's loadStateFromServer — the EJS_onLoadState
// handler that restores a save state from the server when the bottom-bar
// "load state" button is pressed.
//
// The regression this pins: before this handler existed, the load button
// fell through to EmulatorJS's built-in browser-storage path, which
// calls gameManager.loadState(undefined) for a server-resident state and
// aborts the WASM runtime. The contract here is that a server load never
// hands the emulator undefined or zero-length data, and that every
// non-happy branch surfaces a user message instead of a dead page.
//
// Branch contract (mirrors the SRAM restore / HEAD probe in play.js):
//   - 2xx with bytes: apply the state, notify success.
//   - 2xx zero-length: apply nothing, notify "no state".
//   - 404: apply nothing, notify "no state".
//   - other non-2xx: apply nothing, notify failure, log.
//   - fetch threw: apply nothing, notify failure, log.
//   - loadState threw: notify failure, log.

const { describe, it, beforeEach, afterEach } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./loadState.js");

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

// Records the bytes handed to loadState so tests can assert on them.
function makeGM({ throwOnLoad = false } = {}) {
	const loads = [];
	const gm = {
		loadState: (data) => {
			loads.push(data);
			if (throwOnLoad) throw new Error("emulator rejected state");
		},
	};
	return { gm, loads };
}

// Records notify() messages.
function makeNotify() {
	const messages = [];
	const notify = (msg) => messages.push(msg);
	notify.messages = messages;
	return notify;
}

// Fake fetch resolving to a given status + body (ArrayBuffer).
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

describe("loadStateFromServer: happy path", () => {
	it("2xx with bytes: loads the state and notifies success", async () => {
		const { gm, loads } = makeGM();
		const notify = makeNotify();
		const fetchImpl = fakeFetchStatus(200, new Uint8Array([1, 2, 3, 4]).buffer);

		await FP.loadStateFromServer("/api/saves/SNES/Game", gm, notify, fetchImpl);

		assert.equal(fetchImpl.calls.length, 1);
		assert.equal(fetchImpl.calls[0].url, "/api/saves/SNES/Game/state");
		assert.equal(loads.length, 1);
		assert.ok(loads[0] instanceof Uint8Array);
		assert.deepEqual(Array.from(loads[0]), [1, 2, 3, 4]);
		assert.deepEqual(notify.messages, ["Loaded save state"]);
		assert.equal(errorLog.length, 0);
	});
});

describe("loadStateFromServer: nothing to load", () => {
	// The exact undefined/empty inputs that aborted the WASM runtime must
	// never reach gameManager.loadState.
	it("2xx zero-length body: does NOT call loadState, notifies no-state", async () => {
		const { gm, loads } = makeGM();
		const notify = makeNotify();
		const fetchImpl = fakeFetchStatus(200, new ArrayBuffer(0));

		await FP.loadStateFromServer("/x", gm, notify, fetchImpl);

		assert.equal(loads.length, 0);
		assert.deepEqual(notify.messages, ["No save state to load"]);
	});

	it("404: does NOT call loadState, notifies no-state", async () => {
		const { gm, loads } = makeGM();
		const notify = makeNotify();
		const fetchImpl = fakeFetchStatus(404);

		await FP.loadStateFromServer("/x", gm, notify, fetchImpl);

		assert.equal(loads.length, 0);
		assert.deepEqual(notify.messages, ["No save state to load"]);
		assert.equal(errorLog.length, 0);
	});
});

describe("loadStateFromServer: server / network failures", () => {
	it("500: does NOT call loadState, notifies failure, logs status", async () => {
		const { gm, loads } = makeGM();
		const notify = makeNotify();
		const fetchImpl = fakeFetchStatus(500);

		await FP.loadStateFromServer("/x", gm, notify, fetchImpl);

		assert.equal(loads.length, 0);
		assert.deepEqual(notify.messages, ["Could not load save state"]);
		assert.equal(errorLog.length, 1);
		assert.match(errorLog[0][0], /load-state.*non-2xx, non-404/);
		assert.equal(errorLog[0][1], 500);
	});

	it("network failure: does NOT call loadState, notifies failure, logs", async () => {
		const { gm, loads } = makeGM();
		const notify = makeNotify();
		const err = new TypeError("network down");
		const fetchImpl = fakeFetchReject(err);

		await FP.loadStateFromServer("/x", gm, notify, fetchImpl);

		assert.equal(loads.length, 0);
		assert.deepEqual(notify.messages, ["Could not load save state"]);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "load-state failed:");
		assert.equal(errorLog[0][1], err);
	});
});

describe("loadStateFromServer: emulator rejects the state", () => {
	// Even with valid bytes, a corrupt/foreign state could make the core
	// throw. We surface a message rather than leak an unhandled rejection.
	it("loadState throws: notifies failure and logs", async () => {
		const { gm, loads } = makeGM({ throwOnLoad: true });
		const notify = makeNotify();
		const fetchImpl = fakeFetchStatus(200, new Uint8Array([9, 9]).buffer);

		await FP.loadStateFromServer("/x", gm, notify, fetchImpl);

		// loadState was attempted (recorded) but threw.
		assert.equal(loads.length, 1);
		assert.deepEqual(notify.messages, ["Could not load save state"]);
		assert.equal(errorLog.length, 1);
		assert.match(errorLog[0][0], /emulator rejected the state/);
	});
});
