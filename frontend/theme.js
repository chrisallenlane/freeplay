// initThemeToggle wires up the header theme-switch button. The
// data-theme attribute itself is set by a small inline <script> in
// each page's <head> so the correct theme is applied before CSS paints
// — see PERF-10. This module is loaded with `defer` and only runs
// after the DOM is ready.
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
