import "@testing-library/jest-dom/vitest";

// jsdom lacks a few browser APIs that cmdk (and Radix under it) touch on mount.
// Shim them so component tests exercise real behavior instead of crashing.
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
