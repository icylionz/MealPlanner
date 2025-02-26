document.addEventListener("alpine:init", () => {
  Alpine.data("calendar", () => ({
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

      htmx.trigger("body", "viewModeChange", {
        mode: mode,
        date: this.$store.mealPlanner.currentDate,
      });
    },

    switchCalendarView(mode, date) {
      this.$store.mealPlanner.viewMode = mode;
      this.$store.mealPlanner.currentDate = date;

      htmx.trigger("body", "viewModeChange", {
        mode: this.$store.mealPlanner.viewMode,
        date: this.$store.mealPlanner.currentDate,
      });
    },

    showContextMenu(event, day) {
      const container = this.$refs.contextMenuContainer;
      if (!container) return;

      container.innerHTML = "";

      htmx.ajax(
        "GET",
        `/calendar/context-menu?date=${day.date}&x=${event.clientX}&y=${event.clientY}`,
        {
          target: this.$refs.contextMenuContainer,
          swap: "innerHTML",
        },
      );
    },

    deleteSchedulesForDateRange(fromDate, toDate) {
      htmx.ajax(
        "DELETE",
        `/schedules/date-range?start=${fromDate}&end=${toDate}`,
        {
          target: "#calendar-container",
          confirm:
            "Are you sure you want to delete all schedules for this day?",
          handler: (_, xhr) => {
            if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
              this.refreshCalendar();
            }
          },
        },
      );
    },

    deleteScheduleById(id) {
      htmx.ajax("DELETE", `/schedules/ids?ids=${id}`, {
        confirm: "Are you sure you want to delete this schedule?",
        handler: (_, xhr) => {
          if (xhr.xhr.status >= 200 && xhr.xhr.status < 300) {
            this.refreshCalendar();
          }
        },
      });
    },

    refreshCalendar() {
      htmx.trigger("body", "calendarRefresh", {
        mode: this.$store.mealPlanner.viewMode,
        date: this.$store.mealPlanner.currentDate,
      });
    },
  }));
});

// document.body.addEventListener("viewModeChange", (event) => {
//   console.log("ViewMode Event:", event);
//   console.log("Mode value:", event.detail.mode); // most likely path
//   console.log("Alternative mode path:", event.mode); // possible alternative

//   // Log all properties to be sure
//   console.log("All event properties:", Object.keys(event));
// });
