// Progressive-enhancement search-select. Ported from RHWallet: any
// <select data-search-select="1"> is replaced with a searchable text input +
// filtered dropdown of its options. Works without a server round-trip; the
// underlying <select> still holds the value and submits normally with the form.
//
// Opt-in attributes on the <select>:
//   data-search-select="1"                 enable enhancement
//   data-search-select-allow-custom="1"    allow free-text values (adds option)
//   data-search-select-placeholder="…"     input placeholder
(function () {
	"use strict";

	function eqFold(a, b) {
		return String(a || "").trim().toLowerCase() === String(b || "").trim().toLowerCase();
	}

	function render(root) {
		if (!root) return;
		var select = root.__ssSelect;
		var input = root.querySelector("[data-ss-input]");
		var menu = root.querySelector("[data-ss-menu]");
		if (!select || !input || !menu) return;

		var allowCustom = root.getAttribute("data-search-select-allow-custom") === "1";
		var allowEmpty = !select.required;
		var selected = select.options[select.selectedIndex] || null;
		var placeholder = root.getAttribute("data-search-select-placeholder") || "Type to search";
		var queryRaw = String(input.value || "").trim();
		var query = queryRaw.toLowerCase();

		if (document.activeElement !== input) {
			if (selected && String(selected.value || "").trim() !== "") {
				input.value = (selected.textContent || "").trim();
			} else {
				input.value = "";
			}
			input.placeholder = placeholder;
		}

		var options = Array.prototype.filter.call(select.options, function (opt) {
			if (String(opt.value || "").trim() === "") return false;
			if (!query) return true;
			return (opt.textContent || "").toLowerCase().indexOf(query) !== -1;
		});

		var frag = document.createDocumentFragment();
		if (allowEmpty && selected && String(selected.value || "").trim() !== "") {
			var clearBtn = document.createElement("button");
			clearBtn.type = "button";
			clearBtn.className = "ss-clear";
			clearBtn.setAttribute("data-ss-clear", "1");
			clearBtn.textContent = "Clear selection";
			frag.appendChild(clearBtn);
		}
		options.slice(0, 20).forEach(function (opt) {
			var btn = document.createElement("button");
			btn.type = "button";
			btn.className = "ss-option";
			btn.setAttribute("data-ss-option", opt.value);
			btn.textContent = (opt.textContent || "").trim();
			if (selected && opt.value === selected.value) btn.classList.add("ss-selected");
			frag.appendChild(btn);
		});

		if (allowCustom && queryRaw !== "") {
			var exists = Array.prototype.some.call(select.options, function (opt) {
				return eqFold(opt.value, queryRaw) || eqFold(opt.textContent || "", queryRaw);
			});
			if (!exists) {
				var createBtn = document.createElement("button");
				createBtn.type = "button";
				createBtn.className = "ss-create";
				createBtn.setAttribute("data-ss-create", queryRaw);
				createBtn.textContent = 'Use "' + queryRaw + '"';
				frag.appendChild(createBtn);
			}
		}

		menu.innerHTML = "";
		menu.appendChild(frag);
		menu.classList.toggle("hidden", !input.matches(":focus") || menu.childElementCount === 0);

		var candidates = Array.prototype.slice.call(
			menu.querySelectorAll("[data-ss-option], [data-ss-create]")
		);
		if (candidates.length === 0) {
			root.__ssActive = -1;
			return;
		}
		var active = Number.isInteger(root.__ssActive) ? root.__ssActive : -1;
		if (active < 0 || active >= candidates.length) {
			var selIdx = candidates.findIndex(function (b) {
				return b.classList.contains("ss-selected");
			});
			active = selIdx >= 0 ? selIdx : 0;
		}
		root.__ssActive = active;
		candidates.forEach(function (b, i) {
			if (i === active) {
				b.classList.add("ss-active");
				b.scrollIntoView({ block: "nearest" });
			} else {
				b.classList.remove("ss-active");
			}
		});
	}

	function commit(root) {
		var select = root.__ssSelect;
		var input = root.querySelector("[data-ss-input]");
		if (!select || !input) return;
		var raw = String(input.value || "").trim();
		var allowCustom = root.getAttribute("data-search-select-allow-custom") === "1";
		if (raw === "") {
			if (!select.required) {
				select.value = "";
				select.dispatchEvent(new Event("change", { bubbles: true }));
			}
			return;
		}
		var option = Array.prototype.find.call(select.options, function (opt) {
			return eqFold(opt.value, raw) || eqFold(opt.textContent || "", raw);
		});
		if (!option && allowCustom) {
			option = document.createElement("option");
			option.value = raw;
			option.textContent = raw;
			select.appendChild(option);
		}
		if (!option) return;
		select.value = option.value;
		select.dispatchEvent(new Event("change", { bubbles: true }));
	}

	function init(scope) {
		(scope || document).querySelectorAll("select[data-search-select='1']").forEach(function (select) {
			if (select.dataset.ssInit === "1") return;
			select.dataset.ssInit = "1";

			var root = document.createElement("div");
			root.className = "ss-root";
			root.setAttribute("data-ss-root", "1");
			root.setAttribute(
				"data-search-select-allow-custom",
				select.getAttribute("data-search-select-allow-custom") || "0"
			);
			if (select.hasAttribute("data-search-select-placeholder")) {
				root.setAttribute(
					"data-search-select-placeholder",
					select.getAttribute("data-search-select-placeholder") || ""
				);
			}
			root.innerHTML =
				'<input data-ss-input class="input input-sm ss-input" autocomplete="off" placeholder="Type to search"/>' +
				'<div data-ss-menu class="ss-menu hidden"></div>';
			select.insertAdjacentElement("afterend", root);
			select.classList.add("hidden");
			select.tabIndex = -1;
			select.setAttribute("aria-hidden", "true");
			select.__ssRoot = root;
			root.__ssSelect = select;

			var input = root.querySelector("[data-ss-input]");
			input.addEventListener("focus", function () { render(root); });
			input.addEventListener("input", function () { root.__ssActive = -1; render(root); });
			input.addEventListener("keydown", function (event) {
				var menu = root.querySelector("[data-ss-menu]");
				var opts = menu ? Array.prototype.slice.call(menu.querySelectorAll("[data-ss-option], [data-ss-create]")) : [];
				if (event.key === "Escape") { if (menu) menu.classList.add("hidden"); return; }
				if (event.key === "ArrowDown" || event.key === "ArrowUp") {
					if (opts.length === 0) return;
					event.preventDefault();
					var delta = event.key === "ArrowUp" ? -1 : 1;
					var cur = Number.isInteger(root.__ssActive) ? root.__ssActive : -1;
					root.__ssActive = ((cur + delta) % opts.length + opts.length) % opts.length;
					render(root);
					return;
				}
				if (event.key !== "Enter") return;
				event.preventDefault();
				var idx = Number.isInteger(root.__ssActive) ? root.__ssActive : 0;
				var active = opts[idx] || opts[0];
				if (active) { active.click(); return; }
				commit(root);
			});
			input.addEventListener("blur", function () {
				commit(root);
				setTimeout(function () {
					var menu = root.querySelector("[data-ss-menu]");
					if (menu) menu.classList.add("hidden");
				}, 120);
			});

			// keep menu open on option mousedown (blur fires otherwise)
			root.addEventListener("pointerdown", function (event) {
				if (event.target.closest("[data-ss-option], [data-ss-create], [data-ss-clear]")) {
					event.preventDefault();
				}
			});
			select.addEventListener("change", function () { render(root); });
			render(root);
		});
	}

	document.addEventListener("click", function (event) {
		var optBtn = event.target.closest("[data-ss-option]");
		if (optBtn) {
			var root = optBtn.closest("[data-ss-root='1']");
			var select = root && root.__ssSelect;
			var menu = root && root.querySelector("[data-ss-menu]");
			var input = root && root.querySelector("[data-ss-input]");
			if (select) {
				select.value = optBtn.getAttribute("data-ss-option") || "";
				select.dispatchEvent(new Event("change", { bubbles: true }));
			}
			if (input) {
				var sel = select && select.options[select.selectedIndex];
				input.value = sel ? (sel.textContent || "").trim() : "";
			}
			if (menu) menu.classList.add("hidden");
			return;
		}

		var createBtn = event.target.closest("[data-ss-create]");
		if (createBtn) {
			var root2 = createBtn.closest("[data-ss-root='1']");
			var select2 = root2 && root2.__ssSelect;
			var menu2 = root2 && root2.querySelector("[data-ss-menu]");
			var input2 = root2 && root2.querySelector("[data-ss-input]");
			var raw = String(createBtn.getAttribute("data-ss-create") || "").trim();
			if (select2 && raw !== "") {
				var opt = Array.prototype.find.call(select2.options, function (o) { return eqFold(o.value, raw); });
				if (!opt) {
					opt = document.createElement("option");
					opt.value = raw;
					opt.textContent = raw;
					select2.appendChild(opt);
				}
				select2.value = opt.value;
				select2.dispatchEvent(new Event("change", { bubbles: true }));
			}
			if (input2) input2.value = raw;
			if (menu2) menu2.classList.add("hidden");
			return;
		}

		var clearBtn = event.target.closest("[data-ss-clear]");
		if (clearBtn) {
			var root3 = clearBtn.closest("[data-ss-root='1']");
			var select3 = root3 && root3.__ssSelect;
			var menu3 = root3 && root3.querySelector("[data-ss-menu]");
			var input3 = root3 && root3.querySelector("[data-ss-input]");
			if (select3) {
				select3.value = "";
				select3.dispatchEvent(new Event("change", { bubbles: true }));
			}
			if (input3) input3.value = "";
			if (menu3) menu3.classList.add("hidden");
		}
	});

	document.addEventListener("DOMContentLoaded", function () { init(document); });
	// hx-boost / htmx swaps replace body content — re-enhance new selects.
	document.addEventListener("htmx:afterSwap", function () { init(document); });
	document.addEventListener("htmx:load", function () { init(document); });
})();
