/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/view/**/*.templ", "./internal/view/**/*_templ.go", "./web/static/js/**/*.js"],
  theme: {
    extend: {
      fontFamily: {
        display: "var(--font-display)",
        body: "var(--font-body)",
        mono: "var(--font-mono)",
      },
    },
  },
  corePlugins: { preflight: true },
  plugins: [],
};
