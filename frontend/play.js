(() => {
	const FP = window.Freeplay;

	const params = new URLSearchParams(window.location.search);
	const consoleName = params.get("console");
	const rom = params.get("rom");

	if (!consoleName || !rom) {
		showError("Missing console or rom parameter.");
		return;
	}

	const nameEl = document.getElementById("game-name");
	nameEl.textContent = FP.stripExt(rom);
	document.title = `Freeplay - ${nameEl.textContent}`;

	FP.initThemeToggle();

	fetch("/api/games")
		.then((res) => res.json())
		.then((catalog) => {
			const game = FP.findGame(catalog.games, consoleName, rom);
			if (!game) {
				showError("Game not found. It may have been removed from the library.");
				return;
			}
			startEmulator(game);
		})
		.catch(() => {
			showError("Could not load game catalog.");
		});

	function showError(msg) {
		document.getElementById("game").style.display = "none";
		const el = document.getElementById("error");
		el.style.display = "";
		el.textContent = msg;
	}

	function startEmulator(game) {
		const saveBase = FP.saveBasePath(consoleName, nameEl.textContent);

		window.EJS_player = "#game";
		window.EJS_core = game.core;
		window.EJS_gameUrl = FP.romUrl(consoleName, rom);
		window.EJS_pathtodata = "/emulatorjs/data/";
		window.EJS_color =
			document.documentElement.dataset.theme === "light"
				? "#f0f0f5"
				: "#1a1a2e";
		window.EJS_gameName = nameEl.textContent;
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
					body: new Blob([data]),
				});
		}

		window.EJS_onSaveState = (data) => {
			postSave("state", data.state);
		};
		window.EJS_onSaveSave = (data) => {
			postSave("sram", data.save);
		};

		// Load SRAM save from server (if exists), then register periodic saves
		window.EJS_onGameStart = () => {
			if (!window.EJS_emulator) return;

			// Load existing SRAM save from server
			fetch(`${saveBase}/sram`)
				.then((res) => {
					if (!res.ok) return;
					return res.arrayBuffer();
				})
				.then((buf) => {
					if (!buf) return;
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
				})
				.catch(() => {});

			// Register periodic SRAM save
			window.EJS_emulator.on("saveSaveFiles", (data) => {
				postSave("sram", data);
			});
		};

		// Load save state if one exists, then start the emulator
		fetch(`${saveBase}/state`, { method: "HEAD" })
			.then((res) => {
				if (res.ok) {
					window.EJS_loadStateURL = `${saveBase}/state`;
				}
			})
			.catch(() => {})
			.finally(() => {
				const script = document.createElement("script");
				script.src = "/emulatorjs/data/loader.js";
				document.body.appendChild(script);
			});
	}
})();
