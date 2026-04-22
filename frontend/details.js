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

			const navItems = [
				document.querySelector("a.header-btn"),
				document.querySelector(".details-play-btn"),
				document.querySelector(".details-manual-btn"),
			].filter(Boolean);

			let navIndex = Math.min(1, navItems.length - 1);

			function highlight(el) {
				for (const item of navItems) {
					item.classList.remove("highlighted");
				}
				if (el) {
					el.classList.add("highlighted");
					el.scrollIntoView({ behavior: "smooth", block: "nearest" });
				}
			}

			function handleAction(action) {
				switch (action) {
					case FP.ACTION_UP:
						if (navIndex > 0) {
							navIndex--;
							highlight(navItems[navIndex]);
						}
						break;
					case FP.ACTION_DOWN:
						if (navIndex < navItems.length - 1) {
							navIndex++;
							highlight(navItems[navIndex]);
						}
						break;
					case FP.ACTION_ACTIVATE:
						navItems[navIndex]?.click();
						break;
					case FP.ACTION_BACK:
						window.location.href = "/";
						break;
				}
			}

			highlight(navItems[navIndex]);
			FP.gamepadLoop(handleAction);
		})
		.catch(() => {
			FP.showError("content", "Could not load game data.");
		});

	function render(game, details) {
		const displayName = details?.name || gameName;
		document.title = `Freeplay - ${displayName}`;

		const content = document.getElementById("content");
		content.innerHTML = "";

		const hero = FP.el("div", "details-hero");

		if (game.hasCover) {
			const img = document.createElement("img");
			img.src = FP.coverUrl(game);
			img.alt = `${gameName} cover art`;
			img.className = "details-cover";
			hero.appendChild(img);
		}

		const meta = FP.el("div", "details-meta");

		const title = FP.el(
			"h2",
			"details-title",
			details ? details.name : gameName,
		);
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

		const table = FP.el("table", "details-meta-table");
		for (const [label, value] of rows) {
			const tr = FP.el("tr");
			tr.append(FP.el("th", null, label), FP.el("td", null, value));
			table.appendChild(tr);
		}
		meta.appendChild(table);

		if (details?.igdbUrl) {
			const a = FP.el("a", "details-link", "View on IGDB");
			a.href = details.igdbUrl;
			meta.appendChild(a);
		}

		hero.appendChild(meta);
		content.appendChild(hero);

		const actions = FP.el("div", "details-actions");

		const playLink = FP.el(
			"a",
			"btn details-action-btn details-play-btn",
			"Play",
		);
		playLink.href = FP.playUrl(game);
		actions.appendChild(playLink);

		if (game.hasManual) {
			const manualLink = FP.el(
				"a",
				"btn details-action-btn details-manual-btn",
				"View Manual",
			);
			manualLink.href = FP.manualUrl(game);
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
		const section = FP.el("section", "details-section");
		section.appendChild(FP.el("h3", null, heading));
		if (typeof content === "string") {
			for (const para of content.split(/\n\n+/)) {
				const text = para.trim();
				if (!text) continue;
				section.appendChild(FP.el("p", null, text));
			}
		} else {
			section.appendChild(content);
		}
		parent.appendChild(section);
	}

	function buildGallery(heading, urls, galleryClass) {
		const gallery = FP.el("div", galleryClass || "details-gallery");
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
