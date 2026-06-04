import { describe, expect, test } from "bun:test";
import { applyHitchResponse } from "./index";

describe("applyHitchResponse", () => {
  test("mutates event.input in place for tool_call transform responses", () => {
    const event = { input: { command: "pwd", timeout: 1 } };

    const result = applyHitchResponse(event, {
      adapter_action: "mutate_and_return",
      mutations: [{ path: ["input", "command"], value: "echo ok" }],
    });

    expect(result).toBeUndefined();
    expect(event.input).toEqual({ command: "echo ok", timeout: 1 });
  });

  test("returns Pi control value unchanged", () => {
    const response = { action: "handled" };

    const result = applyHitchResponse({}, {
      adapter_action: "return",
      return_value: response,
    });

    expect(result).toBe(response);
  });

  test("returns patch object for tool_result replacement", () => {
    const patch = { output: "replacement" };

    const result = applyHitchResponse({}, {
      adapter_action: "return",
      return_value: patch,
    });

    expect(result).toEqual(patch);
  });

  test("recursion guard no-ops without mutating or returning", () => {
    const previous = process.env.HITCH_CHILD;
    process.env.HITCH_CHILD = "1";
    const event = { input: { command: "pwd" } };

    try {
      const result = applyHitchResponse(event, {
        adapter_action: "mutate_and_return",
        return_value: { block: true },
        mutations: [{ path: ["input", "command"], value: "echo should-not-apply" }],
      });

      expect(result).toBeUndefined();
      expect(event.input.command).toBe("pwd");
    } finally {
      if (previous === undefined) {
        delete process.env.HITCH_CHILD;
      } else {
        process.env.HITCH_CHILD = previous;
      }
    }
  });
});
