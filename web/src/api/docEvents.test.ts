// The event stream is how an edit made elsewhere — an agent over MCP, vim,
// another tab — reaches an open page. Its failure is silent by nature: the
// page simply stops updating. So the rule under test is that no way of
// losing the stream leaves the page stale for good.
import { afterEach, describe, expect, test } from "bun:test";
import { openDocEventStream } from "./docEvents.ts";
import type { DocEvent } from "./types.ts";

class FakeSource extends EventTarget {
  readyState = 0;
  closed = false;
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  open() {
    this.readyState = 1;
    this.dispatchEvent(new Event("open"));
  }
  send(type: string, data = "{}") {
    this.dispatchEvent(new MessageEvent(type, { data }));
  }
  /** The browser's own retry is under way (CONNECTING), or it gave up. */
  fail(gaveUp = false) {
    this.readyState = gaveUp ? 2 : 0;
    this.dispatchEvent(new Event("error"));
  }
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function start(silenceMs = 40) {
  const sources: FakeSource[] = [];
  const events: DocEvent[] = [];
  let resyncs = 0;
  const stop = openDocEventStream({
    onEvent: (event) => events.push(event),
    onResync: () => resyncs++,
    silenceMs,
    initialRetryMs: 10,
    maxRetryMs: 20,
    createSource: () => {
      const source = new FakeSource();
      sources.push(source);
      return source as unknown as EventSource;
    },
  });
  stops.push(stop);
  return { sources, events, resyncs: () => resyncs, stop };
}

const stops: (() => void)[] = [];
afterEach(() => {
  for (const stop of stops.splice(0)) stop();
});

describe("openDocEventStream", () => {
  test("delivers doc events and does not re-read on first connect", () => {
    const stream = start();
    stream.sources[0]!.open();
    stream.sources[0]!.send("doc", '{"path":"notes/x.md","action":"upsert"}');
    stream.sources[0]!.send("doc", "not json");
    expect(stream.events).toEqual([{ path: "notes/x.md", action: "upsert" }]);
    expect(stream.resyncs()).toBe(0);
  });

  test("pings keep a quiet stream alive", async () => {
    const stream = start();
    stream.sources[0]!.open();
    for (let i = 0; i < 4; i++) {
      await wait(20);
      stream.sources[0]!.send("ping");
    }
    expect(stream.sources).toHaveLength(1);
    expect(stream.resyncs()).toBe(0);
  });

  test("a stream that goes silent is replaced, and the page re-reads", async () => {
    const stream = start();
    stream.sources[0]!.open();
    await wait(70);
    expect(stream.sources[0]!.closed).toBe(true);
    expect(stream.sources.length).toBeGreaterThanOrEqual(2);
    expect(stream.resyncs()).toBe(0); // nothing to trust until it reopens
    stream.sources.at(-1)!.open();
    expect(stream.resyncs()).toBe(1);
  });

  test("a stream that never opens still re-reads on every lap", async () => {
    const stream = start();
    await wait(110);
    expect(stream.sources.length).toBeGreaterThanOrEqual(3);
    expect(stream.resyncs()).toBeGreaterThanOrEqual(2);
  });

  test("the browser's own reconnect re-reads once it is back", () => {
    const stream = start();
    stream.sources[0]!.open();
    stream.sources[0]!.fail();
    stream.sources[0]!.open();
    expect(stream.sources).toHaveLength(1);
    expect(stream.resyncs()).toBe(1);
  });

  test("a stream the browser gave up on is reopened with backoff", async () => {
    const stream = start(1_000);
    stream.sources[0]!.open();
    stream.sources[0]!.fail(true);
    await wait(30);
    expect(stream.sources).toHaveLength(2);
    stream.sources[1]!.open();
    expect(stream.resyncs()).toBe(1);
  });

  test("coming back to a tab whose stream died while hidden reconnects at once", async () => {
    const stream = start(1_000);
    stream.sources[0]!.open();
    const realNow = Date.now;
    Date.now = () => realNow() + 5_000; // the machine slept
    try {
      document.dispatchEvent(new Event("visibilitychange"));
    } finally {
      Date.now = realNow;
    }
    expect(stream.sources).toHaveLength(2);
    stream.sources[1]!.open();
    expect(stream.resyncs()).toBe(1);
  });

  test("stopping closes the stream and ends the watchdog", async () => {
    const stream = start();
    stream.sources[0]!.open();
    stream.stop();
    await wait(70);
    expect(stream.sources).toHaveLength(1);
    expect(stream.sources[0]!.closed).toBe(true);
  });
});
