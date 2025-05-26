document.addEventListener("alpine:init", () => {
  Alpine.data("calendar", () => ({
    
    changeDate(newDate) {
      this.$store.mealPlanner.setCurrentDate(newDate);
      this.refreshCalendar();
    },

    previousDay() {
      const date = new Date(this.$store.mealPlanner.currentDate);
      date.setDate(date.getDate() - 1);
      this.$store.mealPlanner.setCurrentDate(date.toISOString().split('T')[0]);
      this.refreshCalendar();
    },

    nextDay() {
      const date = new Date(this.$store.mealPlanner.currentDate);
      date.setDate(date.getDate() + 1);
      this.$store.mealPlanner.setCurrentDate(date.toISOString().split('T')[0]);
      this.refreshCalendar();
    },

    goToToday() {
      this.$store.mealPlanner.setCurrentDate(this.$store.mealPlanner.getToday());
      this.refreshCalendar();
    },

    refreshCalendar() {
      htmx.trigger("body", "calendarRefresh", {
        date: this.$store.mealPlanner.currentDate
      });
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
  }));
});