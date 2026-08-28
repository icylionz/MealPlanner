(function () {
  "use strict";

  if (!("serviceWorker" in navigator)) return;

  var script = document.currentScript;
  var workerURL = script && script.getAttribute("data-service-worker");
  var scope = script && script.getAttribute("data-service-worker-scope");
  if (!workerURL || !scope) return;

  window.addEventListener("load", function () {
    navigator.serviceWorker.register(workerURL, { scope: scope }).catch(function (error) {
      console.warn("Service worker registration failed:", error);
    });
  });
})();
