// Master-volume workaround for EmulatorJS's non-functional volume slider.
//
// ---------------------------------------------------------------------------
// WHY THIS EXISTS
// ---------------------------------------------------------------------------
// EmulatorJS's volume slider and mute button both funnel through one method,
// `emulator.setVolume(v)`, which changes volume ONLY by walking the Emscripten
// OpenAL source list (`Module.AL.currentCtx.sources`) and setting each source's
// gain. That works for cores whose audio flows through the Emscripten OpenAL
// driver.
//
// The RetroArch cores EmulatorJS ships do NOT use OpenAL at runtime. They use
// RetroArch's own `rwebaudio` driver, which streams `AudioBufferSourceNode`s
// connected DIRECTLY to `AudioContext.destination` — no gain stage, and
// `Module.AL.currentCtx` stays null. So `setVolume` iterates an empty list and
// changes nothing: the slider moves, mute toggles its icon, and the audio never
// changes. This was verified live (snes9x: 69 `AudioBufferSourceNode ->
// AudioDestinationNode` connects/sec, `AL.contexts` empty).
//
// It is NOT fixable upstream from our side: the latest EmulatorJS release and
// `main` both leave `setVolume` OpenAL-only, and forcing `audio_driver=openal`
// in retroarch.cfg does not make the core engage OpenAL (tested — it stays on
// rwebaudio). See ARCHITECTURE.md § "EmulatorJS volume workaround".
//
// ---------------------------------------------------------------------------
// WHAT THIS DOES
// ---------------------------------------------------------------------------
// A browser has exactly one audio sink: `AudioContext.destination`. Every audio
// driver — rwebaudio, OpenAL, an AudioWorklet-based one, anything — must
// ultimately connect to it to make sound. So we intercept `AudioNode.connect`
// and splice a single per-context "master" GainNode in front of `destination`:
//
//     source ─► destination        becomes        source ─► masterGain ─► destination
//
// A GainNode at 1.0 is a mathematically transparent passthrough, so this cannot
// break ANY driver's audio (an unknown future driver simply gains a working
// volume control rather than losing one). Setting the master gain scales the
// output linearly — a correct master volume for any source.
//
// ---------------------------------------------------------------------------
// THE ONE HAZARD, AND THE BEHAVIOR-BASED GUARD
// ---------------------------------------------------------------------------
// The single real risk is DOUBLE attenuation: if a core DID use OpenAL,
// EmulatorJS's own `setVolume` would scale the OpenAL gains AND our master would
// scale again, giving volume². That is the ONLY driver-specific hazard, and it
// is positively detectable at runtime: an active OpenAL context means
// `Module.AL.currentCtx` is populated.
//
// So the guard is behavioral, not a driver-name whitelist: on every volume
// change we let EmulatorJS's original `setVolume` run (it updates the slider UI,
// mute-button state, netplay volume, and — for OpenAL cores — the real gains),
// and we then apply OUR master gain ONLY when no active OpenAL context is
// present. If OpenAL is driving, our master stays at 1.0 and EmulatorJS keeps
// control; otherwise our master IS the control. This is safe for unknown
// drivers (transparent when idle, working when needed) and avoids the treadmill
// of a positive `is-rwebaudio` whitelist that would fail closed for any third
// driver.
//
// ---------------------------------------------------------------------------
// SHAPE
// ---------------------------------------------------------------------------
//   installGraphInterceptor() — patch AudioNode.prototype.connect. Runs at
//       module load (below) so it is in place before the core builds its audio
//       graph. Browser-only; a no-op where AudioNode is undefined (Node tests).
//   applyMasterVolume(volume, openalActive) — the pure decision + apply step.
//   wrapSetVolume(emulator) — wrap emulator.setVolume once and sync the initial
//       gain to the slider's starting position. No-op until the emulator's
//       Emscripten Module is attached (EmulatorJS's setVolume needs it).
//   installMasterVolume(getEmulator?) — poll for the emulator to become ready
//       (setVolume defined + Module attached, both landing after "start") and
//       wrap it.
//
// `applyMasterVolume` and `wrapSetVolume` are pure/injectable so the decision
// logic is unit-tested without a real AudioContext; the graph interception
// itself is verified live (Playwright), matching how the other EmulatorJS-
// coupled handlers in this frontend are tested.
((exports) => {
	// Shared state. `masterGains` holds strong references to the GainNodes we
	// insert — one per AudioContext. A play session has a single context (a
	// handful at most if the core recreates one), all live for the page's
	// lifetime, so retaining them is intentional and bounded.
	const state = {
		masterGains: new Set(),
		volume: 1, // last requested volume, 0..1
		attenuate: false, // whether `volume` is actually applied (vs unity passthrough)
	};

	// The gain value the master nodes should currently carry. When we are NOT
	// the active volume authority (OpenAL is), master stays at unity so it is a
	// transparent passthrough.
	const effectiveGain = () => (state.attenuate ? state.volume : 1);

	// Find (or lazily create) the master GainNode for a context, inserted
	// between the context and its real destination. `origConnect` is the
	// unpatched connect, used both to wire master -> destination and to avoid
	// re-entering our interceptor.
	const masterFor = (ctx, origConnect) => {
		for (const g of state.masterGains) {
			if (g.context === ctx) return g;
		}
		const g = ctx.createGain();
		g.gain.value = effectiveGain();
		origConnect.call(g, ctx.destination);
		state.masterGains.add(g);
		return g;
	};

	// Decide whether to attenuate, then push the resulting gain to every master
	// node. `openalActive` true => EmulatorJS owns volume via the OpenAL gains,
	// so we stay transparent (unity). Returns the applied gain for testability.
	exports.applyMasterVolume = (volume, openalActive) => {
		state.volume = volume;
		state.attenuate = !openalActive;
		const g = effectiveGain();
		for (const node of state.masterGains) {
			node.gain.value = g;
		}
		return g;
	};

	// Patch AudioNode.connect so any connect to a context's destination is
	// rerouted through that context's master gain. Idempotent; browser-only.
	exports.installGraphInterceptor = () => {
		if (typeof AudioNode === "undefined") return; // Node test environment
		if (AudioNode.prototype.__fpMasterVolume) return;
		const origConnect = AudioNode.prototype.connect;
		AudioNode.prototype.__fpMasterVolume = true;
		AudioNode.prototype.connect = function (dest, ...rest) {
			if (dest && this.context && dest === this.context.destination) {
				const m = masterFor(this.context, origConnect);
				// Never reroute the master's own connection to destination.
				if (this !== m) return origConnect.call(this, m, ...rest);
			}
			return origConnect.call(this, dest, ...rest);
		};
	};

	// Wrap emulator.setVolume exactly once so the slider and mute button drive
	// the master gain, and sync the master gain to the slider's initial
	// position. Returns true if it wrapped, false if not ready / already
	// wrapped. Pure w.r.t. globals — the emulator is passed in — so it is
	// directly unit-testable with a stub.
	exports.wrapSetVolume = (emulator) => {
		if (
			!emulator ||
			typeof emulator.setVolume !== "function" ||
			// EmulatorJS's own setVolume dereferences `this.Module.AL` WITHOUT
			// optional chaining (emulator.js: `this.Module.AL && ...`), so calling
			// it — including our initial sync below — throws
			// "Cannot read properties of undefined (reading 'AL')" until the
			// Emscripten Module is attached. setVolume is defined in
			// createBottomMenuBar, which can run a beat before Module lands, so
			// gate on Module here and let the poll keep retrying until it exists.
			!emulator.Module ||
			emulator.__fpMasterVolumeInstalled
		) {
			return false;
		}
		emulator.__fpMasterVolumeInstalled = true;

		const orig = emulator.setVolume;
		emulator.setVolume = function (volume) {
			// Let EmulatorJS do its thing first: slider UI, mute state, netplay,
			// and (for OpenAL cores) the real per-source gains.
			const result = orig.call(this, volume);
			const openalActive = !!this.Module?.AL?.currentCtx;
			exports.applyMasterVolume(volume, openalActive);
			return result;
		};

		// Sync the master gain to the current slider value. Without this the
		// slider would read e.g. 100% while the freshly-created master sat at
		// its default — the initial call aligns them.
		emulator.setVolume(
			typeof emulator.volume === "number" ? emulator.volume : 1,
		);
		return true;
	};

	// Poll until the emulator exists, setVolume is defined (it is created
	// inside createBottomMenuBar, which runs AFTER the "start" event, so no
	// single EmulatorJS event is a reliable hook), AND its Emscripten Module is
	// attached (see the Module gate in wrapSetVolume), then wrap it. Bounded so a
	// core that never finishes starting doesn't leave a live interval.
	exports.installMasterVolume = (getEmulator) => {
		const resolve = getEmulator || (() => window.EJS_emulator);
		let tries = 0;
		const timer = setInterval(() => {
			if (exports.wrapSetVolume(resolve()) || ++tries > 100) {
				clearInterval(timer);
			}
		}, 200);
		return timer;
	};

	// Test seam: lets unit tests inspect/reset the tracked master gains without
	// a real AudioContext.
	exports.__state = state;

	// Install the graph interceptor immediately on load, before the emulator
	// core boots and builds its audio graph.
	exports.installGraphInterceptor();
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
