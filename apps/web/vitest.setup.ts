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
// jsdom has <dialog> but none of its behaviour. The smallest faithful part:
// showModal opens it, close(returnValue) closes it and fires "close" — which is
// what our native dialogs listen to. (Top layer and inert background are the
// browser's; e2e covers those.)
if (typeof HTMLDialogElement !== "undefined" && typeof HTMLDialogElement.prototype.showModal !== "function") {
  const proto = HTMLDialogElement.prototype as HTMLDialogElement & { returnValue: string };
  Object.defineProperty(proto, "open", {
    configurable: true,
    get(this: HTMLDialogElement) { return this.hasAttribute("open"); },
    set(this: HTMLDialogElement, value: boolean) { this.toggleAttribute("open", value); },
  });
  proto.returnValue = "";
  proto.showModal = function showModal(this: HTMLDialogElement): void {
    this.open = true;
  };
  proto.close = function close(this: HTMLDialogElement & { returnValue: string }, returnValue?: string): void {
    if (!this.open) return;
    if (returnValue !== undefined) this.returnValue = returnValue;
    this.open = false;
    this.dispatchEvent(new Event("close"));
  };
}
if (typeof Element !== "undefined" && typeof Element.prototype.scrollIntoView !== "function") {
  Element.prototype.scrollIntoView = function scrollIntoView(): void {};
}
