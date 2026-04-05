(() => {
	const FP = window.Freeplay;

	const subpage = FP.initSubpage();
	if (!subpage) {
		FP.showError("content", "Missing console or rom parameter.");
		return;
	}
	const { consoleName, rom, gameName } = subpage;

	const catalogPromise = fetch("/api/games").then((res) => res.json());
	const detailsPromise = fetch(FP.gameDetailsUrl(consoleName, rom))
		.then((res) => {
			if (!res.ok) return null;
			return res.json();
		})
		.catch(() => null);

	Promise.all([catalogPromise, detailsPromise])
		.then(([catalog, details]) => {
			const game = FP.findGame(catalog.games, consoleName, rom);
			if (!game) {
				FP.showError(
					"content",
					"Game not found. It may have been removed from the library.",
				);
				return;
			}
			render(game, details);
		})
		.catch(() => {
			FP.showError("content", "Could not load game data.");
		});

	function render(game, details) {
		const displayName = details?.name || gameName;
		document.title = `Freeplay - ${displayName}`;

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

		const rows = [];
		if (details) {
			rows.push(["Console", consoleName]);
			if (details.firstReleaseDate)
				rows.push(["Year", details.firstReleaseDate.substring(0, 4)]);
			if (details.developers?.length)
				rows.push(["Developer", details.developers.join(", ")]);
			if (details.publishers?.length)
				rows.push(["Publisher", details.publishers.join(", ")]);
			if (details.platforms?.length)
				rows.push(["Platforms", details.platforms.join(", ")]);
			if (details.collection) rows.push(["Series", details.collection]);
		} else {
			rows.push(["Console", consoleName]);
		}

		const table = document.createElement("table");
		table.className = "details-meta-table";
		for (const [label, value] of rows) {
			const tr = document.createElement("tr");
			const th = document.createElement("th");
			th.textContent = label;
			const td = document.createElement("td");
			td.textContent = value;
			tr.appendChild(th);
			tr.appendChild(td);
			table.appendChild(tr);
		}
		meta.appendChild(table);

		if (details?.igdbUrl) {
			const a = document.createElement("a");
			a.href = details.igdbUrl;
			a.textContent = "View on IGDB";
			a.className = "details-link";
			meta.appendChild(a);
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
			appendSection(content, "Cover Art", link);
		}

		if (details.screenshots?.length) {
			appendSection(
				content,
				"Screenshots",
				buildGallery("Screenshots", details.screenshots),
			);
		}

		if (details.artworks?.length) {
			appendSection(
				content,
				"Artworks",
				buildGallery("Artworks", details.artworks, "details-gallery-full"),
			);
		}
	}

	function appendSection(parent, heading, content) {
		const section = document.createElement("section");
		section.className = "details-section";
		const h3 = document.createElement("h3");
		h3.textContent = heading;
		section.appendChild(h3);
		if (typeof content === "string") {
			for (const para of content.split(/\n\n+/)) {
				const text = para.trim();
				if (!text) continue;
				const p = document.createElement("p");
				p.textContent = text;
				section.appendChild(p);
			}
		} else {
			section.appendChild(content);
		}
		parent.appendChild(section);
	}

	function buildGallery(heading, urls, galleryClass) {
		const gallery = document.createElement("div");
		gallery.className = galleryClass || "details-gallery";
		for (let i = 0; i < urls.length; i++) {
			const link = document.createElement("a");
			link.href = urls[i];
			const img = document.createElement("img");
			img.src = urls[i];
			img.loading = "lazy";
			img.alt = `${heading} ${i + 1} of ${urls.length}`;
			link.setAttribute(
				"aria-label",
				`View full image: ${heading} ${i + 1} of ${urls.length}`,
			);
			link.appendChild(img);
			gallery.appendChild(link);
		}
		return gallery;
	}
})();
