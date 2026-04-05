(() => {
	const FP = window.Freeplay;

	const subpage = FP.initSubpage();
	if (!subpage) {
		showError("Missing console or rom parameter.");
		return;
	}
	const { consoleName, rom, gameName } = subpage;

	const catalogPromise = fetch("/api/games").then((res) => res.json());
	const detailsPromise = fetch(
		`/api/game-details?console=${encodeURIComponent(consoleName)}&rom=${encodeURIComponent(rom)}`,
	)
		.then((res) => {
			if (!res.ok) return null;
			return res.json();
		})
		.catch(() => null);

	Promise.all([catalogPromise, detailsPromise])
		.then(([catalog, details]) => {
			const game = FP.findGame(catalog.games, consoleName, rom);
			if (!game) {
				showError("Game not found. It may have been removed from the library.");
				return;
			}
			render(game, details);
		})
		.catch(() => {
			showError("Could not load game data.");
		});

	function showError(msg) {
		document.getElementById("content").style.display = "none";
		const el = document.getElementById("error");
		el.style.display = "";
		el.textContent = msg;
	}

	function render(game, details) {
		const content = document.getElementById("content");
		content.innerHTML = "";

		const hero = document.createElement("div");
		hero.className = "details-hero";

		if (game.hasCover) {
			const img = document.createElement("img");
			img.src = FP.coverUrl(game);
			img.alt = `${gameName} cover art`;
			img.className = "details-cover";
			hero.appendChild(img);
		}

		const meta = document.createElement("div");
		meta.className = "details-meta";

		const title = document.createElement("h2");
		title.className = "details-title";
		title.textContent = details ? details.name : gameName;
		meta.appendChild(title);

		if (details) {
			const info = [];
			if (details.firstReleaseDate)
				info.push(
					`${consoleName} \u00B7 ${details.firstReleaseDate.substring(0, 4)}`,
				);
			else info.push(consoleName);
			if (details.developers?.length)
				info.push(`Developer: ${details.developers.join(", ")}`);
			if (details.publishers?.length)
				info.push(`Publisher: ${details.publishers.join(", ")}`);
			if (details.platforms?.length)
				info.push(`Platforms: ${details.platforms.join(", ")}`);
			if (details.collection) info.push(`Series: ${details.collection}`);

			for (const line of info) {
				const p = document.createElement("p");
				p.className = "details-info-line";
				p.textContent = line;
				meta.appendChild(p);
			}

			if (details.igdbUrl) {
				const p = document.createElement("p");
				p.className = "details-info-line";
				const a = document.createElement("a");
				a.href = details.igdbUrl;
				a.textContent = "View on IGDB";
				a.className = "details-link";
				p.appendChild(a);
				meta.appendChild(p);
			}
		} else {
			const p = document.createElement("p");
			p.className = "details-info-line";
			p.textContent = consoleName;
			meta.appendChild(p);
		}

		hero.appendChild(meta);
		content.appendChild(hero);

		const actions = document.createElement("div");
		actions.className = "details-actions";

		const playLink = document.createElement("a");
		playLink.href = FP.playUrl(game);
		playLink.className = "btn details-action-btn details-play-btn";
		playLink.textContent = "Play";
		actions.appendChild(playLink);

		if (game.hasManual) {
			const manualLink = document.createElement("a");
			manualLink.href = FP.manualUrl(game);
			manualLink.className = "btn details-action-btn details-manual-btn";
			manualLink.textContent = "View Manual";
			actions.appendChild(manualLink);
		}

		content.appendChild(actions);

		if (!details) return;

		if (details.summary) {
			appendSection(content, "Summary", details.summary);
		}

		if (details.storyline) {
			appendSection(content, "Storyline", details.storyline);
		}

		if (details.coverUrl) {
			const link = document.createElement("a");
			link.href = details.coverUrl;
			const img = document.createElement("img");
			img.src = details.coverUrl;
			img.alt = `${details.name} cover art`;
			img.className = "details-cover-full";
			link.appendChild(img);
			appendSectionWithContent(content, "Cover Art", link);
		}

		if (details.screenshots?.length) {
			appendGallery(content, "Screenshots", details.screenshots);
		}

		if (details.artworks?.length) {
			appendGallery(
				content,
				"Artworks",
				details.artworks,
				"details-gallery-full",
			);
		}
	}

	function appendSectionWithContent(parent, heading, contentEl) {
		const section = document.createElement("section");
		section.className = "details-section";
		const h3 = document.createElement("h3");
		h3.textContent = heading;
		section.appendChild(h3);
		section.appendChild(contentEl);
		parent.appendChild(section);
	}

	function appendSection(parent, heading, text) {
		const section = document.createElement("section");
		section.className = "details-section";
		const h3 = document.createElement("h3");
		h3.textContent = heading;
		section.appendChild(h3);
		const p = document.createElement("p");
		p.textContent = text;
		section.appendChild(p);
		parent.appendChild(section);
	}

	function appendGallery(parent, heading, urls, galleryClass) {
		const section = document.createElement("section");
		section.className = "details-section";
		const h3 = document.createElement("h3");
		h3.textContent = heading;
		section.appendChild(h3);
		const gallery = document.createElement("div");
		gallery.className = galleryClass || "details-gallery";
		for (const url of urls) {
			const link = document.createElement("a");
			link.href = url;
			const img = document.createElement("img");
			img.src = url;
			img.loading = "lazy";
			img.alt = heading;
			link.appendChild(img);
			gallery.appendChild(link);
		}
		section.appendChild(gallery);
		parent.appendChild(section);
	}
})();
