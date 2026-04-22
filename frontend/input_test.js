var { describe, it } = require("node:test");
var assert = require("node:assert/strict");
var FP = require("./input.js");

describe("readGamepadAction", () => {
	function makeGamepad(overrides) {
		var buttons = Array.from({ length: 16 }, () => ({ pressed: false }));
		var gp = { buttons: buttons, axes: [0, 0] };
		if (overrides) overrides(gp);
		return gp;
	}

	it("returns null when nothing is pressed", () => {
		assert.equal(FP.readGamepadAction(makeGamepad()), null);
	});

	it("maps D-pad up (button 12) to ACTION_UP", () => {
		var gp = makeGamepad((g) => (g.buttons[12].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("maps D-pad down (button 13) to ACTION_DOWN", () => {
		var gp = makeGamepad((g) => (g.buttons[13].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_DOWN);
	});

	it("maps D-pad left (button 14) to ACTION_LEFT", () => {
		var gp = makeGamepad((g) => (g.buttons[14].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_LEFT);
	});

	it("maps D-pad right (button 15) to ACTION_RIGHT", () => {
		var gp = makeGamepad((g) => (g.buttons[15].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_RIGHT);
	});

	it("maps button 0 (A/Cross) to ACTION_ACTIVATE", () => {
		var gp = makeGamepad((g) => (g.buttons[0].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_ACTIVATE);
	});

	it("maps button 9 (Start) to ACTION_ACTIVATE", () => {
		var gp = makeGamepad((g) => (g.buttons[9].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_ACTIVATE);
	});

	it("maps button 1 (B/Circle) to ACTION_BACK", () => {
		var gp = makeGamepad((g) => (g.buttons[1].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_BACK);
	});

	it("maps L1 (button 4) to ACTION_PREV_FILTER", () => {
		var gp = makeGamepad((g) => (g.buttons[4].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_PREV_FILTER);
	});

	it("maps R1 (button 5) to ACTION_NEXT_FILTER", () => {
		var gp = makeGamepad((g) => (g.buttons[5].pressed = true));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_NEXT_FILTER);
	});

	it("falls back to axes when no buttons pressed", () => {
		var gp = makeGamepad((g) => (g.axes = [0, -0.8]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("maps positive Y axis to ACTION_DOWN", () => {
		var gp = makeGamepad((g) => (g.axes = [0, 0.8]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_DOWN);
	});

	it("maps negative X axis to ACTION_LEFT", () => {
		var gp = makeGamepad((g) => (g.axes = [-0.8, 0]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_LEFT);
	});

	it("maps positive X axis to ACTION_RIGHT", () => {
		var gp = makeGamepad((g) => (g.axes = [0.8, 0]));
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_RIGHT);
	});

	it("ignores axes below threshold", () => {
		var gp = makeGamepad((g) => (g.axes = [0.3, -0.4]));
		assert.equal(FP.readGamepadAction(gp), null);
	});

	it("buttons take priority over axes", () => {
		var gp = makeGamepad((g) => {
			g.buttons[12].pressed = true;
			g.axes = [0.8, 0];
		});
		assert.equal(FP.readGamepadAction(gp), FP.ACTION_UP);
	});

	it("handles gamepads with fewer than 16 buttons", () => {
		var gp = { buttons: [{ pressed: false }], axes: [0, 0] };
		assert.equal(FP.readGamepadAction(gp), null);
	});
});
