// Loads a save state from the server and applies it to the running
// emulator. Wired to EmulatorJS's "loadState" event (via
// EJS_onLoadState) so the bottom-bar "load state" button restores from
// the server — the same place the bottom-bar "save state" button writes
// to via EJS_onSaveState.
//
// Registering ANY loadState listener also makes EmulatorJS's button
// handler short-circuit (callEvent returns the listener count; the
// button returns early when it is > 0). That matters for correctness,
// not just consistency: without a listener the button falls through to
// EmulatorJS's built-in paths, and the browser-storage path calls
// gameManager.loadState(undefined) when the state lives on the server
// rather than in browser storage. That undefined aborts the WASM
// runtime ("Aborted(Unsupported data type)"), which cannot be recovered
// without a page reload.
//
// Branching mirrors the SRAM restore and the save-state HEAD probe in
// play.js:
//   - 2xx with bytes: apply the state, confirm to the user.
//   - 2xx zero-length: nothing to apply (mirrors postSave's zero-byte
//     no-op guard); tell the user.
//   - 404: no state on the server; tell the user.
//   - other non-2xx / fetch threw: log via console.error and surface a
//     failure message.
//
// `gm` must expose loadState(Uint8Array). `notify(msg)` surfaces a
// transient message to the user (EmulatorJS displayMessage in prod).
// `fetchImpl` is injected for testing; production callers omit it.
((exports) => {
	exports.loadStateFromServer = async (
		saveBase,
		gm,
		notify,
		fetchImpl = fetch,
	) => {
		let res;
		try {
			res = await fetchImpl(`${saveBase}/state`);
		} catch (err) {
			console.error("load-state failed:", err);
			notify("Could not load save state");
			return;
		}

		if (res.status === 404) {
			notify("No save state to load");
			return;
		}
		if (!res.ok) {
			console.error(
				"load-state: server returned non-2xx, non-404; skipping load",
				res.status,
			);
			notify("Could not load save state");
			return;
		}

		const buf = await res.arrayBuffer();
		// A zero-length body would mean loadState(new Uint8Array(0)) — not
		// the undefined that aborts the runtime, but still nothing worth
		// applying. Treat it as "no state" rather than corrupting the
		// running game with an empty load.
		if (buf.byteLength === 0) {
			notify("No save state to load");
			return;
		}

		// Guard the emulator call: a state written by this same emulator is
		// valid, but a corrupt/foreign payload could make loadState throw.
		// Surfacing a message beats an unhandled rejection. (A hard WASM
		// abort would still be unrecoverable — but the undefined that
		// caused the original crash can no longer reach here.)
		try {
			gm.loadState(new Uint8Array(buf));
		} catch (err) {
			console.error("load-state: emulator rejected the state:", err);
			notify("Could not load save state");
			return;
		}

		notify("Loaded save state");
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
