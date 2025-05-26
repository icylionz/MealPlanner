document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    activeTab: "calendar",
    showModal: false,

    init() {
      this.activeTab = "calendar";
      this.showModal = false;
    },

    showScheduleModal(date) {
      this.showModal = true;
      // Create modal container if it doesn't exist
      this.ensureModalContainer();
      
      htmx.ajax("GET", `/schedules/modal?date=${date.date}`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    showEditFoodModal(food) {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/foods/${food.id}/edit`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    showAddFoodModal() {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/foods/new`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    showViewFoodModal(food) {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/foods/modal/details?id=${food.id}`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    showShoppingListGenerateModal() {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/shopping-lists/generate`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    toggleModal(value) {
      this.showModal = value;
      if (!value) {
        this.clearModalContainer();
      }
    },

    ensureModalContainer() {
      let container = document.getElementById("dynamic-modal-container");
      if (!container) {
        container = document.createElement("div");
        container.id = "dynamic-modal-container";
        container.className = "fixed inset-0 z-50 overflow-y-auto";
        container.setAttribute("x-show", "$store.mealPlanner.showModal");
        document.body.appendChild(container);
      }
      container.innerHTML = "";
    },

    clearModalContainer() {
      const container = document.getElementById("dynamic-modal-container");
      if (container) {
        container.innerHTML = "";
      }
    },

    navigateToCalendar() {
      this.activeTab = "calendar";
      htmx.ajax("GET", "/calendar", {
        target: "#main-content",
      });
    },
    navigateToFoods() {
      this.activeTab = "foods";
      htmx.ajax("GET", "/foods", {
        target: "#main-content",
      });
    },

    navigateToShoppingLists() {
      this.activeTab = "shoppinglists";
      htmx.ajax("GET", "/shopping-lists", {
        target: "#main-content",
      });
    },
  });
});

document.addEventListener("htmx:configRequest", function (evt) {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  evt.detail.headers["X-Timezone"] = timezone;
  localStorage.setItem("userTimezone", timezone);
});
