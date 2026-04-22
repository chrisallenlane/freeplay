((exports) => {
	// Logical actions for directional navigation (shared by keyboard and gamepad).
	exports.ACTION_LEFT = "left";
	exports.ACTION_RIGHT = "right";
	exports.ACTION_UP = "up";
	exports.ACTION_DOWN = "down";
	exports.ACTION_ACTIVATE = "activate";
	exports.ACTION_BACK = "back";
	exports.ACTION_PREV_FILTER = "prevFilter";
	exports.ACTION_NEXT_FILTER = "nextFilter";

	exports.readGamepadAction = (gp) => {
		const b = gp.buttons;
		if (b[12]?.pressed) return exports.ACTION_UP;
		if (b[13]?.pressed) return exports.ACTION_DOWN;
		if (b[14]?.pressed) return exports.ACTION_LEFT;
		if (b[15]?.pressed) return exports.ACTION_RIGHT;
		if (b[0]?.pressed || b[9]?.pressed) return exports.ACTION_ACTIVATE;
		if (b[1]?.pressed) return exports.ACTION_BACK;
		if (b[4]?.pressed) return exports.ACTION_PREV_FILTER;
		if (b[5]?.pressed) return exports.ACTION_NEXT_FILTER;

		const ax = gp.axes;
		if (ax.length >= 2) {
			if (ax[1] < -0.5) return exports.ACTION_UP;
			if (ax[1] > 0.5) return exports.ACTION_DOWN;
			if (ax[0] < -0.5) return exports.ACTION_LEFT;
			if (ax[0] > 0.5) return exports.ACTION_RIGHT;
		}

		return null;
	};

	/**
	 * Starts a gamepad poll loop and manages connect/disconnect listeners.
	 * Calls `handleAction` with a logical action string whenever input is read.
	 * @param {(action: string) => void} handleAction
	 */
	exports.gamepadLoop = (handleAction) => {
		// Number of gamepads currently connected.
		let gamepadCount = 0;

		// ID of the running requestAnimationFrame loop, or null when stopped.
		let rafId = null;

		// Debounce state: which logical action is being held and when it last fired.
		let heldAction = null;
		let heldLastFired = 0;

		// Debounce interval (ms).
		const REPEAT_DELAY = 180;

		/**
		 * The main poll loop. Runs every animation frame while a gamepad is connected.
		 * @param {DOMHighResTimeStamp} now
		 */
		function pollGamepads(now) {
			if (gamepadCount === 0) {
				rafId = null;
				return;
			}

			const gamepads = navigator.getGamepads();
			let action = null;

			for (const gp of gamepads) {
				if (!gp) continue;
				const candidate = exports.readGamepadAction(gp);
				if (candidate) {
					action = candidate;
					break;
				}
			}

			if (action === null) {
				// No input — reset debounce state.
				heldAction = null;
				heldLastFired = 0;
			} else if (action !== heldAction) {
				// New action started — fire immediately.
				heldAction = action;
				heldLastFired = now;
				handleAction(action);
			} else {
				// Continuing to hold the same action — repeat after REPEAT_DELAY.
				if (now - heldLastFired >= REPEAT_DELAY) {
					heldLastFired = now;
					handleAction(action);
				}
			}

			rafId = requestAnimationFrame(pollGamepads);
		}

		window.addEventListener("gamepadconnected", () => {
			gamepadCount++;
			if (rafId === null) {
				rafId = requestAnimationFrame(pollGamepads);
			}
		});

		window.addEventListener("gamepaddisconnected", () => {
			gamepadCount = Math.max(0, gamepadCount - 1);
			// The loop will stop itself on the next frame when gamepadCount reaches 0.
		});
	};
})(
	typeof module !== "undefined"
		? (module.exports = globalThis.Freeplay = globalThis.Freeplay || {})
		: (window.Freeplay = window.Freeplay || {}),
);
