(() => {
	const FP = window.Freeplay;

	let allGames = [];
	let consoles = [];
	let activeConsole = null;
	let activeFavorites = false;
	const favorites = new Set(
		JSON.parse(localStorage.getItem("freeplay-favorites") || "[]"),
	);

	const grid = document.getElementById("game-grid");
	const filtersBar = document.getElementById("filters");
	const searchInput = document.getElementById("search");
	const rescanBtn = document.getElementById("rescan-btn");

	function saveFavorites() {
		localStorage.setItem(
			"freeplay-favorites",
			JSON.stringify(Array.from(favorites)),
		);
	}

	function getFilteredGames() {
		return FP.filterGames(allGames, {
			favorites: favorites,
			favoritesOnly: activeFavorites,
			console: activeConsole,
			query: searchInput.value,
		});
	}

	function renderAll() {
		renderFilters();
		renderGrid();
	}

	function renderFilters() {
		filtersBar.innerHTML = "";

		const favBtn = document.createElement("button");
		favBtn.className = `filter-btn${activeFavorites ? " active" : ""}`;
		favBtn.textContent = "\u2605 Favorites";
		favBtn.addEventListener("click", () => {
			activeFavorites = !activeFavorites;
			if (activeFavorites) activeConsole = null;
			renderAll();
		});
		filtersBar.appendChild(favBtn);

		const allBtn = document.createElement("button");
		allBtn.className = `filter-btn${!activeConsole && !activeFavorites ? " active" : ""}`;
		allBtn.textContent = "All";
		allBtn.addEventListener("click", () => {
			activeConsole = null;
			activeFavorites = false;
			renderAll();
		});
		filtersBar.appendChild(allBtn);

		consoles.forEach((name) => {
			const btn = document.createElement("button");
			btn.className = `filter-btn${activeConsole === name ? " active" : ""}`;
			btn.textContent = name;
			btn.addEventListener("click", () => {
				activeConsole = name;
				activeFavorites = false;
				renderAll();
			});
			filtersBar.appendChild(btn);
		});
	}

	function renderGrid() {
		grid.innerHTML = "";
		const games = getFilteredGames();

		if (games.length === 0) {
			const msg = document.createElement("div");
			msg.className = "message";
			if (allGames.length === 0) {
				msg.textContent =
					"No games found. Add ROMs to your library and check your freeplay.toml configuration.";
			} else if (activeFavorites) {
				msg.textContent =
					"No favorites yet. Click the star on a game to add it.";
			} else {
				msg.textContent = "No games match your search.";
			}
			grid.appendChild(msg);
			return;
		}

		games.forEach((game) => {
			const card = document.createElement("div");
			card.className = "game-card";

			// Favorite button
			const isFav = favorites.has(FP.favKey(game));
			const fav = document.createElement("button");
			fav.className = `fav-btn${isFav ? " favorited" : ""}`;
			fav.textContent = isFav ? "\u2605" : "\u2606";
			fav.addEventListener("click", (e) => {
				e.stopPropagation();
				const key = FP.favKey(game);
				if (favorites.has(key)) {
					favorites.delete(key);
					fav.textContent = "\u2606";
					fav.classList.remove("favorited");
				} else {
					favorites.add(key);
					fav.textContent = "\u2605";
					fav.classList.add("favorited");
				}
				saveFavorites();
				if (activeFavorites) renderGrid();
			});
			card.appendChild(fav);

			// Cover art or placeholder
			if (game.hasCover) {
				const img = document.createElement("img");
				img.className = "cover";
				img.src = FP.coverUrl(game);
				img.alt = game.filename;
				img.loading = "lazy";
				card.appendChild(img);
			} else {
				const ph = document.createElement("div");
				ph.className = "placeholder-cover";
				const phName = document.createElement("div");
				phName.className = "placeholder-name";
				phName.textContent = FP.stripExt(game.filename);
				const phConsole = document.createElement("div");
				phConsole.className = "placeholder-console";
				phConsole.textContent = game.console;
				ph.appendChild(phName);
				ph.appendChild(phConsole);
				card.appendChild(ph);
			}

			// Card info
			const info = document.createElement("div");
			info.className = "card-info";
			const title = document.createElement("div");
			title.className = "card-title";
			title.textContent = FP.stripExt(game.filename);
			const consoleName = document.createElement("div");
			consoleName.className = "card-console";
			consoleName.textContent = game.console;
			info.appendChild(title);
			info.appendChild(consoleName);
			card.appendChild(info);

			// Click to play
			card.addEventListener("click", () => {
				window.location.href = FP.playUrl(game);
			});

			grid.appendChild(card);
		});
	}

	function loadCatalog() {
		return fetch("/api/games")
			.then((res) => {
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return res.json();
			})
			.then((catalog) => {
				allGames = catalog.games || [];
				consoles = catalog.consoles || [];
				renderAll();
			})
			.catch(() => {
				grid.innerHTML = "";
				const msg = document.createElement("div");
				msg.className = "message";
				msg.textContent =
					"Could not load game library. Check that Freeplay is running.";
				const retry = document.createElement("button");
				retry.textContent = "Retry";
				retry.addEventListener("click", loadCatalog);
				msg.appendChild(retry);
				grid.appendChild(msg);
			});
	}

	searchInput.addEventListener("input", renderGrid);

	// Rescan button
	let statusPollTimer = null;

	function resetRescanBtn() {
		rescanBtn.disabled = false;
		rescanBtn.textContent = "Rescan \u21BB";
		rescanBtn.classList.remove("fetching");
	}

	function pollCoverStatus() {
		fetch("/api/status")
			.then((res) => res.json())
			.then((data) => {
				if (data.fetchingCovers) {
					rescanBtn.disabled = true;
					rescanBtn.innerHTML =
						'<span class="spinner">\u21BB</span> Fetching covers\u2026';
					rescanBtn.classList.add("fetching");
					statusPollTimer = setTimeout(pollCoverStatus, 2000);
				} else {
					resetRescanBtn();
					loadCatalog();
				}
			})
			.catch(resetRescanBtn);
	}

	rescanBtn.addEventListener("click", () => {
		rescanBtn.disabled = true;
		rescanBtn.textContent = "Scanning\u2026";
		fetch("/api/rescan", { method: "POST" })
			.then((res) => {
				if (res.status === 409) {
					alert("Scan already in progress.");
					return;
				}
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				return loadCatalog().then(pollCoverStatus);
			})
			.catch(() => {
				alert("Rescan failed. Check that Freeplay is running.");
			})
			.finally(() => {
				if (!statusPollTimer) {
					resetRescanBtn();
				}
			});
	});

	loadCatalog();
})();
