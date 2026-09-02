// Registers a DOM for component tests, and tears it down between them.
//
// Bun has no browser environment of its own, so happy-dom supplies
// window/document globally before any test file imports React. Bun also has
// no automatic React Testing Library cleanup, so without the afterEach below
// each render is appended to the same document: getByRole then finds matches
// from earlier tests and fails only when the whole suite runs together —
// green in isolation, red in CI, which is the worst kind of test.
//
// Wired in via bunfig.toml's `preload`, which applies to `bun test src` and
// nothing else — the Playwright suite under web/e2e drives a real browser
// and must not see this.
import { afterEach } from "bun:test";
import { GlobalRegistrator } from "@happy-dom/global-registrator";

GlobalRegistrator.register();

const { cleanup } = await import("@testing-library/react");
afterEach(cleanup);
