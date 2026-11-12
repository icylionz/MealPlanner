// Small progressive enhancements. Every flow works without this file.
(function () {
  "use strict";

  document.addEventListener("click", function (e) {
    var el = e.target.closest ? e.target.closest("[data-print]") : null;
    if (el) window.print();
  });

  var script = document.currentScript;
  var sw = script && script.getAttribute("data-sw");
  if (sw && "serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register(sw).catch(function () {});
    });
  }
})();
