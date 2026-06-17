import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@garcia-lorca/ui/flags": resolve(__dirname, "../../packages/ui/src/flags/index.ts"),
      "@garcia-lorca/ui": resolve(__dirname, "../../packages/ui/index.ts"),
      "@garcia-lorca/domain": resolve(__dirname, "../../packages/domain/index.ts"),
      "@garcia-lorca/i18n": resolve(__dirname, "../../packages/i18n/index.ts"),
      "@garcia-lorca/api-client": resolve(__dirname, "../../packages/api-client/index.ts")
    }
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["**/*.test.{ts,tsx}"]
  }
});
