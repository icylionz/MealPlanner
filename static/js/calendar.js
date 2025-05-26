document.addEventListener("alpine:init", () => {
  Alpine.data("calendar", () => ({
    currentDate: new Date().toISOString().split('T')[0],

    init() {
      // Initialize with today's date
      this.currentDate = new Date().toISOString().split('T')[0];
    },

    changeDate(newDate) {
      this.currentDate = newDate;
      this.refreshCalendar();
    },

    previousDay() {
      const date = new Date(this.currentDate);
      date.setDate(date.getDate() - 1);
      this.currentDate = date.toISOString().split('T')[0];
      this.refreshCalendar();
    },

    nextDay() {
      const date = new Date(this.currentDate);
      date.setDate(date.getDate() + 1);
      this.currentDate = date.toISOString().split('T')[0];
      this.refreshCalendar();
    },

    goToToday() {
      this.currentDate = new Date().toISOString().split('T')[0];
      this.refreshCalendar();
    },

    refreshCalendar() {
      htmx.trigger("body", "calendarRefresh", {
        date: this.currentDate
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