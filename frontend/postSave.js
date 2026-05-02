// Posts save data to the given saveBase + type endpoint.
// Returns the fetch Response promise (or undefined when data is
// falsy and the request was skipped). Logs to console.error on
// non-2xx or network failure — fetch() resolves on 4xx/5xx so
// res.ok must be checked explicitly to surface server-side
// failures instead of swallowing them.
//
// fetchImpl is injected for testing; production callers omit it
// and get the global fetch.
((exports) => {
	exports.postSave = (saveBase, type, data, fetchImpl = fetch) => {
		if (!data) return undefined;
		// An empty Uint8Array / ArrayBuffer is truthy in JS but POSTing
		// zero bytes would overwrite a valid save with nothing. Treat
		// zero-length payloads as a no-op like falsy data.
		const size = data.byteLength ?? data.length;
		if (size === 0) return undefined;
		return fetchImpl(`${saveBase}/${type}`, {
			method: "POST",
			headers: { "X-Requested-With": "freeplay" },
			body: new Blob([data]),
		})
			.then((res) => {
				if (!res.ok) console.error(`Save failed (${type}): HTTP ${res.status}`);
				return res;
			})
			.catch((err) => {
				console.error(`Save failed (${type}):`, err);
				// Returns undefined so callers don't see unhandledrejection
				// in the browser console — postSave is fire-and-forget on
				// the EmulatorJS save hooks.
			});
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
