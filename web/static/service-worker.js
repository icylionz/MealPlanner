"use strict";

const scopeURL = new URL(self.registration.scope);
const scopeKey = scopeURL.pathname.replace(/[^a-zA-Z0-9_-]/g, "_") || "root";
const CACHE_PREFIX = `backbone-plate-shell-${scopeKey}-`;
const CACHE_NAME = `${CACHE_PREFIX}v1`;
const staticPath = new URL("static/", scopeURL).pathname;
const offlineURL = new URL("static/offline.html", scopeURL).href;
const shellAssets = [
  offlineURL,
  new URL("static/css/app.css", scopeURL).href,
  new URL("static/js/htmx.min.js", scopeURL).href,
  new URL("static/js/search-select.js", scopeURL).href,
  new URL("static/js/pwa-register.js", scopeURL).href,
  new URL("static/icons/icon-192.svg", scopeURL).href,
  new URL("static/icons/icon-512.svg", scopeURL).href
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(shellAssets))
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(
        keys
          .filter((key) => key.startsWith(CACHE_PREFIX) && key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      ))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== scopeURL.origin) return;

  // App pages always come from the server. Only a failed navigation receives
  // the read-only offline shell; authenticated HTML is never cached.
  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request).catch(() => caches.match(offlineURL))
    );
    return;
  }

  if (!url.pathname.startsWith(staticPath)) return;
  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) return cached;
      return fetch(request).then((response) => {
        if (!response.ok) return response;
        const copy = response.clone();
        return caches.open(CACHE_NAME)
          .then((cache) => cache.put(request, copy))
          .then(() => response);
      });
    })
  );
});
