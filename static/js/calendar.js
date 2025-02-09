document.addEventListener("alpine:init", () => {
  Alpine.data("calendar", () => ({
    showContextMenu: false,
    contextMenuPosition: { x: 0, y: 0 },
    isMobile: window.innerWidth < 640,

    init() {
      this.setupMobileDetection();
    },

    setupMobileDetection() {
      window.addEventListener("resize", () => {
        this.isMobile = window.innerWidth < 640;
      });
    },

    switchViewMode(mode) {
      this.$store.mealPlanner.viewMode = mode;
      htmx.trigger("body", "viewModeChange", { mode: mode });
    },
    

    handleContextMenu(event, date) {
      event.preventDefault();
      this.contextMenuPosition = {
        x: event.clientX,
        y: event.clientY,
      };
      this.showContextMenu = true;
    },
  }));
});

document.body.addEventListener('viewModeChange', (event) => {
    console.log('ViewMode Event:', event);
    console.log('Mode value:', event.detail.mode);  // most likely path
    console.log('Alternative mode path:', event.mode); // possible alternative
    
    // Log all properties to be sure
    console.log('All event properties:', Object.keys(event));
});