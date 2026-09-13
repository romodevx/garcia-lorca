import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    // THE SAME LESSON apps/web already learned, never applied here.
    //
    // Without it these tests inherit vitest's 5s default, and on 2026-09-12 the
    // deploy died on it: "lists all commands when open with an empty query" is
    // a SYNCHRONOUS render with no waiting in it, and it still blew past five
    // seconds on a contended Cloud Build worker — one that spent 80s just
    // standing jsdom up and 2.6s rendering a single Button. Nothing about the
    // code had changed; the build simply landed on a slow machine.
    //
    // That is the worst kind of gate: it fails for a reason nobody can see, on
    // a commit that is fine, and it blocks every deploy behind it until someone
    // re-runs it and gets a faster worker.
    //
    // Kept EQUAL to apps/web/vitest.config.ts. Two suites with two budgets is
    // how one goes green and the other red for no reason in the code.
    testTimeout: 30_000,
    hookTimeout: 30_000
  }
});
