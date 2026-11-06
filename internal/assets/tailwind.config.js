/** @type {import('tailwindcss').Config} */
// Build-time Tailwind config shared with RHWallet (Backbone design system).
// Compiled by internal/assets/build-css.sh; the browser never runs Tailwind.
module.exports = {
  content: [
    "./internal/**/*.templ",
    "./internal/**/*.go",
    "./internal/assets/static/app.js",
  ],
  theme: {
    extend: {
      colors: {
        primary: "#22386A",
        // Backbone palette remap: neutral == cool gray, danger == rose,
        // success == emerald, warning == amber, accent navy == indigo.
        slate: {
          50: "#F4F5F7", 100: "#ECEEF1", 200: "#E6E8EC", 300: "#D5D9DF",
          400: "#A8AEB7", 500: "#7C828C", 600: "#5A616C", 700: "#3B4048",
          800: "#262A31", 900: "#14161A", 950: "#0E1115",
        },
        indigo: {
          50: "#EBEEF6", 100: "#D7DEED", 200: "#C8D2E6", 300: "#9FB0D1",
          400: "#5874AE", 500: "#3A528A", 600: "#22386A", 700: "#1B2E59",
          800: "#152444", 900: "#101B34", 950: "#0A1223",
        },
        emerald: {
          50: "#E4F4EC", 100: "#C7E9D7", 200: "#9BD9B8", 300: "#63C293",
          400: "#38A873", 500: "#1E8E5A", 600: "#197A4D", 700: "#156240",
          800: "#124E34", 900: "#0E3F2B", 950: "#072218",
        },
        rose: {
          50: "#FBE9E8", 100: "#F7D2D0", 200: "#F0ABA8", 300: "#E67F7B",
          400: "#DA5B56", 500: "#CE3B36", 600: "#B5302B", 700: "#952622",
          800: "#771F1C", 900: "#601A17", 950: "#350C0A",
        },
        amber: {
          50: "#FAEFDC", 100: "#F3DCB4", 200: "#E9C07C", 300: "#DDA24A",
          400: "#C98A27", 500: "#B57415", 600: "#9A6212", 700: "#7C4F0F",
          800: "#653F0D", 900: "#53340B", 950: "#2F1C05",
        },
      },
      fontFamily: {
        display: ["Space Grotesk", "Hanken Grotesk", "sans-serif"],
        body: ["Hanken Grotesk", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      },
      borderRadius: {
        DEFAULT: "0.625rem",
        lg: "0.875rem",
        xl: "1.25rem",
        full: "9999px",
      },
    },
  },
  plugins: [require("@tailwindcss/forms")],
};
