document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    activeTab: "calendar",
    showModal: false,
    currentDate: new Date().toLocaleDateString("en-CA"),

    init() {
      const path = window.location.pathname;
      if (path.startsWith("/shopping-lists")) {
        this.activeTab = "shoppinglists";
      } else if (path.startsWith("/foods")) {
        this.activeTab = "foods";
      } else {
        // default for '/' and '/calendar'
        this.activeTab = "calendar";
      }
      this.showModal = false;
      this.currentDate = this.getToday();
    },

    getToday() {
      return new Date().toLocaleDateString("en-CA");
    },

    setCurrentDate(date) {
      this.currentDate = date;
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

    showEditScheduleModal(schedule) {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/schedules/${schedule.id}/edit`, {
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
    refreshCalendar() {
      htmx.ajax("GET", "/calendar", {
        target: "#calendar-container",
        swap: "outerHTML",
        values: { date: this.currentDate },
      });
    },

    showCreateShoppingListModal() {
      this.showModal = true;
      this.ensureModalContainer();

      htmx.ajax("GET", `/shopping-lists/new`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },

    showAddItemsModal() {
      this.showModal = true;
      this.ensureModalContainer();

      // Get current list ID from URL
      const pathParts = window.location.pathname.split("/");
      const listId = pathParts[pathParts.length - 1];

      htmx.ajax("GET", `/shopping-lists/${listId}/add-items`, {
        target: "#dynamic-modal-container",
        swap: "innerHTML",
      });
    },
  });
});

document.addEventListener("htmx:configRequest", function (evt) {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  evt.detail.headers["X-Timezone"] = timezone;
  localStorage.setItem("userTimezone", timezone);
});

document.addEventListener("closeModal", function () {
  Alpine.store("mealPlanner").toggleModal(false);
});
