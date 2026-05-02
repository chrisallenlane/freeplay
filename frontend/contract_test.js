// Contract test for the frontend↔server save endpoint.
//
// Loads the same urls.js and postSave.js the browser executes, spawns the
// real freeplay binary against a minimal data fixture this test owns,
// then verifies the URL the frontend would build for a catalog game
// round-trips correctly through the server. Failures here mean the URL
// convention drifted on one side without the other.
//
// Requires ./dist/freeplay to exist. Make's test target depends on build.

const { describe, before, after, it } = require("node:test");
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const fs = require("node:fs/promises");

require("./urls.js");
require("./postSave.js");
const FP = globalThis.Freeplay;

const REPO_ROOT = path.resolve(__dirname, "..");
const BINARY = path.join(REPO_ROOT, "dist", "freeplay");

// Minimal freeplay.toml the binary needs to start. The contract test
// passes -port on the CLI, so the toml port value is overridden.
const FIXTURE_TOML = `port = 8080

[roms.NES]
path = "roms/nes"
core = "fceumm"
`;

// Asks the OS for an unused port. Brief race window between close and
// freeplay's own bind, but acceptable for a single-developer test suite.
function findFreePort() {
	return new Promise((resolve, reject) => {
		const srv = net.createServer();
		srv.unref();
		srv.on("error", reject);
		srv.listen(0, "127.0.0.1", () => {
			const { port } = srv.address();
			srv.close(() => resolve(port));
		});
	});
}

async function waitForHealth(baseURL, timeoutMs = 10000) {
	const deadline = Date.now() + timeoutMs;
	let lastErr;
	while (Date.now() < deadline) {
		try {
			const res = await fetch(`${baseURL}/api/health`);
			if (res.ok) return;
		} catch (err) {
			lastErr = err;
		}
		await new Promise((r) => setTimeout(r, 50));
	}
	throw new Error(`server did not become healthy: ${lastErr}`);
}

let server;
let baseURL;
let dataDir;

// Builds a fresh, self-contained data directory under os.tmpdir() with
// a minimal freeplay.toml and a single ROM file. The repo's testdata/
// is gitignored, so the test cannot depend on it being present in CI.
async function setupDataDir() {
	const dir = await fs.mkdtemp(path.join(os.tmpdir(), "freeplay-contract-"));
	await fs.writeFile(path.join(dir, "freeplay.toml"), FIXTURE_TOML);
	const romsDir = path.join(dir, "roms", "nes");
	await fs.mkdir(romsDir, { recursive: true });
	// Scanner only lists directory entries; ROM contents are irrelevant.
	await fs.writeFile(path.join(romsDir, "Mega Man.nes"), "");
	return dir;
}

before(async () => {
	dataDir = await setupDataDir();
	const port = await findFreePort();
	baseURL = `http://127.0.0.1:${port}`;
	server = spawn(BINARY, ["-data", dataDir, "-port", String(port)], {
		stdio: ["ignore", "pipe", "pipe"],
	});
	server.on("error", (err) => {
		throw new Error(`failed to spawn freeplay: ${err.message}`);
	});
	await waitForHealth(baseURL);
});

after(async () => {
	if (server && !server.killed) {
		server.kill();
		await new Promise((resolve) => server.on("exit", resolve));
	}
	if (dataDir) {
		await fs.rm(dataDir, { recursive: true, force: true });
	}
});

describe("contract: frontend save URL round-trips through the server", () => {
	it("posts and gets a save using saveBasePath + stripExt + postSave", async () => {
		// 1. Fetch the catalog the way the frontend would.
		const catalogRes = await fetch(`${baseURL}/api/games`);
		assert.equal(catalogRes.status, 200);
		const catalog = await catalogRes.json();
		assert.ok(catalog.games.length > 0, "testdata catalog must not be empty");
		const game = catalog.games[0];

		// 2. Build the save URL exactly as play.js does:
		//    gameName = stripExt(rom); saveBase = saveBasePath(console, gameName).
		const saveBase = FP.saveBasePath(game.console, FP.stripExt(game.filename));

		// 3. Inject a fetch that prefixes the absolute baseURL — the
		//    browser's fetch resolves relative URLs against location.origin;
		//    Node's fetch requires absolute URLs.
		const fetchImpl = (url, init) => fetch(`${baseURL}${url}`, init);

		const saveData = new Uint8Array([0xde, 0xad, 0xbe, 0xef, 0x00, 0xff]);

		// 4. POST via the real postSave helper.
		const postRes = await FP.postSave(saveBase, "sram", saveData, fetchImpl);
		assert.ok(postRes, "postSave should return a Response");
		assert.equal(postRes.status, 200);

		// 5. GET the same URL the frontend's onGameStart hook uses.
		const getRes = await fetchImpl(`${saveBase}/sram`);
		assert.equal(getRes.status, 200);
		const body = new Uint8Array(await getRes.arrayBuffer());
		assert.deepEqual(body, saveData);
		// after() rms the whole tmp dataDir, so no per-test cleanup needed.
	});

	it("rejects POSTs to a slug not in the catalog with 404", async () => {
		const saveBase = FP.saveBasePath("NES", "Definitely Not A Game");
		const fetchImpl = (url, init) => fetch(`${baseURL}${url}`, init);
		const res = await FP.postSave(
			saveBase,
			"sram",
			new Uint8Array([1]),
			fetchImpl,
		);
		assert.equal(res.status, 404);
	});
});
