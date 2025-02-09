document.addEventListener("alpine:init", () => {
  Alpine.store("mealPlanner", {
    activeTab: "calendar",
    viewMode: "month",
    init() {
      this.activeTab = "calendar";
      this.viewMode = "month";
    },
  });
});
