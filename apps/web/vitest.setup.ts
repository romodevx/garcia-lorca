import "@testing-library/jest-dom/vitest";
import { configure } from "@testing-library/react";

// Testing Library keeps its OWN timeout for findBy*/waitFor, default 1000ms, and
// it is INDEPENDENT of vitest's testTimeout — raising one does nothing for the
// other. That bit us on Cloud Build: the whole suite runs in ~5s on a dev machine
// and ~520s on the default e2-medium worker, so a `findByText` that resolves
// instantly here had well under a second of headroom there and failed with
// "Unable to find an element with the text".
//
// The tests are not flaky in behaviour — they are bounded by how fast jsdom can
// render on the machine underneath. A build that fails for the worker's speed
// rather than for the code is the worst kind of red.
configure({ asyncUtilTimeout: 15_000 });

// jsdom lacks a few browser APIs cmdk (and Radix under it) touch on mount.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  };
}
if (typeof Element !== "undefined" && typeof Element.prototype.scrollIntoView !== "function") {
  Element.prototype.scrollIntoView = function scrollIntoView(): void {};
}
