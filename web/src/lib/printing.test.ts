// Tests for the print hook registry — the ordering contract printPage() rests
// on: nothing prints until every registered hook has finished, including the
// hooks that only appear *because* an earlier hook ran (read mode mounting a
// diagram), and one broken or wedged hook can never hold the dialog hostage.
import { afterEach, describe, expect, test } from "bun:test";
import {
  preparePrint,
  registerPrintHook,
  restoreAfterPrint,
  type PrintPhase,
} from "./printing.ts";

/** Hooks registered by a test, torn down so module state never leaks. */
const cleanups: (() => void)[] = [];

function register(hook: Parameters<typeof registerPrintHook>[0]) {
  cleanups.push(registerPrintHook(hook));
}

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
  restoreAfterPrint();
});

const tick = (ms = 1) => new Promise((resolve) => setTimeout(resolve, ms));

describe("preparePrint", () => {
  test("resolves only after async hooks have finished", async () => {
    let done = false;
    register(async () => {
      await tick(5);
      done = true;
    });
    await preparePrint();
    expect(done).toBe(true);
  });

  test("passes the phase and runs every hook", async () => {
    const phases: PrintPhase[] = [];
    register((phase) => void phases.push(phase));
    register((phase) => void phases.push(phase));
    await preparePrint();
    expect(phases).toEqual(["print", "print"]);
  });

  test("a rejecting hook does not stop the others", async () => {
    let reached = false;
    register(() => Promise.reject(new Error("mermaid exploded")));
    register(async () => {
      await tick(1);
      reached = true;
    });
    await preparePrint();
    expect(reached).toBe(true);
  });

  test("waits for hooks registered while the pass is running", async () => {
    // The edit → read → print path: switching to read mode mounts diagrams
    // that were not registered when preparation started.
    let lateRan = false;
    register(async () => {
      register(async () => {
        await tick(5);
        lateRan = true;
      });
      await tick(1);
    });
    await preparePrint();
    expect(lateRan).toBe(true);
  });

  test("a wedged hook is capped by the timeout", async () => {
    register(() => new Promise<void>(() => {}));
    const started = Date.now();
    await preparePrint(10);
    expect(Date.now() - started).toBeLessThan(500);
  });

  test("is a no-op until the print ends", async () => {
    // window.print() fires `beforeprint`, which prepares again; re-rendering
    // every diagram a second time at that point would be pure waste.
    let calls = 0;
    register(() => void (calls += 1));
    await preparePrint();
    await preparePrint();
    expect(calls).toBe(1);

    restoreAfterPrint();
    await preparePrint();
    expect(calls).toBe(3); // one "screen" pass, then a fresh "print" pass
  });
});

describe("restoreAfterPrint", () => {
  test("runs the hooks with the screen phase", async () => {
    const phases: PrintPhase[] = [];
    register((phase) => void phases.push(phase));
    await preparePrint();
    restoreAfterPrint();
    expect(phases).toEqual(["print", "screen"]);
  });

  test("does nothing when no print was prepared", () => {
    const phases: PrintPhase[] = [];
    register((phase) => void phases.push(phase));
    restoreAfterPrint();
    expect(phases).toEqual([]);
  });
});

describe("registerPrintHook", () => {
  test("unregistering stops the hook from running", async () => {
    let ran = false;
    const unregister = registerPrintHook(() => void (ran = true));
    unregister();
    await preparePrint();
    expect(ran).toBe(false);
  });
});
