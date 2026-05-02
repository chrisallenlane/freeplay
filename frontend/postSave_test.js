const { describe, it, beforeEach, afterEach } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./postSave.js");

// Records calls to console.error so tests can assert what postSave logged.
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

// Builds a fake fetch that resolves to a Response-like object with the
// given status. ok mirrors res.ok semantics (status 200-299).
function fakeFetchOK(status = 200) {
	const calls = [];
	const fetchImpl = (url, init) => {
		calls.push({ url, init });
		return Promise.resolve({ ok: status >= 200 && status < 300, status });
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

describe("postSave", () => {
	it("POSTs to saveBase + type with the X-Requested-With CSRF header", async () => {
		const fetchImpl = fakeFetchOK(200);
		await FP.postSave("/api/saves/NES/Mega Man", "sram", "data", fetchImpl);
		assert.equal(fetchImpl.calls.length, 1);
		assert.equal(fetchImpl.calls[0].url, "/api/saves/NES/Mega Man/sram");
		assert.equal(fetchImpl.calls[0].init.method, "POST");
		assert.equal(
			fetchImpl.calls[0].init.headers["X-Requested-With"],
			"freeplay",
		);
	});

	it("returns the Response on success and logs nothing", async () => {
		const fetchImpl = fakeFetchOK(200);
		const res = await FP.postSave("/x", "sram", "data", fetchImpl);
		assert.equal(res.status, 200);
		assert.equal(errorLog.length, 0);
	});

	it("logs HTTP status on non-2xx responses", async () => {
		const fetchImpl = fakeFetchOK(404);
		const res = await FP.postSave("/x", "sram", "data", fetchImpl);
		assert.equal(res.status, 404);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "Save failed (sram): HTTP 404");
	});

	it("logs HTTP status on 500 server errors", async () => {
		const fetchImpl = fakeFetchOK(500);
		await FP.postSave("/x", "state", "data", fetchImpl);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "Save failed (state): HTTP 500");
	});

	it("logs the error and returns undefined on network rejection", async () => {
		const err = new TypeError("network down");
		const fetchImpl = fakeFetchReject(err);
		const res = await FP.postSave("/x", "sram", "data", fetchImpl);
		assert.equal(res, undefined);
		assert.equal(errorLog.length, 1);
		assert.equal(errorLog[0][0], "Save failed (sram):");
		assert.equal(errorLog[0][1], err);
	});

	it("skips the fetch and returns undefined when data is null", async () => {
		const fetchImpl = fakeFetchOK();
		const res = FP.postSave("/x", "sram", null, fetchImpl);
		assert.equal(res, undefined);
		assert.equal(fetchImpl.calls.length, 0);
		assert.equal(errorLog.length, 0);
	});

	it("skips the fetch when data is undefined", async () => {
		const fetchImpl = fakeFetchOK();
		FP.postSave("/x", "sram", undefined, fetchImpl);
		assert.equal(fetchImpl.calls.length, 0);
	});

	it("skips the fetch for an empty Uint8Array (don't overwrite saves with zero bytes)", async () => {
		const fetchImpl = fakeFetchOK();
		const res = FP.postSave("/x", "sram", new Uint8Array([]), fetchImpl);
		assert.equal(res, undefined);
		assert.equal(fetchImpl.calls.length, 0);
	});

	it("skips the fetch for a zero-length ArrayBuffer", async () => {
		const fetchImpl = fakeFetchOK();
		FP.postSave("/x", "sram", new ArrayBuffer(0), fetchImpl);
		assert.equal(fetchImpl.calls.length, 0);
	});

	it("posts a non-empty Uint8Array", async () => {
		const fetchImpl = fakeFetchOK(200);
		await FP.postSave("/x", "sram", new Uint8Array([1, 2, 3]), fetchImpl);
		assert.equal(fetchImpl.calls.length, 1);
	});
});
