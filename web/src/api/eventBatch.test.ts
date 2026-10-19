// A burst of doc events must cost one round of refetches, not one per
// event — but no event may be lost, and a long burst must not starve the
// page of updates.
import { describe, expect, test } from "bun:test";
import { batchDocEvents } from "./eventBatch.ts";

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

describe("batchDocEvents", () => {
  test("a burst flushes once, with every path", async () => {
    const flushes: string[][] = [];
    const batch = batchDocEvents((paths) => flushes.push(paths), {
      delayMs: 20,
      maxWaitMs: 500,
    });
    for (let i = 0; i < 50; i++) batch.push(`notes/${i % 10}.md`);
    expect(flushes).toHaveLength(0);
    await wait(60);
    expect(flushes).toHaveLength(1);
    expect(flushes[0]?.sort()).toEqual(
      Array.from({ length: 10 }, (_, i) => `notes/${i}.md`).sort(),
    );
  });

  test("a burst that keeps going still flushes by maxWait", async () => {
    const flushes: string[][] = [];
    const batch = batchDocEvents((paths) => flushes.push(paths), {
      delayMs: 30,
      maxWaitMs: 80,
    });
    const start = Date.now();
    while (Date.now() - start < 200) {
      batch.push("daily/2026-10-18.md");
      await wait(10);
    }
    expect(flushes.length).toBeGreaterThanOrEqual(2);
    batch.cancel();
  });

  test("events after a flush start a new batch", async () => {
    const flushes: string[][] = [];
    const batch = batchDocEvents((paths) => flushes.push(paths), {
      delayMs: 10,
    });
    batch.push("a.md");
    await wait(30);
    batch.push("b.md");
    await wait(30);
    expect(flushes).toEqual([["a.md"], ["b.md"]]);
  });

  test("cancel drops what is pending", async () => {
    const flushes: string[][] = [];
    const batch = batchDocEvents((paths) => flushes.push(paths), {
      delayMs: 10,
    });
    batch.push("a.md");
    batch.cancel();
    await wait(30);
    expect(flushes).toHaveLength(0);
  });
});
