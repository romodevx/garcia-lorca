import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@garcia-lorca/ui/flags": resolve(__dirname, "../../packages/ui/src/flags/index.ts"),
      "@garcia-lorca/ui/command-core": resolve(__dirname, "../../packages/ui/src/command/core.ts"),
      "@garcia-lorca/ui": resolve(__dirname, "../../packages/ui/index.ts"),
      "@garcia-lorca/domain/student-name": resolve(__dirname, "../../packages/domain/src/studentName.ts"),
      "@garcia-lorca/domain/grade-average": resolve(__dirname, "../../packages/domain/src/gradeAverage.ts"),
      "@garcia-lorca/domain": resolve(__dirname, "../../packages/domain/index.ts"),
      "@garcia-lorca/i18n": resolve(__dirname, "../../packages/i18n/index.ts"),
      "@garcia-lorca/api-client": resolve(__dirname, "../../packages/api-client/index.ts")
    }
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["**/*.test.{ts,tsx}"],
    // Vitest defaults to 5s. That is comfortable here (the whole suite runs in
    // ~5s on a dev machine) and far too tight on Cloud Build's default e2-medium
    // worker, where the same suite takes ~300s and the heaviest test — a full
    // login → dashboard → finance → logout flow driven by userEvent — blew past
    // 5s and failed the deploy.
    //
    // This is a machine-speed accommodation, not a flaky test being papered
    // over: the flow passes deterministically, it is simply bounded by how fast
    // jsdom and userEvent can run. Kept generous so a slower CI worker never
    // fails a build for a reason that has nothing to do with the code.
    testTimeout: 30_000,
    hookTimeout: 30_000
  }
});
