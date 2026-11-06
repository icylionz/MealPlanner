// Progressive enhancement only: every flow works without this file.
(function () {
	"use strict";

	// Selects and date inputs marked data-autosubmit submit their form on change.
	document.addEventListener("change", function (e) {
		var el = e.target;
		if (el && el.matches && el.matches("[data-autosubmit]") && el.form) {
			if (typeof el.form.requestSubmit === "function") el.form.requestSubmit();
			else el.form.submit();
		}
	});

	// Forms marked data-confirm ask before destructive actions.
	document.addEventListener("submit", function (e) {
		var form = e.target;
		var msg = form && form.getAttribute && form.getAttribute("data-confirm");
		if (msg && !window.confirm(msg)) {
			e.preventDefault();
			e.stopImmediatePropagation();
		}
	}, true);

	// Live filter on the foods list: debounce typing, then submit the GET form.
	// With hx-boost the body swaps in place and htmx restores focus to the
	// input by id.
	var timer;
	document.addEventListener("input", function (e) {
		var el = e.target;
		if (!el || !el.matches || !el.matches("[data-live-input]") || !el.form) return;
		clearTimeout(timer);
		timer = setTimeout(function () {
			if (typeof el.form.requestSubmit === "function") el.form.requestSubmit();
			else el.form.submit();
		}, 350);
	});

	// Print button on printable views.
	document.addEventListener("click", function (e) {
		var btn = e.target.closest && e.target.closest("[data-print]");
		if (btn) window.print();
	});

	// Desktop sidebar collapse, remembered locally.
	function applySidebar() {
		if (localStorage.getItem("mp-sidebar") === "collapsed") document.body.classList.add("sidebar-collapsed");
	}
	document.addEventListener("click", function (e) {
		var t = e.target.closest && e.target.closest("[data-nav-desktop-toggle]");
		if (!t) return;
		var collapsed = document.body.classList.toggle("sidebar-collapsed");
		localStorage.setItem("mp-sidebar", collapsed ? "collapsed" : "open");
		t.setAttribute("aria-expanded", collapsed ? "false" : "true");
	});
	document.addEventListener("DOMContentLoaded", applySidebar);
	document.addEventListener("htmx:afterSettle", applySidebar);

	// Register the service worker for installability (static assets only).
	var script = document.currentScript;
	var base = (script && script.getAttribute("data-base")) || "";
	if ("serviceWorker" in navigator) {
		window.addEventListener("load", function () {
			navigator.serviceWorker.register(base + "/sw.js", { scope: base + "/" }).catch(function () {});
		});
	}
})();
