// Tailwind only supplies layout utilities here. Every colour, radius and font
// resolves to a Household design-system token (css/tokens.css); the default
// spacing scale already matches the system's 4px steps (gap-4 = space-4).
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/ui/**/*.templ", "./internal/ui/*.go", "./internal/assets/static/app.js"],
  theme: {
    colors: {
      transparent: "transparent",
      current: "currentColor",
      ground: "var(--surface-ground)",
      card: "var(--surface-card)",
      sunk: "var(--surface-sunk)",
      raised: "var(--surface-raised)",
      ink: { DEFAULT: "var(--ink)", muted: "var(--ink-muted)", subtle: "var(--ink-subtle)" },
      line: { hair: "var(--line-hair)", strong: "var(--line-strong)" },
      primary: { DEFAULT: "var(--primary)", soft: "var(--primary-soft)", ink: "var(--primary-ink)" },
      meals: { DEFAULT: "var(--app-meals)", soft: "var(--app-meals-soft)", ink: "var(--app-meals-ink)" },
      success: "var(--status-success)",
      danger: "var(--status-danger)",
      warning: "var(--status-warning)",
      info: "var(--status-info)",
    },
    borderRadius: {
      none: "0",
      s: "var(--radius-s)",
      m: "var(--radius-m)",
      l: "var(--radius-l)",
      xl: "var(--radius-xl)",
      pill: "var(--radius-pill)",
    },
    fontFamily: {
      display: "var(--font-display)",
      body: "var(--font-body)",
      hand: "var(--font-hand)",
    },
    screens: { md: "640px", lg: "960px" },
  },
  corePlugins: { preflight: false },
};
