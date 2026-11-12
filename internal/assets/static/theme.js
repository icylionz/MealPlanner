// Apply the light/dark choice before first paint: this device's pick from
// Settings, else the household's preferred mode. Loaded in <head> without
// defer; it is a file rather than inline because the CSP allows no inline script.
try {
  var t = localStorage.getItem("mp-theme");
  if (t !== "light" && t !== "dark") t = document.querySelector('meta[name="household-theme-mode"]').content;
  if (t === "light" || t === "dark") document.documentElement.setAttribute("data-theme", t);
} catch (e) {}
