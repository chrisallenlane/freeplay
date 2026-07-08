(async () => {
	const FP = window.Freeplay;

	const subpage = FP.initSubpage();
	if (!subpage) {
		FP.showError("game", "Missing console or rom parameter.");
		return;
	}
	const { consoleName, rom, gameName } = subpage;

	let game;
	try {
		game = await FP.loadGame(consoleName, rom, "game");
	} catch {
		FP.showError("game", "Could not load game catalog.");
		return;
	}
	if (!game) return;

	const toggle = document.getElementById("theme-toggle");

	// Update page title with IGDB name if available
	FP.loadGameDetails(consoleName, rom).then((details) => {
		if (details?.name) document.title = `Freeplay - ${details.name}`;
	});

	if (game.hasManual) {
		const manualLink = FP.el("a", "btn header-btn", "Manual");
		manualLink.href = FP.manualUrl(game);
		manualLink.title = "View the manual";
		toggle.parentNode.insertBefore(manualLink, toggle);
	}

	await startEmulator(game);

	async function startEmulator(game) {
		const saveBase = FP.saveBasePath(consoleName, gameName);

		window.EJS_player = "#game";
		window.EJS_core = game.core;
		window.EJS_gameUrl = FP.romUrl(consoleName, rom);
		window.EJS_pathtodata = "/emulatorjs/data/";
		window.EJS_color =
			document.documentElement.dataset.theme === "light"
				? "#f0f0f5"
				: "#1a1a2e";
		window.EJS_gameName = gameName;
		window.EJS_startOnLoaded = true;

		if (game.hasBios) {
			window.EJS_biosUrl = FP.biosUrl(consoleName);
		}

		// Surface a transient confirmation via the emulator's own message
		// overlay. Falls back to a no-op if the emulator isn't up yet.
		const notify = (msg) => window.EJS_emulator?.displayMessage?.(msg);

		// Manual save (bottom-bar "save state" button). Posts to the
		// server and confirms — without the message the button "appears to
		// do nothing", since EmulatorJS suppresses its own SAVED-STATE
		// message once a saveState listener is registered.
		window.EJS_onSaveState = (data) => {
			const req = FP.postSave(saveBase, "state", data?.state);
			if (!req) {
				// postSave skipped the request (empty/falsy state) — nothing
				// reached the server.
				notify("Could not save state");
				return;
			}
			req.then((res) =>
				notify(res?.ok ? "Saved save state" : "Could not save state"),
			);
		};

		// Manual load (bottom-bar "load state" button). Restores from the
		// server, mirroring the save above. Registering this listener also
		// stops EmulatorJS's button from falling through to its built-in
		// browser-storage path, which aborts the WASM runtime when it calls
		// gameManager.loadState(undefined) for a server-resident state.
		window.EJS_onLoadState = () => {
			const gm = window.EJS_emulator?.gameManager;
			if (!gm) return;
			FP.loadStateFromServer(saveBase, gm, notify);
		};

		// Load SRAM save from server (if exists), then register periodic
		// saves. Branch on response shape:
		//   - 2xx: restore the bytes, then register the periodic save
		//   - 404: no save on disk (legitimate fresh game), register
		//     the periodic save so the user's progress is captured
		//   - other non-2xx (5xx, 503, etc.): server can't tell us
		//     whether a save exists; refuse to register the periodic
		//     save so the next auto-save tick doesn't overwrite a
		//     still-on-disk-but-unreadable real save
		//   - fetch threw (network down): same — don't register
		let sramHandlerRegistered = false;
		window.EJS_onGameStart = async () => {
			if (!window.EJS_emulator) return;

			let safeToRegister = false;
			try {
				const res = await fetch(`${saveBase}/sram`);
				if (res.ok) {
					const buf = await res.arrayBuffer();
					FP.restoreSaveToFS(window.EJS_emulator.gameManager, buf);
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
				window.EJS_emulator.on("saveSaveFiles", (data) => {
					FP.postSave(saveBase, "sram", data);
				});
			}
		};

		// Probe for a save state. Same branching shape as the SRAM
		// restore: only set EJS_loadStateURL when the server confirms a
		// state exists; on transient failures, leave it unset and log
		// so operators can correlate against server-side logs.
		try {
			const res = await fetch(`${saveBase}/state`, { method: "HEAD" });
			if (res.ok) {
				window.EJS_loadStateURL = `${saveBase}/state`;
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

		const script = document.createElement("script");
		script.src = "/emulatorjs/data/loader.js";
		document.body.appendChild(script);
	}
})();
