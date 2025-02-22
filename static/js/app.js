document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    activeTab: "calendar",
    viewMode: "month",
    showModal: false,

    init() {
      this.activeTab = "calendar";
      this.viewMode = "month";
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
  });
});
