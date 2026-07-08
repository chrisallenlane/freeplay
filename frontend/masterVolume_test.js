// Tests for masterVolume.js — the workaround that gives EmulatorJS's dead
// volume slider a working master GainNode (see the file header and
// ARCHITECTURE.md § "EmulatorJS volume workaround" for the why).
//
// The graph interceptor (AudioNode.prototype.connect patch) is browser-only
// and verified live with Playwright. These tests pin the parts that carry the
// actual decision logic and are the regression-prone bits:
//   - applyMasterVolume's attenuate-or-passthrough decision (the behavioral
//     guard that avoids double-attenuating OpenAL cores), and that it pushes
//     the resulting gain to every tracked master node.
//   - wrapSetVolume: wraps once, forwards to the original setVolume, derives
//     openalActive from the live Module.AL state, and syncs the initial gain.

const { describe, it, beforeEach } = require("node:test");
const assert = require("node:assert/strict");

const FP = require("./masterVolume.js");

// Fake GainNode whose only observable is gain.value.
function fakeGain(initial = 1) {
	return { gain: { value: initial } };
}

// Reset shared module state between tests (masterGains persists across calls
// by design in production, but tests need isolation).
beforeEach(() => {
	FP.__state.masterGains.clear();
	FP.__state.volume = 1;
	FP.__state.attenuate = false;
});

describe("applyMasterVolume: behavioral guard", () => {
	it("no OpenAL: attenuates — pushes the requested volume to every master node", () => {
		const a = fakeGain();
		const b = fakeGain();
		FP.__state.masterGains.add(a);
		FP.__state.masterGains.add(b);

		const applied = FP.applyMasterVolume(0.3, false);

		assert.equal(applied, 0.3);
		assert.equal(a.gain.value, 0.3);
		assert.equal(b.gain.value, 0.3);
	});

	it("OpenAL active: stays transparent — master nodes held at unity regardless of volume", () => {
		const a = fakeGain(0.3); // pretend it was attenuated earlier
		FP.__state.masterGains.add(a);

		const applied = FP.applyMasterVolume(0.3, true);

		// EmulatorJS owns the volume via OpenAL gains; our master must not
		// also scale, or the core would get volume-squared.
		assert.equal(applied, 1);
		assert.equal(a.gain.value, 1);
	});

	it("volume 0 (mute), no OpenAL: silences the master node", () => {
		const a = fakeGain();
		FP.__state.masterGains.add(a);

		FP.applyMasterVolume(0, false);

		assert.equal(a.gain.value, 0);
	});

	it("switching from OpenAL-active back to inactive re-applies attenuation", () => {
		const a = fakeGain();
		FP.__state.masterGains.add(a);

		FP.applyMasterVolume(0.5, true); // OpenAL owns it -> unity
		assert.equal(a.gain.value, 1);

		FP.applyMasterVolume(0.5, false); // OpenAL gone -> we attenuate
		assert.equal(a.gain.value, 0.5);
	});
});

describe("wrapSetVolume: hooking EmulatorJS setVolume", () => {
	// Minimal emulator stub: records original setVolume calls and exposes a
	// mutable Module.AL so tests can toggle the OpenAL-active signal.
	function makeEmulator({ volume = 1, openalActive = false } = {}) {
		const origCalls = [];
		return {
			volume,
			Module: openalActive ? { AL: { currentCtx: {} } } : { AL: {} },
			setVolume(v) {
				origCalls.push(v);
			},
			_origCalls: origCalls,
		};
	}

	it("wraps once and syncs the initial gain to the slider position", () => {
		const node = fakeGain();
		FP.__state.masterGains.add(node);
		const e = makeEmulator({ volume: 1 });

		const wrapped = FP.wrapSetVolume(e);

		assert.equal(wrapped, true);
		assert.equal(e.__fpMasterVolumeInstalled, true);
		// The sync called the (now-wrapped) setVolume with e.volume, which
		// forwards to the original and applies the master gain.
		assert.deepEqual(e._origCalls, [1]);
		assert.equal(node.gain.value, 1);
	});

	it("a later setVolume forwards to the original AND drives the master gain (no OpenAL)", () => {
		const node = fakeGain();
		FP.__state.masterGains.add(node);
		const e = makeEmulator({ volume: 1, openalActive: false });
		FP.wrapSetVolume(e);

		e.setVolume(0.25);

		// Original was called (slider UI / mute state still handled upstream)...
		assert.ok(e._origCalls.includes(0.25));
		// ...and our master gain now reflects it.
		assert.equal(node.gain.value, 0.25);
	});

	it("with OpenAL active, a later setVolume keeps the master transparent", () => {
		const node = fakeGain();
		FP.__state.masterGains.add(node);
		const e = makeEmulator({ volume: 1, openalActive: true });
		FP.wrapSetVolume(e);

		e.setVolume(0.25);

		assert.ok(e._origCalls.includes(0.25));
		// Master stays at unity — EmulatorJS's OpenAL path owns the volume.
		assert.equal(node.gain.value, 1);
	});

	it("is idempotent: a second wrap is a no-op", () => {
		const e = makeEmulator();
		assert.equal(FP.wrapSetVolume(e), true);
		const afterFirst = e.setVolume;
		assert.equal(FP.wrapSetVolume(e), false);
		assert.equal(e.setVolume, afterFirst, "setVolume must not be re-wrapped");
	});

	it("returns false when the emulator isn't ready (no setVolume yet)", () => {
		assert.equal(FP.wrapSetVolume(undefined), false);
		assert.equal(FP.wrapSetVolume({ volume: 1 }), false);
	});

	it("returns false — and does not call setVolume — until Module is attached", () => {
		// Reproduces the crash: EmulatorJS's real setVolume dereferences
		// this.Module.AL without optional chaining, so the initial sync throws
		// "reading 'AL'" if we wrap before Module lands. Model that here with an
		// orig setVolume that throws when this.Module is undefined; wrapSetVolume
		// must not wrap (and thus must not fire the sync) while Module is absent.
		let origCalls = 0;
		const e = {
			volume: 1,
			// Module deliberately absent.
			setVolume() {
				origCalls++;
				// eslint-disable-next-line no-unused-expressions
				this.Module.AL; // throws TypeError when Module is undefined
			},
		};

		assert.equal(FP.wrapSetVolume(e), false);
		assert.equal(
			e.__fpMasterVolumeInstalled,
			undefined,
			"must not mark installed",
		);
		assert.equal(origCalls, 0, "must not call setVolume before Module exists");
		assert.equal(typeof e.setVolume, "function");

		// Once Module is attached, the wrap installs and the initial sync runs
		// against the original without throwing.
		e.Module = { AL: {} };
		assert.equal(FP.wrapSetVolume(e), true);
		assert.equal(origCalls, 1, "initial sync forwards to the original once");
	});

	it("defaults the initial sync to full volume when emulator.volume is absent", () => {
		const node = fakeGain(0.2);
		FP.__state.masterGains.add(node);
		const e = {
			Module: { AL: {} },
			setVolume() {},
		};

		FP.wrapSetVolume(e);

		// No numeric emulator.volume -> sync to 1.0.
		assert.equal(node.gain.value, 1);
	});
});
