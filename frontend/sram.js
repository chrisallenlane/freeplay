// Pure helper for restoring an SRAM save into the EmulatorJS game
// manager's virtual filesystem. Separated from play.js so it can be
// unit-tested against a stub gameManager without loading EmulatorJS.
//
// `gm` must expose getSaveFilePath(), loadSaveFiles(), and an FS object
// with analyzePath(p), mkdir(p), unlink(p), and writeFile(p, data).
((exports) => {
	exports.restoreSaveToFS = (gm, buf) => {
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
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
