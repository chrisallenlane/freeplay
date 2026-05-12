// Tests for the rescan + cover-status-poll state machine in app.js
// (lines ~383-435).
//
// app.js is an IIFE, so its `statusPollTimer` closure variable cannot be
// accessed directly. Following the same approach used in
// play_onGameStart_test.js and play_onSaveState_test.js, this file
// REPLAYS the exact production control flow against injectable stubs
// (fetch / setTimeout / alert / DOM button). Every branch and side
// effect mirrors the lines linked in the comments below; if app.js's
// flow changes, this harness must change with it.
//
// The contract under test is the BUTTON-RECOVERY invariant: after the
// rescan operation has fully settled, the button must reach an enabled,
// non-"Scanning..." state — regardless of which path the response took
// (success, 409, network failure, scheduled poll, etc.). The user
// should never be locked out of the rescan control by a stale poll
// timer or a swallowed error.

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

// Builds a stub <button> with the same surface app.js's reset/scan
// transitions touch (disabled, textContent, innerHTML, classList).
function makeButton() {
	const classes = new Set();
	return {
		disabled: false,
		textContent: "",
		innerHTML: "",
		classList: {
			add: (c) => classes.add(c),
			remove: (c) => classes.delete(c),
			contains: (c) => classes.has(c),
		},
		_classes: classes,
	};
}

function makeStatusEl() {
	return { textContent: "" };
}

// Stub setTimeout/clearTimeout so tests can drive scheduled callbacks
// deterministically. Mirrors the production "set ID, possibly clear ID"
// shape without involving the real event loop.
function makeTimers() {
	let nextId = 1;
	const scheduled = new Map(); // id -> { cb, delay }
	const cleared = []; // log of IDs passed to clearTimeout
	return {
		setTimeout: (cb, delay) => {
			const id = nextId++;
			scheduled.set(id, { cb, delay });
			return id;
		},
		clearTimeout: (id) => {
			cleared.push(id);
			scheduled.delete(id);
		},
		// Test helpers:
		fire: (id) => {
			const entry = scheduled.get(id);
			if (!entry) throw new Error(`no scheduled callback for id ${id}`);
			scheduled.delete(id);
			return entry.cb();
		},
		scheduled, // expose so tests can inspect IDs in flight
		cleared,
	};
}

// Builds a fetch stub that responds to specific URLs with queued
// responses. URLs that match a prefix dequeue from that prefix's array.
function makeFetch() {
	const responses = new Map(); // url-prefix -> array of (res|err) entries
	const calls = [];

	function setQueue(urlPrefix, queue) {
		responses.set(urlPrefix, queue.slice());
	}

	function fetchImpl(url, init) {
		calls.push({ url, init });
		for (const [prefix, queue] of responses) {
			if (url.startsWith(prefix)) {
				if (queue.length === 0) {
					return Promise.reject(
						new Error(`fetch stub: no more responses queued for ${url}`),
					);
				}
				const entry = queue.shift();
				if (entry instanceof Error) return Promise.reject(entry);
				return Promise.resolve(entry);
			}
		}
		return Promise.reject(new Error(`fetch stub: unhandled URL ${url}`));
	}

	fetchImpl.calls = calls;
	fetchImpl.setQueue = setQueue;
	return fetchImpl;
}

// Synthetic Response-shaped objects.
const ok200 = (body) => ({
	ok: true,
	status: 200,
	json: () => Promise.resolve(body),
});
const conflict409 = () => ({
	ok: false,
	status: 409,
	json: () => Promise.resolve({}),
});
const err500 = () => ({
	ok: false,
	status: 500,
	json: () => Promise.resolve({}),
});

// Creates a fresh harness that owns the closure state app.js does.
// resetRescanBtn / pollCoverStatus / handleRescanClick are returned and
// mirror the production functions line-for-line.
function makeHarness() {
	const rescanBtn = makeButton();
	const rescanStatus = makeStatusEl();
	const timers = makeTimers();
	const fetchImpl = makeFetch();
	const alerts = [];
	const catalogLoads = [];

	let statusPollTimer = null;

	function resetRescanBtn() {
		rescanBtn.disabled = false;
		rescanBtn.textContent = "Rescan ↻";
		rescanBtn.classList.remove("fetching");
		rescanStatus.textContent = "";
	}

	// loadCatalog mirrors app.js:213-236 — has its own internal .catch
	// that swallows errors and renders an error message. So
	// loadCatalog() NEVER rejects, even when the underlying fetch
	// fails. This is exactly what the production code does and is
	// important for the rescan control-flow analysis.
	function loadCatalog() {
		catalogLoads.push(Date.now());
		return fetchImpl("/api/games")
			.then((res) => {
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return res.json();
			})
			.then(() => {
				/* renderAll() — irrelevant to rescan state machine */
			})
			.catch(() => {
				/* renders an error message; resolves */
			});
	}

	// Returns nothing — mirrors app.js exactly. The inner promise chain
	// runs in the background. Tests use `drainPoll()` below to await
	// quiescence rather than relying on this function's return value.
	let pendingPolls = [];
	function pollCoverStatus() {
		const p = fetchImpl("/api/status")
			.then((res) => res.json())
			.then((data) => {
				if (data.fetchingDetails) {
					rescanBtn.disabled = true;
					rescanBtn.innerHTML =
						'<span class="spinner">↻</span> Fetching game data…';
					rescanBtn.classList.add("fetching");
					rescanStatus.textContent = "Fetching game data…";
					statusPollTimer = timers.setTimeout(pollCoverStatus, 2000);
				} else {
					resetRescanBtn();
					loadCatalog();
				}
			})
			.catch(resetRescanBtn);
		pendingPolls.push(p);
		// IMPORTANT: do NOT return p — app.js (line 425) doesn't either.
	}

	// Drains any in-flight poll promises so the test can observe the
	// post-quiescent state.
	async function drainPolls() {
		while (pendingPolls.length > 0) {
			const batch = pendingPolls;
			pendingPolls = [];
			await Promise.all(batch);
		}
	}

	// Mirrors the rescan-button click handler (app.js:411-435).
	function handleRescanClick() {
		rescanBtn.disabled = true;
		rescanBtn.textContent = "Scanning…";
		rescanStatus.textContent = "Scanning…";
		return fetchImpl("/api/rescan", {
			method: "POST",
			headers: { "X-Requested-With": "freeplay" },
		})
			.then((res) => {
				if (res.status === 409) {
					alerts.push("Scan already in progress.");
					return;
				}
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return loadCatalog().then(pollCoverStatus);
			})
			.catch(() => {
				alerts.push("Rescan failed. Check that Freeplay is running.");
			})
			.finally(() => {
				if (!statusPollTimer) {
					resetRescanBtn();
				}
			});
	}

	return {
		rescanBtn,
		rescanStatus,
		timers,
		fetchImpl,
		alerts,
		catalogLoads,
		handleRescanClick,
		drainPolls,
		// Allow inspection of the closure-scoped poll timer ID:
		getStatusPollTimer: () => statusPollTimer,
	};
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("rescan happy path: single rescan, no fetchingDetails", () => {
	it("button ends up enabled with 'Rescan' label after the operation settles", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [ok200({})]);
		// Two /api/games: one from the rescan click, one from
		// pollCoverStatus's else-branch.
		h.fetchImpl.setQueue("/api/games", [
			ok200({ games: [], consoles: [] }),
			ok200({ games: [], consoles: [] }),
		]);
		h.fetchImpl.setQueue("/api/status", [ok200({ fetchingDetails: false })]);

		await h.handleRescanClick();
		await h.drainPolls();

		assert.equal(h.rescanBtn.disabled, false, "button must be re-enabled");
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
		assert.equal(h.rescanStatus.textContent, "");
		assert.equal(h.alerts.length, 0);
	});
});

describe("rescan with fetchingDetails poll cycle", () => {
	it("schedules a poll while details are being fetched, then resets when done", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [ok200({})]);
		h.fetchImpl.setQueue("/api/games", [
			ok200({ games: [], consoles: [] }),
			ok200({ games: [], consoles: [] }), // second loadCatalog after poll completes
		]);
		// First /api/status: fetching. Second: done.
		h.fetchImpl.setQueue("/api/status", [
			ok200({ fetchingDetails: true }),
			ok200({ fetchingDetails: false }),
		]);

		await h.handleRescanClick();
		await h.drainPolls();

		// A timer should have been scheduled.
		assert.equal(
			h.timers.scheduled.size,
			1,
			"a poll callback must be scheduled while fetching",
		);
		const timerId = [...h.timers.scheduled.keys()][0];
		assert.equal(
			h.getStatusPollTimer(),
			timerId,
			"statusPollTimer should reference the scheduled callback",
		);
		// While polling, button must show the spinner state.
		assert.equal(h.rescanBtn.disabled, true);
		assert.match(h.rescanBtn.innerHTML, /Fetching game data/);

		// Fire the scheduled poll. Second /api/status returns done.
		h.timers.fire(timerId);
		await h.drainPolls();

		assert.equal(h.rescanBtn.disabled, false, "button must be re-enabled");
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
	});

	// BUG HYPOTHESIS: statusPollTimer is never cleared back to null when
	// the poll chain finishes via the else-branch (data.fetchingDetails
	// === false). The closure variable retains the stale timer ID from
	// the LAST setTimeout call, even though that callback has already
	// fired.
	// SKIPPED: deferred per lead-bug-hunt 2026-05-12 severity floor (Critical+High).
	// See LEAD_BUG_HUNT_STATE.md § Deferred findings — MEDIUM, statusPollTimer lifecycle.
	it.skip("BUG: statusPollTimer is not cleared to null after the poll chain completes", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [ok200({})]);
		h.fetchImpl.setQueue("/api/games", [
			ok200({ games: [], consoles: [] }),
			ok200({ games: [], consoles: [] }),
		]);
		h.fetchImpl.setQueue("/api/status", [
			ok200({ fetchingDetails: true }), // schedules poll
			ok200({ fetchingDetails: false }), // poll completes
		]);

		await h.handleRescanClick();
		await h.drainPolls();
		const scheduledId = [...h.timers.scheduled.keys()][0];
		h.timers.fire(scheduledId);
		await h.drainPolls();

		// After the poll chain has fully resolved (button re-enabled),
		// statusPollTimer should be null so that future
		// rescan-click .finally() blocks correctly fall into the
		// safety-reset branch.
		assert.equal(
			h.getStatusPollTimer(),
			null,
			"statusPollTimer must be cleared back to null when polling finishes; " +
				"a stale ID poisons the next rescan's .finally() guard",
		);
	});
});

describe("two rescans back-to-back: second rescan's safety-reset must work", () => {
	// BUG: After rescan #1 fires poll-then-done, statusPollTimer holds a
	// stale (already-fired) timer ID. When rescan #2 hits an error path
	// that doesn't reach pollCoverStatus (e.g., network failure on the
	// /api/rescan POST, or a 409), the .finally() guard sees the stale
	// non-null statusPollTimer and SKIPS resetRescanBtn(). The button
	// remains disabled with text "Scanning..." until manual page reload.
	// SKIPPED: deferred per lead-bug-hunt 2026-05-12 severity floor (Critical+High).
	it.skip("BUG: second rescan that hits 409 leaves the button stuck disabled", async () => {
		const h = makeHarness();

		// Rescan #1: full happy poll cycle. Leaves statusPollTimer
		// holding a stale ID (per the prior test).
		h.fetchImpl.setQueue("/api/rescan", [ok200({}), conflict409()]);
		h.fetchImpl.setQueue("/api/games", [
			ok200({ games: [], consoles: [] }),
			ok200({ games: [], consoles: [] }),
		]);
		h.fetchImpl.setQueue("/api/status", [
			ok200({ fetchingDetails: true }),
			ok200({ fetchingDetails: false }),
		]);

		await h.handleRescanClick();
		await h.drainPolls();
		const id = [...h.timers.scheduled.keys()][0];
		h.timers.fire(id);
		await h.drainPolls();

		// Sanity: button is reset after rescan #1, but timer is stale.
		assert.equal(h.rescanBtn.disabled, false);

		// Rescan #2: server returns 409. No poll is scheduled.
		await h.handleRescanClick();
		await h.drainPolls();

		// EXPECTED CORRECT BEHAVIOR: button is re-enabled, "Rescan" label.
		// ACTUAL: button stays disabled with "Scanning..." text because
		// the .finally() guard sees stale statusPollTimer != null and
		// skips resetRescanBtn().
		assert.equal(
			h.rescanBtn.disabled,
			false,
			"button must be re-enabled after 409 — user needs to retry",
		);
		assert.equal(
			h.rescanBtn.textContent,
			"Rescan ↻",
			"button label must return to 'Rescan'",
		);
		assert.equal(h.rescanStatus.textContent, "", "status line must be cleared");
	});

	// Same bug surface, different trigger: rescan POST itself fails.
	// SKIPPED: deferred per lead-bug-hunt 2026-05-12 severity floor (Critical+High).
	it.skip("BUG: second rescan that fails the POST leaves the button stuck disabled", async () => {
		const h = makeHarness();
		// Rescan #1 success poll-cycle, rescan #2 network-rejected POST.
		h.fetchImpl.setQueue("/api/rescan", [ok200({}), new Error("network down")]);
		h.fetchImpl.setQueue("/api/games", [
			ok200({ games: [], consoles: [] }),
			ok200({ games: [], consoles: [] }),
		]);
		h.fetchImpl.setQueue("/api/status", [
			ok200({ fetchingDetails: true }),
			ok200({ fetchingDetails: false }),
		]);

		await h.handleRescanClick();
		await h.drainPolls();
		const id = [...h.timers.scheduled.keys()][0];
		h.timers.fire(id);
		await h.drainPolls();
		assert.equal(h.rescanBtn.disabled, false, "sanity: rescan #1 succeeded");

		// Rescan #2 — POST rejects.
		await h.handleRescanClick();
		await h.drainPolls();

		assert.deepEqual(h.alerts, [
			"Rescan failed. Check that Freeplay is running.",
		]);
		assert.equal(
			h.rescanBtn.disabled,
			false,
			"button must be re-enabled after a failed rescan #2",
		);
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
	});
});

describe("first-rescan error paths (no prior poll)", () => {
	// On a fresh page load, statusPollTimer is null. .finally() correctly
	// fires resetRescanBtn() on these paths — so these are coverage
	// improvements, not bug repros. They lock in the expected behavior
	// for the no-prior-state case so a fix can't regress it.
	it("first rescan, 409 response: button is reset", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [conflict409()]);
		await h.handleRescanClick();
		await h.drainPolls();
		assert.deepEqual(h.alerts, ["Scan already in progress."]);
		assert.equal(h.rescanBtn.disabled, false);
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
	});

	it("first rescan, 500 response: button is reset", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [err500()]);
		await h.handleRescanClick();
		await h.drainPolls();
		assert.deepEqual(h.alerts, [
			"Rescan failed. Check that Freeplay is running.",
		]);
		assert.equal(h.rescanBtn.disabled, false);
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
	});

	it("first rescan, network rejection on POST: button is reset", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [new Error("network down")]);
		await h.handleRescanClick();
		await h.drainPolls();
		assert.deepEqual(h.alerts, [
			"Rescan failed. Check that Freeplay is running.",
		]);
		assert.equal(h.rescanBtn.disabled, false);
	});
});

describe("/api/status fetch rejects mid-poll: button is reset via .catch(resetRescanBtn)", () => {
	// pollCoverStatus().catch(resetRescanBtn) is the only path that
	// would currently scrub the spinner-state when the status endpoint
	// fails. Confirms that catch handler fires and resets the button.
	it("status fetch rejects: button comes back enabled", async () => {
		const h = makeHarness();
		h.fetchImpl.setQueue("/api/rescan", [ok200({})]);
		h.fetchImpl.setQueue("/api/games", [ok200({ games: [], consoles: [] })]);
		h.fetchImpl.setQueue("/api/status", [new Error("status down")]);

		await h.handleRescanClick();
		await h.drainPolls();

		assert.equal(h.rescanBtn.disabled, false);
		assert.equal(h.rescanBtn.textContent, "Rescan ↻");
	});
});

describe("concurrent poll chains: clicking rescan during an active poll", () => {
	// SCENARIO: In real browsers, the /api/status fetch takes 1+ms
	// to resolve. Because app.js does NOT return pollCoverStatus's
	// promise from the click handler's .then chain, the .finally
	// block races the inner /api/status response. With realistic
	// network latency, .finally wins — statusPollTimer is still null
	// at that moment, so the safety-reset enables the button. The
	// user briefly sees an enabled "Rescan" button between the rescan
	// POST returning and the first /api/status response arriving.
	//
	// If the user clicks again during that flash, a SECOND
	// pollCoverStatus chain is initiated. Both chains schedule their
	// own setTimeout(pollCoverStatus, 2000) and both overwrite
	// `statusPollTimer`. The earlier setTimeout ID is forgotten — no
	// clearTimeout will ever fire on it. When it later wakes up, it
	// runs alongside the second chain, doubling the poll rate and
	// stomping on UI state.
	//
	// CONTRACT we'd like: at most one /api/status poll callback
	// scheduled at any time.
	// SKIPPED: deferred per lead-bug-hunt 2026-05-12 severity floor (Critical+High).
	it.skip("BUG: re-click during /api/status latency window spawns concurrent poll chains", async () => {
		// Custom harness with a /api/status fetch that defers
		// resolution until the test explicitly releases it. This
		// matches what happens in real browsers: the /api/status
		// fetch takes longer than the microtask drain, so the
		// click handler's .finally runs BEFORE /api/status returns.
		const rescanBtn = makeButton();
		const rescanStatus = makeStatusEl();
		const timers = makeTimers();
		const alerts = [];
		let statusPollTimer = null;
		const pendingPolls = [];

		// Manually controllable /api/status responses.
		const statusGate = [];

		function fetchImpl(url) {
			if (url === "/api/rescan") {
				return Promise.resolve(ok200({}));
			}
			if (url === "/api/games") {
				return Promise.resolve(ok200({ games: [], consoles: [] }));
			}
			if (url === "/api/status") {
				let resolve;
				const p = new Promise((r) => {
					resolve = r;
				});
				statusGate.push(() => resolve(ok200({ fetchingDetails: true })));
				return p;
			}
			return Promise.reject(new Error(`unhandled ${url}`));
		}

		function resetRescanBtn() {
			rescanBtn.disabled = false;
			rescanBtn.textContent = "Rescan ↻";
			rescanBtn.classList.remove("fetching");
			rescanStatus.textContent = "";
		}
		function loadCatalog() {
			return fetchImpl("/api/games").then(() => {});
		}
		function pollCoverStatus() {
			const p = fetchImpl("/api/status")
				.then((res) => res.json())
				.then((data) => {
					if (data.fetchingDetails) {
						rescanBtn.disabled = true;
						rescanBtn.innerHTML = "spinner";
						rescanBtn.classList.add("fetching");
						rescanStatus.textContent = "Fetching game data…";
						statusPollTimer = timers.setTimeout(pollCoverStatus, 2000);
					} else {
						resetRescanBtn();
						loadCatalog();
					}
				})
				.catch(resetRescanBtn);
			pendingPolls.push(p);
		}
		function handleRescanClick() {
			rescanBtn.disabled = true;
			rescanBtn.textContent = "Scanning…";
			rescanStatus.textContent = "Scanning…";
			return fetchImpl("/api/rescan", {})
				.then((res) => {
					if (res.status === 409) {
						alerts.push("Scan already in progress.");
						return;
					}
					if (!res.ok) throw new Error(`HTTP ${res.status}`);
					return loadCatalog().then(pollCoverStatus);
				})
				.catch(() => {
					alerts.push("Rescan failed.");
				})
				.finally(() => {
					if (!statusPollTimer) {
						resetRescanBtn();
					}
				});
		}

		// CLICK 1: rescan POST resolves, loadCatalog resolves, pollCoverStatus
		// kicks off a /api/status fetch (still pending — deferred). Then
		// .finally runs with statusPollTimer still null → resetRescanBtn().
		await handleRescanClick();
		assert.equal(
			rescanBtn.disabled,
			false,
			"click #1 .finally ran before /api/status resolved — button is enabled (the flash)",
		);
		assert.equal(
			statusPollTimer,
			null,
			"timer not yet set because /api/status response is still in flight",
		);

		// CLICK 2 during the flash — same outcome: kicks off another
		// /api/status fetch. Now TWO /api/status fetches are pending.
		await handleRescanClick();
		assert.equal(statusGate.length, 2, "two /api/status fetches now pending");

		// Now release both /api/status responses.
		for (const release of statusGate) release();
		// Drain microtasks until everything settles.
		while (pendingPolls.length > 0) {
			const batch = pendingPolls.splice(0);
			await Promise.all(batch);
		}

		// CONTRACT: at most one scheduled poll callback. The current
		// code overwrites statusPollTimer each time, so the first
		// setTimeout ID is forgotten — but it's still scheduled and
		// will fire.
		const ids = [...timers.scheduled.keys()];
		assert.ok(
			ids.length <= 1,
			`expected at most one scheduled poll, found ${ids.length} — ` +
				`re-click during the /api/status latency window spawned a ` +
				`second poll chain whose timer leaks`,
		);
	});
});
