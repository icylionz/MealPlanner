document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    activeTab: "calendar",
    viewMode: "month",
    currentDate: new Date().toISOString().split("T")[0], 
    showModal: false,

    init() {
      this.activeTab = "calendar";
      this.viewMode = "month";
      this.currentDate = new Date().toISOString().split("T")[0]; 
      this.showModal = false;
    },

    showScheduleModal(date) {
      this.toggleModal(true);
      const container = document.getElementById("modal-container");
      if (!container) return;
      container.innerHTML = "";

      htmx.ajax("GET", `/schedules/modal?date=${date.date}`, {
        target: "#modal-container",
        swap: "innerHTML",
      });
    },

    showEditFoodModal(food) {
      this.toggleModal(true);
      const container = document.getElementById("modal-container");
      if (!container) return;
      container.innerHTML = "";

      htmx.ajax("GET", `/foods/${food.id}/edit`, {
        target: "#modal-container",
        swap: "innerHTML",
      });
    },

    showAddFoodModal() {
      this.toggleModal(true);
      const container = document.getElementById("modal-container");
      if (!container) return;
      container.innerHTML = "";

      htmx.ajax("GET", `/foods/new`, {
        target: "#modal-container",
        swap: "innerHTML",
      });
    },

    showViewFoodModal(food) {
      this.toggleModal(true);
      const container = document.getElementById("modal-container");
      if (!container) return;
      container.innerHTML = "";

      htmx.ajax("GET", `/foods/modal/details?id=${food.id}`, {
        target: "#modal-container",
        swap: "innerHTML",
      });
    },

    toggleModal(value) {
      this.showModal = value;
      if (!value) {
        const container = document.getElementById("modal-container");
        if (!container) return;
        container.innerHTML = "";
      }
    },

    navigateToCalendar() {
      this.activeTab = "calendar";

      htmx.ajax(
        "GET",
        `/calendar?mode=${this.viewMode}&date=${this.currentDate}`,
        {
          target: "#main-content",
        },
      );
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

document.addEventListener('htmx:configRequest', function(evt) {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  evt.detail.headers['X-Timezone'] = timezone;
  localStorage.setItem('userTimezone', timezone);
});