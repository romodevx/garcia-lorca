// Shared Tailwind preset for the García Lorca design system.
// apps/web tailwind config does: `presets: [require("@garcia-lorca/config/tailwind")]`.
// Design tokens (color/spacing/typography) are centralized here so the web app
// and any future surface share one source of truth.

/** @type {import("tailwindcss").Config} */
const preset = {
  theme: {
    extend: {
      colors: {
        // Placeholder brand tokens — finalized via the design-system phase.
        brand: {
          DEFAULT: "#1d4ed8",
          fg: "#ffffff"
        }
      }
    }
  },
  plugins: []
};

export default preset;
