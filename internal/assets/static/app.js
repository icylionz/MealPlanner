// Small progressive enhancements. Every flow works without this file.
(function () {
  "use strict";

  document.addEventListener("click", function (e) {
    var el = e.target.closest ? e.target.closest("[data-print]") : null;
    if (el) window.print();
  });

  // Settings, Appearance: light, dark, or the default (the household theme's
  // mode, else the device). Stored per device; theme.js applies it before paint.
  function themeChoice() {
    try { return localStorage.getItem("mp-theme") || "system"; } catch (e) { return "system"; }
  }
  function householdMode() {
    var meta = document.querySelector('meta[name="household-theme-mode"]');
    return meta ? meta.content : "system";
  }
  function applyTheme(value) {
    var root = document.documentElement;
    var mode = value === "light" || value === "dark" ? value : householdMode();
    if (mode === "light" || mode === "dark") root.setAttribute("data-theme", mode);
    else root.removeAttribute("data-theme");
    try {
      if (value === "system") localStorage.removeItem("mp-theme");
      else localStorage.setItem("mp-theme", value);
    } catch (e) {}
  }
  function syncTheme() {
    var value = themeChoice();
    document.querySelectorAll("[data-theme-choice]").forEach(function (group) {
      group.hidden = false;
      group.querySelectorAll("[data-theme-value]").forEach(function (btn) {
        btn.setAttribute("aria-checked", btn.getAttribute("data-theme-value") === value ? "true" : "false");
      });
    });
  }
  document.addEventListener("click", function (e) {
    var btn = e.target.closest ? e.target.closest("[data-theme-choice] [data-theme-value]") : null;
    if (!btn) return;
    applyTheme(btn.getAttribute("data-theme-value"));
    syncTheme();
  });
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", syncTheme);
  else syncTheme();
  document.addEventListener("htmx:afterSettle", syncTheme);

  var script = document.currentScript;
  var sw = script && script.getAttribute("data-sw");
  if (sw && "serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register(sw).catch(function () {});
    });
  }
})();
