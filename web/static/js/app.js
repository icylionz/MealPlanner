document.addEventListener("htmx:configRequest", (event) => {
  const token = document.querySelector('meta[name="csrf-token"]')?.content;
  if (token) {
    event.detail.headers["X-CSRF-Token"] = token;
  }
});

const mobileNavQuery = window.matchMedia("(max-width: 980px)");
const navDrawer = document.querySelector("[data-nav-drawer]");
const navToggle = document.querySelector("[data-nav-toggle]");
const navClosers = document.querySelectorAll("[data-nav-close]");

function setMobileNavOpen(open) {
  const shouldOpen = open && mobileNavQuery.matches && !!navDrawer && !!navToggle;
  document.body.classList.toggle("nav-open", shouldOpen);
  navToggle?.setAttribute("aria-expanded", shouldOpen ? "true" : "false");
}

navToggle?.addEventListener("click", () => {
  setMobileNavOpen(!document.body.classList.contains("nav-open"));
});

navClosers.forEach((element) => {
  element.addEventListener("click", () => {
    setMobileNavOpen(false);
  });
});

navDrawer?.querySelectorAll("a").forEach((link) => {
  link.addEventListener("click", () => {
    setMobileNavOpen(false);
  });
});

mobileNavQuery.addEventListener("change", () => {
  if (!mobileNavQuery.matches) {
    setMobileNavOpen(false);
  }
});

window.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    setMobileNavOpen(false);
  }
});

if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/service-worker.js").catch(() => {});
  });
}
