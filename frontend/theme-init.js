(() => {
	var saved = localStorage.getItem("freeplay-theme");
	document.documentElement.dataset.theme =
		saved ||
		(matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
})();
