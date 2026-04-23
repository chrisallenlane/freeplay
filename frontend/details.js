(() => {
	const FP = window.Freeplay;

	const subpage = FP.initSubpage();
	if (!subpage) {
		FP.showError("content", "Missing console or rom parameter.");
		return;
	}
	const { consoleName, rom, gameName } = subpage;

	const gamePromise = FP.loadGame(consoleName, rom, "content");
	const detailsPromise = FP.loadGameDetails(consoleName, rom);

	Promise.all([gamePromise, detailsPromise])
		.then(([game, details]) => {
			if (!game) return;
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
					el.scrollIntoView({
						behavior: FP.prefersReducedMotion() ? "instant" : "smooth",
						block: "nearest",
					});
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
			const img = FP.el("img", "details-cover");
			img.src = FP.coverUrl(game);
			img.alt = `${gameName} cover art`;
			hero.appendChild(img);
		}

		const meta = FP.el("div", "details-meta");

		const title = FP.el("h2", "details-title", displayName);
		meta.appendChild(title);

		const rows = [
			["Console", consoleName],
			["Year", details?.firstReleaseDate?.substring(0, 4)],
			["Developer", details?.developers?.join(", ")],
			["Publisher", details?.publishers?.join(", ")],
			["Platforms", details?.platforms?.join(", ")],
			["Series", details?.collection],
		].filter(([, v]) => v);

		const table = FP.el("table", "details-meta-table");
		for (const [label, value] of rows) {
			const tr = FP.el("tr");
			tr.append(FP.el("th", null, label), FP.el("td", null, value));
			table.appendChild(tr);
		}
		meta.appendChild(table);

		const igdbHref = FP.safeExternalHref(details?.igdbUrl);
		if (igdbHref) {
			const a = FP.el("a", "details-link", "View on IGDB");
			a.href = igdbHref;
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
			const link = FP.el("a");
			link.href = details.coverUrl;
			const img = FP.el("img", "details-cover-full");
			img.src = details.coverUrl;
			// Cover renders below the fold; deprioritize its byte cost
			// so screenshots/artworks aren't starved on first paint.
			img.loading = "lazy";
			img.alt = `${displayName} cover art`;
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

	function buildGallery(heading, refs, galleryClass) {
		const gallery = FP.el("div", galleryClass || "details-gallery");
		for (let i = 0; i < refs.length; i++) {
			// Each ref is {url, thumbUrl?}. Older details.json written
			// before PERF-6 decode to {url: "..."} with no thumbUrl;
			// fall back to url in that case.
			const ref = refs[i];
			const fullUrl = ref.url;
			const thumbUrl = ref.thumbUrl || ref.url;
			const link = FP.el("a");
			link.href = fullUrl;
			link.setAttribute(
				"aria-label",
				`View full image: ${heading} ${i + 1} of ${refs.length}`,
			);
			const img = FP.el("img");
			img.src = thumbUrl;
			img.loading = "lazy";
			img.alt = `${heading} ${i + 1} of ${refs.length}`;
			link.appendChild(img);
			gallery.appendChild(link);
		}
		return gallery;
	}
})();
