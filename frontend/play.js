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
		// Load unminified EmulatorJS sources. The vendored emulator.min.js
		// does not include our controller port device patches (lightgun
		// support), so we must load the individual source files instead.
		window.EJS_DEBUG_XX = true;

		if (game.hasBios) {
			window.EJS_biosUrl = FP.biosUrl(consoleName);
		}

		function postSave(type, data) {
			if (data)
				fetch(`${saveBase}/${type}`, {
					method: "POST",
					headers: { "X-Requested-With": "freeplay" },
					body: new Blob([data]),
				}).catch((err) => console.error(`Save failed (${type}):`, err));
		}

		window.EJS_onSaveState = (data) => {
			postSave("state", data.state);
		};

		// Load SRAM save from server (if exists), then register periodic saves
		let sramHandlerRegistered = false;
		window.EJS_onGameStart = async () => {
			if (!window.EJS_emulator) return;

			try {
				const res = await fetch(`${saveBase}/sram`);
				if (res.ok) {
					const buf = await res.arrayBuffer();
					const gm = window.EJS_emulator.gameManager;
					const path = gm.getSaveFilePath();
					const parts = path.split("/");
					let cp = "";
					for (let i = 0; i < parts.length - 1; i++) {
						if (parts[i] === "") continue;
						cp += `/${parts[i]}`;
						if (!gm.FS.analyzePath(cp).exists) gm.FS.mkdir(cp);
					}
					if (gm.FS.analyzePath(path).exists) gm.FS.unlink(path);
					gm.FS.writeFile(path, new Uint8Array(buf));
					gm.loadSaveFiles();
				}
			} catch (err) {
				console.error("SRAM restore failed:", err);
			}

			// Register periodic SRAM save (once only)
			if (!sramHandlerRegistered) {
				sramHandlerRegistered = true;
				window.EJS_emulator.on("saveSaveFiles", (data) => {
					postSave("sram", data);
				});
			}
		};

		// Load save state if one exists, then start the emulator
		try {
			const res = await fetch(`${saveBase}/state`, { method: "HEAD" });
			if (res.ok) {
				window.EJS_loadStateURL = `${saveBase}/state`;
			}
		} catch {
			// Ignore — save state is optional.
		}

		const script = document.createElement("script");
		script.src = "/emulatorjs/data/loader.js";
		document.body.appendChild(script);
	}
})();
