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
		window.EJS_color = "#1a1a2e";
		window.EJS_gameName = nameEl.textContent;
		window.EJS_startOnLoaded = true;

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

		// Register periodic SRAM save after game starts
		window.EJS_onGameStart = () => {
			if (window.EJS_emulator) {
				window.EJS_emulator.on("saveSaveFiles", (data) => {
					postSave("sram", data);
				});
			}
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
