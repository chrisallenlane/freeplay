// Pre-DOM theme attribute set. Runs synchronously at script load so the
// correct theme is applied before CSS paints — prevents a flash of the
// wrong theme when a user has saved a non-default preference.
(() => {
	var saved =
		typeof localStorage !== "undefined"
			? localStorage.getItem("freeplay-theme")
			: null;
	if (typeof document === "undefined") return;
	document.documentElement.dataset.theme =
		saved ||
		(matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
})();

((exports) => {
	exports.initThemeToggle = () => {
		const btn = document.getElementById("theme-toggle");
		if (!btn) return;

		const update = () => {
			btn.textContent =
				document.documentElement.dataset.theme === "light" ? "☽" : "☀";
		};

		btn.addEventListener("click", () => {
			const next =
				document.documentElement.dataset.theme === "light" ? "dark" : "light";
			document.documentElement.dataset.theme = next;
			localStorage.setItem("freeplay-theme", next);
			update();
		});

		update();
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
