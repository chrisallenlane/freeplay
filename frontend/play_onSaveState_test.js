// Tests for the EJS_onSaveState callback (play.js:58-60) and the parallel
// saveSaveFiles handler (play.js:80-82). Both forward EmulatorJS event
// payloads to FP.postSave; this file pins the contract that postSave's
// existing falsy and zero-byteLength guards (postSave.js:12, 17) handle
// the edge cases the EmulatorJS event API can produce in normal
// operation — null, undefined, zero-length buffers — without making any
// further assumptions about save-data shape (per the project's threat
// model: consoles are weird, no content validation at the trust
// boundary).
//
// The file replays the production control flow against injectable stubs
// rather than loading play.js directly (play.js is tightly coupled to
// EmulatorJS and the DOM).

const { describe, it, beforeEach, afterEach } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./postSave.js");

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

// Records calls + lets the test inspect the serialized request body.
function fakeFetchOK(status = 200) {
	const calls = [];
	const fetchImpl = (url, init) => {
		calls.push({ url, init });
		return Promise.resolve({ ok: status >= 200 && status < 300, status });
	};
	fetchImpl.calls = calls;
	return fetchImpl;
}

// Decodes the Blob body that postSave hands fetch.
async function bodyBytes(call) {
	const blob = call.init.body;
	const buf = await blob.arrayBuffer();
	return new Uint8Array(buf);
}

// Replays play.js:58-60: EJS_onSaveState handler.
function onSaveState(saveBase, data, fetchImpl) {
	return FP.postSave(saveBase, "state", data?.state, fetchImpl);
}

// Replays play.js:80-82: saveSaveFiles handler.
function onSaveSaveFiles(saveBase, data, fetchImpl) {
	return FP.postSave(saveBase, "sram", data, fetchImpl);
}

describe("EJS_onSaveState happy path", () => {
	it("forwards data.state (typed array) to postSave with the state endpoint", async () => {
		const fetchImpl = fakeFetchOK(200);
		const evt = {
			screenshot: new Uint8Array([0xff]),
			format: "state",
			state: new Uint8Array([1, 2, 3, 4, 5]),
		};

		await onSaveState("/api/saves/NES/Mega Man", evt, fetchImpl);

		assert.equal(fetchImpl.calls.length, 1);
		assert.equal(fetchImpl.calls[0].url, "/api/saves/NES/Mega Man/state");
		const bytes = await bodyBytes(fetchImpl.calls[0]);
		assert.deepEqual(Array.from(bytes), [1, 2, 3, 4, 5]);
	});
});

describe("EJS_onSaveState payload-shape edge cases", () => {
	// data.state === undefined: postSave's `if (!data) return` catches this.
	it("data.state undefined: skips fetch (postSave falsy-guard fires)", () => {
		const fetchImpl = fakeFetchOK();
		const evt = { screenshot: null, format: "state" };

		const res = onSaveState("/x", evt, fetchImpl);

		assert.equal(res, undefined);
		assert.equal(fetchImpl.calls.length, 0);
	});

	it("data.state null: skips fetch (postSave falsy-guard fires)", () => {
		const fetchImpl = fakeFetchOK();
		const evt = { screenshot: null, format: "state", state: null };

		onSaveState("/x", evt, fetchImpl);

		assert.equal(fetchImpl.calls.length, 0);
	});

	// The optional-chain in our wrapper makes data?.state undefined when
	// the entire event payload is missing, which postSave then skips.
	it("entire event undefined: handler is a no-op", () => {
		const fetchImpl = fakeFetchOK();
		onSaveState("/x", undefined, fetchImpl);
		assert.equal(fetchImpl.calls.length, 0);
	});

	// Empty Uint8Array — postSave.js:17 byteLength guard fires.
	it("data.state empty Uint8Array: guard fires, no fetch", () => {
		const fetchImpl = fakeFetchOK();
		const evt = {
			screenshot: null,
			format: "state",
			state: new Uint8Array(0),
		};

		onSaveState("/x", evt, fetchImpl);

		assert.equal(fetchImpl.calls.length, 0);
	});

	// A typed array of all zeros has byteLength > 0, so the guard does not
	// fire. This is intentional — postSave does not (and per the project's
	// threat model must not) attempt content-based validation.
	it("data.state zero-filled Uint8Array(8192): posts (no content validation)", async () => {
		const fetchImpl = fakeFetchOK(200);
		const evt = {
			screenshot: null,
			format: "state",
			state: new Uint8Array(8192),
		};

		await onSaveState("/x", evt, fetchImpl);

		assert.equal(fetchImpl.calls.length, 1);
		const bytes = await bodyBytes(fetchImpl.calls[0]);
		assert.equal(bytes.byteLength, 8192);
	});

	// An ArrayBuffer with byteLength > 0 passes the guard. Blob handles
	// ArrayBuffer correctly.
	it("data.state ArrayBuffer(16): posts the bytes", async () => {
		const fetchImpl = fakeFetchOK(200);
		const evt = {
			screenshot: null,
			format: "state",
			state: new ArrayBuffer(16),
		};

		await onSaveState("/x", evt, fetchImpl);

		assert.equal(fetchImpl.calls.length, 1);
		const bytes = await bodyBytes(fetchImpl.calls[0]);
		assert.equal(bytes.byteLength, 16);
	});
});

describe("saveSaveFiles handler payload-shape edge cases (play.js:80-82)", () => {
	// Same falsy/empty guard contracts as EJS_onSaveState but on a
	// different callsite. EmulatorJS passes `data` directly (no .state
	// indirection).

	it("data null: postSave falsy-guard fires, no fetch", () => {
		const fetchImpl = fakeFetchOK();
		onSaveSaveFiles("/x", null, fetchImpl);
		assert.equal(fetchImpl.calls.length, 0);
	});

	it("data empty Uint8Array: guard fires, no fetch", () => {
		const fetchImpl = fakeFetchOK();
		onSaveSaveFiles("/x", new Uint8Array(0), fetchImpl);
		assert.equal(fetchImpl.calls.length, 0);
	});
});
